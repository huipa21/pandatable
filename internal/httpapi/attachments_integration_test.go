package httpapi

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"pandatable/internal/attachments"
	"pandatable/internal/structure"
)

func attachmentUpload(f *aclFixture, user *aclUser, tableID, fieldID, name string, content []byte) *httptest.ResponseRecorder {
	f.t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", name)
	if err != nil {
		f.t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		f.t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		f.t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost,
		"/api/v1/tables/"+tableID+"/fields/"+fieldID+"/attachments", &body)
	request.RemoteAddr = "192.0.2.250:5000"
	f.decorate(request, user)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, request)
	return response
}

func TestAttachmentUploadRowACLAndFileLifecycle(t *testing.T) {
	f := newACLFixture(t)
	owner, stranger := f.user("user", false), f.user("user", false)
	table := f.newTable(owner, f.newBook(owner, f.newWorkspace(owner, "Attachment test").ID, "Book").ID, "Table")
	field := f.newField(owner, table.Table.ID,
		`{"name":"Files","uiType":"attachment","options":{"accept":"image/*, application/pdf","maxFiles":2}}`)
	otherField := f.newField(owner, table.Table.ID,
		`{"name":"Other files","uiType":"attachment","options":{"maxFiles":2}}`)
	created := f.api(owner, http.MethodPost, "/tables/"+table.Table.ID+"/rows", `{"rows":[{"fields":{}}]}`)
	row := decode[struct {
		Rows []structure.RowRecord `json:"rows"`
	}](f, created, http.StatusCreated).Rows[0]

	if _, err := f.pool.Exec(f.ctx, `INSERT INTO meta.settings(key,value) VALUES
		('upload_max_mb','1'::jsonb),('image_max_megapixels','1'::jsonb)
		ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := f.pool.Exec(f.ctx, `DELETE FROM meta.settings WHERE key IN ('upload_max_mb','image_max_megapixels')`); err != nil {
			t.Error(err)
		}
	})

	var small bytes.Buffer
	if err := png.Encode(&small, image.NewRGBA(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatal(err)
	}
	imageResponse := attachmentUpload(f, owner, table.Table.ID, field.ID, "tiny.png", small.Bytes())
	if imageResponse.Code != http.StatusCreated {
		_, directErr := f.attachmentService.Upload(f.ctx, owner.id, table.Table.ID, field.ID,
			"direct.png", bytes.NewReader(small.Bytes()), 1<<20, 1)
		t.Fatalf("HTTP upload status=%d body=%s; direct service error=%v", imageResponse.Code, imageResponse.Body, directErr)
	}
	pngFile := decode[attachments.File](f, imageResponse, http.StatusCreated)
	if pngFile.MIME != "image/png" || pngFile.Thumb == nil || pngFile.Size != int64(small.Len()) {
		t.Fatalf("image upload = %+v", pngFile)
	}
	pdf := []byte("%PDF-1.4\nattachment fixture\n%%EOF\n")
	pdfFile := decode[attachments.File](f, attachmentUpload(f, owner, table.Table.ID, field.ID, "fixture.pdf", pdf), http.StatusCreated)
	if pdfFile.MIME != "application/pdf" || pdfFile.Thumb != nil {
		t.Fatalf("PDF upload = %+v", pdfFile)
	}
	html := []byte("<!doctype html><script>alert(1)</script>")
	htmlFile := decode[attachments.File](f,
		attachmentUpload(f, owner, table.Table.ID, otherField.ID, "disguised.png", html), http.StatusCreated)
	if htmlFile.MIME != "text/html" || htmlFile.Thumb != nil {
		t.Fatalf("disguised HTML upload = %+v", htmlFile)
	}
	disguised := f.api(owner, http.MethodGet, "/attachments/"+htmlFile.ID, "")
	f.expectStatus(disguised, http.StatusOK, "")
	if !strings.HasPrefix(disguised.Header().Get("Content-Disposition"), "attachment") ||
		disguised.Header().Get("Content-Type") != "text/html" ||
		disguised.Header().Get("X-Content-Type-Options") != "nosniff" ||
		!strings.Contains(disguised.Header().Get("Content-Security-Policy"), "sandbox") {
		t.Fatalf("unsafe attachment response headers = %v", disguised.Header())
	}
	if _, err := f.pool.Exec(f.ctx, `UPDATE meta.attachments SET mime_type='image/png' WHERE id=$1`, htmlFile.ID); err != nil {
		t.Fatal(err)
	}
	f.expectStatus(f.api(owner, http.MethodGet, "/attachments/"+htmlFile.ID, ""),
		http.StatusInternalServerError, "ERR_INTERNAL")
	if _, err := f.pool.Exec(f.ctx, `UPDATE meta.attachments SET mime_type='text/html' WHERE id=$1`, htmlFile.ID); err != nil {
		t.Fatal(err)
	}
	f.expectStatus(f.api(owner, http.MethodGet, "/attachments/"+htmlFile.ID, ""), http.StatusOK, "")
	f.expectStatus(attachmentUpload(f, owner, table.Table.ID, otherField.ID, "broken.png",
		[]byte("\x89PNG\r\n\x1a\nnot-an-image")), http.StatusBadRequest, "ERR_INVALID_IMAGE")

	f.expectStatus(f.api(stranger, http.MethodGet, "/attachments/"+pngFile.ID, ""), http.StatusNotFound, "ERR_NOT_FOUND")
	f.expectStatus(attachmentUpload(f, stranger, table.Table.ID, field.ID, "stranger.pdf", pdf), http.StatusNotFound, "ERR_NOT_FOUND")

	for _, file := range []attachments.File{pngFile, pdfFile} {
		response := f.api(owner, http.MethodGet, "/attachments/"+file.ID, "")
		f.expectStatus(response, http.StatusOK, "")
		wantDisposition := "attachment"
		if file.ID == pngFile.ID {
			wantDisposition = "inline"
		}
		if response.Header().Get("Content-Disposition") == "" || !strings.HasPrefix(response.Header().Get("Content-Disposition"), wantDisposition) ||
			response.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("file response headers = %v", response.Header())
		}
	}
	thumbResponse := f.api(owner, http.MethodGet, "/attachments/"+pngFile.ID+"/thumbnail", "")
	f.expectStatus(thumbResponse, http.StatusOK, "")
	if thumbResponse.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("thumbnail content type = %s", thumbResponse.Header().Get("Content-Type"))
	}

	cell := []attachments.File{pngFile, pdfFile}
	cellJSON, _ := json.Marshal(cell)
	patch := `{"fields":{"` + field.ID + `":` + string(cellJSON) + `}}`
	patched := f.api(owner, http.MethodPatch, "/tables/"+table.Table.ID+"/rows/"+row.ID, patch)
	if patched.Code != http.StatusOK {
		service, err := structure.New(f.pool, f.resolver)
		if err != nil {
			t.Fatal(err)
		}
		_, directErr := service.PatchRow(f.ctx, structure.Actor{ID: owner.id}, table.Table.ID, row.ID,
			map[string]json.RawMessage{field.ID: cellJSON})
		t.Fatalf("row PATCH status=%d body=%s; direct service error=%v", patched.Code, patched.Body, directErr)
	}
	f.expectStatus(patched, http.StatusOK, "")
	forged := `{"fields":{"` + otherField.ID + `":` + string(cellJSON) + `}}`
	f.expectStatus(f.api(owner, http.MethodPatch, "/tables/"+table.Table.ID+"/rows/"+row.ID, forged),
		http.StatusBadRequest, "ERR_VALIDATION")
	f.expectStatus(f.api(owner, http.MethodDelete, "/tables/"+table.Table.ID+"/fields/"+field.ID+"/attachments/"+pngFile.ID, ""),
		http.StatusConflict, "ERR_ATTACHMENT_IN_USE")
	for _, file := range []attachments.File{pngFile, pdfFile} {
		discardPath := "/tables/" + table.Table.ID + "/fields/" + field.ID + "/attachments/" + file.ID + "/discard"
		f.expectStatus(f.api(owner, http.MethodPost, discardPath, ""), http.StatusOK, "")
		f.expectStatus(f.api(owner, http.MethodGet, "/attachments/"+file.ID, ""), http.StatusOK, "")
	}

	var oversized bytes.Buffer
	oversized.WriteString("%PDF-1.4\n")
	oversized.Write(bytes.Repeat([]byte("x"), 1<<20))
	f.expectStatus(attachmentUpload(f, owner, table.Table.ID, field.ID, "large.pdf", oversized.Bytes()),
		http.StatusRequestEntityTooLarge, "ERR_UPLOAD_TOO_LARGE")

	imageInfo := image.NewRGBA(image.Rect(0, 0, 1200, 1000))
	for y := 0; y < 1000; y++ {
		for x := 0; x < 1200; x++ {
			imageInfo.Set(x, y, color.RGBA{R: 32, G: 64, B: 96, A: 255})
		}
	}
	var largeImage bytes.Buffer
	if err := png.Encode(&largeImage, imageInfo); err != nil {
		t.Fatal(err)
	}
	f.expectStatus(attachmentUpload(f, owner, table.Table.ID, field.ID, "large.png", largeImage.Bytes()),
		http.StatusRequestEntityTooLarge, "ERR_IMAGE_TOO_LARGE")

	emptyCell := `{"fields":{"` + field.ID + `":[]}}`
	f.expectStatus(f.api(owner, http.MethodPatch, "/tables/"+table.Table.ID+"/rows/"+row.ID, emptyCell), http.StatusOK, "")
	for _, file := range []attachments.File{pngFile, pdfFile} {
		discardPath := "/tables/" + table.Table.ID + "/fields/" + field.ID + "/attachments/" + file.ID + "/discard"
		f.expectStatus(f.api(owner, http.MethodPost, discardPath, ""), http.StatusOK, "")
		f.expectStatus(f.api(owner, http.MethodGet, "/attachments/"+file.ID, ""), http.StatusNotFound, "ERR_NOT_FOUND")
	}
}
