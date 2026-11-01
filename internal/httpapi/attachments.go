package httpapi

import (
	"errors"
	"io"
	"mime"
	"net/http"

	"github.com/go-chi/chi/v5"

	"pandatable/internal/apperr"
	"pandatable/internal/auth"
)

const attachmentMultipartOverhead = 1 << 20

func (api *authHTTP) mountAttachments(router chi.Router) {
	router.Post("/tables/{id}/fields/{fieldId}/attachments", api.authenticated(api.uploadAttachment))
	router.Delete("/tables/{id}/fields/{fieldId}/attachments/{attachmentId}", api.authenticated(api.deleteAttachment))
	router.Post("/tables/{id}/fields/{fieldId}/attachments/{attachmentId}/discard", api.authenticated(api.discardAttachment))
	router.Get("/attachments/{id}", api.authenticated(api.getAttachment))
	router.Get("/attachments/{id}/thumbnail", api.authenticated(api.getAttachmentThumbnail))
}

func (api *authHTTP) uploadAttachment(w http.ResponseWriter, r *http.Request, user auth.User) {
	limits, err := api.admin.AttachmentLimits(r.Context())
	if err != nil {
		api.fail(w, r, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, limits.MaxBytes+attachmentMultipartOverhead)
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || contentType != "multipart/form-data" {
		api.fail(w, r, apperr.New(415, "ERR_CONTENT_TYPE", "Content-Type must be multipart/form-data"))
		return
	}
	multipart, err := r.MultipartReader()
	if err != nil {
		api.fail(w, r, apperr.New(400, "ERR_VALIDATION", "Invalid multipart upload"))
		return
	}
	part, err := multipart.NextPart()
	if err != nil || part.FormName() != "file" || part.FileName() == "" {
		if part != nil {
			_ = part.Close()
		}
		api.fail(w, r, apperr.New(400, "ERR_VALIDATION", "Upload requires one file part named file"))
		return
	}
	file, uploadErr := api.attachments.Upload(r.Context(), user.ID, chi.URLParam(r, "id"),
		chi.URLParam(r, "fieldId"), part.FileName(), part, limits.MaxBytes, limits.MaxImageMegapixels)
	closeErr := part.Close()
	if uploadErr != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(uploadErr, &tooLarge) {
			uploadErr = apperr.New(413, "ERR_UPLOAD_TOO_LARGE", "File exceeds the upload limit")
		}
		api.fail(w, r, uploadErr)
		return
	}
	if closeErr != nil {
		_ = api.attachments.Delete(r.Context(), user.ID, chi.URLParam(r, "id"), chi.URLParam(r, "fieldId"), file.ID)
		api.fail(w, r, apperr.New(400, "ERR_VALIDATION", "Invalid multipart upload"))
		return
	}
	if _, err := multipart.NextPart(); !errors.Is(err, io.EOF) {
		_ = api.attachments.Delete(r.Context(), user.ID, chi.URLParam(r, "id"), chi.URLParam(r, "fieldId"), file.ID)
		if err == nil {
			api.fail(w, r, apperr.New(400, "ERR_VALIDATION", "Upload accepts one file per request"))
		} else {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				err = apperr.New(413, "ERR_UPLOAD_TOO_LARGE", "File exceeds the upload limit")
			}
			api.fail(w, r, err)
		}
		return
	}
	writeJSON(w, http.StatusCreated, file)
}

func (api *authHTTP) getAttachment(w http.ResponseWriter, r *http.Request, user auth.User) {
	api.serveAttachment(w, r, user, false)
}

func (api *authHTTP) getAttachmentThumbnail(w http.ResponseWriter, r *http.Request, user auth.User) {
	api.serveAttachment(w, r, user, true)
}

func (api *authHTTP) serveAttachment(w http.ResponseWriter, r *http.Request, user auth.User, thumbnail bool) {
	if err := noQuery(r); err != nil {
		api.fail(w, r, err)
		return
	}
	file, err := api.attachments.Open(r.Context(), user.ID, chi.URLParam(r, "id"), thumbnail)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	defer file.Reader.Close()
	info, err := file.Reader.Stat()
	if err != nil {
		api.fail(w, r, apperr.New(404, "ERR_NOT_FOUND", "Not found"))
		return
	}
	disposition := "attachment"
	if file.Inline {
		disposition = "inline"
	}
	contentDisposition := mime.FormatMediaType(disposition, map[string]string{"filename": file.File.Name})
	if contentDisposition == "" {
		contentDisposition = "attachment"
	}
	w.Header().Set("Content-Type", file.ContentType)
	w.Header().Set("Content-Disposition", contentDisposition)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'; frame-ancestors 'none'")
	if file.SHA256 != "" && !thumbnail {
		w.Header().Set("ETag", `"`+file.SHA256+`"`)
	}
	http.ServeContent(w, r, file.File.Name, info.ModTime(), file.Reader)
}

func (api *authHTTP) deleteAttachment(w http.ResponseWriter, r *http.Request, user auth.User) {
	err := api.attachments.Delete(r.Context(), user.ID, chi.URLParam(r, "id"), chi.URLParam(r, "fieldId"),
		chi.URLParam(r, "attachmentId"))
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"eventId": eventID(r)})
}

func (api *authHTTP) discardAttachment(w http.ResponseWriter, r *http.Request, user auth.User) {
	err := api.attachments.Discard(r.Context(), user.ID, chi.URLParam(r, "id"), chi.URLParam(r, "fieldId"),
		chi.URLParam(r, "attachmentId"))
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"eventId": eventID(r)})
}
