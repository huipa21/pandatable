package httpapi

import (
	"bytes"
	"context"
	"encoding/csv"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"pandatable/internal/acl"
	"pandatable/internal/structure"
)

func (f *aclFixture) importUpload(user *aclUser, path, filename, content string) *httptest.ResponseRecorder {
	f.t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		f.t.Fatal(err)
	}
	if _, err := io.WriteString(part, content); err != nil {
		f.t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		f.t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1"+path, &body)
	f.decorate(request, user)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, request)
	return response
}

func TestImportAPIJobsPermissionsAndReport(t *testing.T) {
	f := newACLFixture(t)
	owner, reader, stranger := f.user("user", false), f.user("user", false), f.user("user", false)
	workspace := f.newWorkspace(owner, "Import API")
	book := f.newBook(owner, workspace.ID, "Import")
	f.grantOK(owner, acl.Workspace, workspace.ID, "user", reader.id, acl.Read)
	path := "/books/" + book.ID + "/imports/analyze"
	f.expectStatus(f.importUpload(reader, path, "input.csv", "Amount\n1\n"), 403, "ERR_PERMISSION_DENIED")
	f.expectStatus(f.importUpload(stranger, path, "input.csv", "Amount\n1\n"), 404, "ERR_NOT_FOUND")

	analyzed := decode[struct {
		ID       string                   `json:"id"`
		Analysis structure.ImportAnalysis `json:"analysis"`
	}](f, f.importUpload(owner, path, "input.csv", "Amount\n1\nbad\n3\n"), 201)
	if analyzed.ID == "" || analyzed.Analysis.Format != "csv" || len(analyzed.Analysis.Columns) != 1 {
		t.Fatalf("analysis = %+v", analyzed)
	}
	jobPath := "/imports/" + analyzed.ID
	f.expectStatus(f.api(reader, "GET", jobPath, ""), 404, "ERR_NOT_FOUND")
	f.expectStatus(f.api(stranger, "GET", jobPath, ""), 404, "ERR_NOT_FOUND")
	f.expectStatus(f.api(owner, "POST", jobPath,
		`{"name":"Imported","hasHeader":true,"columns":[{"index":0,"name":"Amount","uiType":"integer","options":{}}]}`), 202, "")
	f.expectStatus(f.api(owner, "POST", jobPath,
		`{"name":"Again","hasHeader":true,"columns":[]}`), 409, "ERR_IMPORT_STATE")

	var status importJobStatus
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		status = decode[importJobStatus](f, f.api(owner, "GET", jobPath, ""), 200)
		if status.Status != "running" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if status.Status != "done" || status.Result.Rows != 3 || status.Result.InvalidCells != 1 ||
		status.Result.TableID == "" || !status.HasReport {
		t.Fatalf("job result = %+v", status)
	}
	report := f.api(owner, "GET", jobPath+"/issues", "")
	f.expectStatus(report, 200, "")
	if got := report.Header().Get("Content-Type"); got != "text/csv; charset=utf-8" {
		t.Fatalf("report content type = %q", got)
	}
	records, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(report.Body.String(), "\ufeff"))).ReadAll()
	if err != nil || len(records) != 2 || records[1][0] != "3" ||
		records[1][1] != "Amount" || records[1][2] != "bad" {
		t.Fatalf("report = %q, %v", report.Body.String(), err)
	}
	f.expectStatus(f.api(owner, "DELETE", jobPath, ""), 200, "")
	f.expectStatus(f.api(owner, "GET", jobPath, ""), 404, "ERR_NOT_FOUND")
}

func TestImportJobLimitsAndCleanup(t *testing.T) {
	f := newACLFixture(t)
	owner := f.user("user", false)
	workspace := f.newWorkspace(owner, "Import limits")
	book := f.newBook(owner, workspace.ID, "Import")
	path := "/books/" + book.ID + "/imports/analyze"
	ids := make([]string, 0, maxUserImportJobs)
	for range maxUserImportJobs {
		response := decode[struct {
			ID string `json:"id"`
		}](f, f.importUpload(owner, path, "small.csv", "Name\nAlice\n"), 201)
		ids = append(ids, response.ID)
	}
	f.expectStatus(f.importUpload(owner, path, "small.csv", "Name\nAlice\n"), 429, "ERR_IMPORT_BUSY")
	for _, id := range ids {
		f.expectStatus(f.api(owner, "DELETE", "/imports/"+id, ""), 200, "")
	}
	last := decode[struct {
		ID string `json:"id"`
	}](f, f.importUpload(owner, path, "small.csv", "Name\nAlice\n"), 201)
	f.expectStatus(f.api(owner, "DELETE", "/imports/"+last.ID, ""), 200, "")
}

func TestImportAPIShutdownCleansPendingAndCompletedJobs(t *testing.T) {
	f := newACLFixture(t)
	owner := f.user("user", false)
	book := f.newBook(owner, f.newWorkspace(owner, "Import shutdown").ID, "Import")
	path := "/books/" + book.ID + "/imports/analyze"
	analyze := func(content string) string {
		t.Helper()
		return decode[struct {
			ID string `json:"id"`
		}](f, f.importUpload(owner, path, "input.csv", content), 201).ID
	}
	pending := analyze("Name\nAlice\n")
	completed := analyze("Amount\n1\nbad\n3\n")
	jobPath := "/imports/" + completed
	f.expectStatus(f.api(owner, "POST", jobPath,
		`{"name":"Shutdown test","hasHeader":true,"columns":[{"index":0,"name":"Amount","uiType":"integer","options":{}}]}`), 202, "")
	managed := f.handler.(*managedHandler)
	var report string
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		status, reportPath, err := managed.imports.status(completed, owner.id)
		if err != nil {
			t.Fatal(err)
		}
		if status.Status == "done" {
			report = reportPath
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if report == "" {
		t.Fatal("completed import did not produce an issue report")
	}
	managed.imports.mu.Lock()
	upload := managed.imports.items[pending].uploadPath
	managed.imports.mu.Unlock()
	if upload == "" {
		t.Fatal("pending import did not retain its upload")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := managed.shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	requireImportFileRemoved(t, upload)
	requireImportFileRemoved(t, report)
	f.expectStatus(f.api(owner, "GET", "/imports/"+pending, ""), 404, "ERR_NOT_FOUND")
}
