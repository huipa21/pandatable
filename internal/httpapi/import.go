package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"pandatable/internal/acl"
	"pandatable/internal/apperr"
	"pandatable/internal/auth"
	"pandatable/internal/events"
	"pandatable/internal/structure"
)

const (
	importUploadTimeout = 10 * time.Minute
	importJobTimeout    = 30 * time.Minute
	importJobTTL        = 30 * time.Minute
	maxImportJobs       = 16
	maxUserImportJobs   = 4
	maxImportUploads    = 2
	importRequestLimit  = 1 << 20
)

type importJob struct {
	id         string
	userID     string
	target     structure.ImportTarget
	file       *structure.ImportFile
	uploadPath string
	reportPath string
	status     string
	result     structure.ImportResult
	errorCode  string
	timer      *time.Timer
}

type importJobs struct {
	mu     sync.Mutex
	items  map[string]*importJob
	logger *slog.Logger
}

type importJobStatus struct {
	ID        string                 `json:"id"`
	Status    string                 `json:"status"`
	Result    structure.ImportResult `json:"result"`
	ErrorCode string                 `json:"errorCode,omitempty"`
	HasReport bool                   `json:"hasReport"`
}

func newImportJobs(logger *slog.Logger) *importJobs {
	return &importJobs{items: make(map[string]*importJob), logger: logger}
}

func (jobs *importJobs) reserve(userID string, target structure.ImportTarget) (*importJob, error) {
	jobs.mu.Lock()
	defer jobs.mu.Unlock()
	userCount, uploads := 0, 0
	for _, job := range jobs.items {
		if job.userID == userID {
			userCount++
		}
		if job.status == "uploading" {
			uploads++
		}
	}
	if len(jobs.items) >= maxImportJobs || userCount >= maxUserImportJobs || uploads >= maxImportUploads {
		return nil, apperr.New(429, "ERR_IMPORT_BUSY", "Too many pending imports")
	}
	job := &importJob{userID: userID, target: target, status: "uploading"}
	for job.id == "" || jobs.items[job.id] != nil {
		job.id = rand.Text()
	}
	jobs.items[job.id] = job
	job.timer = time.AfterFunc(importJobTTL, func() { jobs.expire(job.id) })
	return job, nil
}

func (jobs *importJobs) expire(id string) {
	jobs.mu.Lock()
	job := jobs.items[id]
	if job == nil {
		jobs.mu.Unlock()
		return
	}
	if job.status == "running" || job.status == "uploading" {
		job.timer = time.AfterFunc(importJobTTL, func() { jobs.expire(id) })
		jobs.mu.Unlock()
		return
	}
	delete(jobs.items, id)
	upload, report := job.uploadPath, job.reportPath
	jobs.mu.Unlock()
	jobs.removeFiles(id, upload, report)
}

func (jobs *importJobs) removeFiles(id string, paths ...string) {
	for _, path := range paths {
		if path != "" {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				jobs.logger.Error("import temporary file cleanup failed", "job_id", id)
			}
		}
	}
}

func (jobs *importJobs) discard(id, userID string) error {
	jobs.mu.Lock()
	job := jobs.items[id]
	if job == nil || job.userID != userID {
		jobs.mu.Unlock()
		return acl.ErrNotFound()
	}
	if job.status == "running" {
		jobs.mu.Unlock()
		return apperr.New(409, "ERR_IMPORT_RUNNING", "Import is still running")
	}
	delete(jobs.items, id)
	job.timer.Stop()
	upload, report := job.uploadPath, job.reportPath
	jobs.mu.Unlock()
	jobs.removeFiles(id, upload, report)
	return nil
}

func (jobs *importJobs) target(id, userID string) (structure.ImportTarget, error) {
	jobs.mu.Lock()
	defer jobs.mu.Unlock()
	job := jobs.items[id]
	if job == nil || job.userID != userID || job.status == "uploading" {
		return structure.ImportTarget{}, acl.ErrNotFound()
	}
	return job.target, nil
}

func (jobs *importJobs) start(id, userID string) (*structure.ImportFile, structure.ImportTarget, error) {
	jobs.mu.Lock()
	defer jobs.mu.Unlock()
	job := jobs.items[id]
	if job == nil || job.userID != userID {
		return nil, structure.ImportTarget{}, acl.ErrNotFound()
	}
	if job.status != "ready" {
		return nil, structure.ImportTarget{}, apperr.New(409, "ERR_IMPORT_STATE", "Import is not ready")
	}
	for _, other := range jobs.items {
		if other.status == "running" {
			return nil, structure.ImportTarget{}, apperr.New(429, "ERR_IMPORT_BUSY", "Another import is running")
		}
	}
	job.status = "running"
	return job.file, job.target, nil
}

func (jobs *importJobs) progress(id string, result structure.ImportResult) {
	jobs.mu.Lock()
	defer jobs.mu.Unlock()
	if job := jobs.items[id]; job != nil && job.status == "running" {
		result.Issues = nil
		job.result = result
	}
}

func (jobs *importJobs) finish(id string, result structure.ImportResult, reportPath, errorCode string) {
	jobs.mu.Lock()
	job := jobs.items[id]
	if job == nil {
		jobs.mu.Unlock()
		jobs.removeFiles(id, reportPath)
		return
	}
	upload := job.uploadPath
	job.uploadPath, job.file = "", nil
	job.reportPath, job.errorCode = reportPath, errorCode
	result.Issues = nil
	job.result = result
	job.status = "done"
	if errorCode != "" {
		job.status = "failed"
	}
	job.timer.Stop()
	job.timer = time.AfterFunc(importJobTTL, func() { jobs.expire(id) })
	jobs.mu.Unlock()
	jobs.removeFiles(id, upload)
}

func (jobs *importJobs) status(id, userID string) (importJobStatus, string, error) {
	jobs.mu.Lock()
	defer jobs.mu.Unlock()
	job := jobs.items[id]
	if job == nil || job.userID != userID || job.status == "uploading" {
		return importJobStatus{}, "", acl.ErrNotFound()
	}
	return importJobStatus{ID: id, Status: job.status, Result: job.result, ErrorCode: job.errorCode,
		HasReport: job.reportPath != ""}, job.reportPath, nil
}

func (api *authHTTP) mountImports(router chi.Router) {
	router.Post("/books/{id}/imports/analyze", api.authenticated(api.analyzeBookImport))
	router.Post("/tables/{id}/imports/analyze", api.authenticated(api.analyzeTableImport))
	router.Post("/imports/{id}", api.authenticated(api.startImport))
	router.Get("/imports/{id}", api.authenticated(api.importStatus))
	router.Get("/imports/{id}/issues", api.authenticated(api.importIssues))
	router.Delete("/imports/{id}", api.authenticated(api.discardImport))
}

func (api *authHTTP) analyzeBookImport(w http.ResponseWriter, r *http.Request, user auth.User) {
	api.analyzeImport(w, r, user, structure.ImportTarget{Kind: "book", ID: chi.URLParam(r, "id")})
}

func (api *authHTTP) analyzeTableImport(w http.ResponseWriter, r *http.Request, user auth.User) {
	api.analyzeImport(w, r, user, structure.ImportTarget{Kind: "table", ID: chi.URLParam(r, "id")})
}

func (api *authHTTP) analyzeImport(w http.ResponseWriter, r *http.Request, user auth.User, target structure.ImportTarget) {
	if err := noQuery(r); err != nil {
		api.fail(w, r, err)
		return
	}
	actor := actorOf(user)
	if err := api.structure.CheckImportAccess(r.Context(), actor, target); err != nil {
		api.fail(w, r, err)
		return
	}
	limits, err := api.admin.AttachmentLimits(r.Context())
	if err != nil {
		api.fail(w, r, err)
		return
	}
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || contentType != "multipart/form-data" {
		api.fail(w, r, apperr.New(415, "ERR_CONTENT_TYPE", "Content-Type must be multipart/form-data"))
		return
	}
	controller := http.NewResponseController(w)
	deadline := time.Now().Add(importUploadTimeout)
	if err := controller.SetReadDeadline(deadline); err != nil && !errors.Is(err, http.ErrNotSupported) {
		api.fail(w, r, fmt.Errorf("set import read deadline: %w", err))
		return
	}
	if err := controller.SetWriteDeadline(deadline); err != nil && !errors.Is(err, http.ErrNotSupported) {
		api.fail(w, r, fmt.Errorf("set import write deadline: %w", err))
		return
	}
	job, err := api.imports.reserve(user.ID, target)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	ready := false
	defer func() {
		if !ready {
			if err := api.imports.discard(job.id, user.ID); err != nil {
				api.logger.Error("import reservation cleanup failed", "job_id", job.id)
			}
		}
	}()
	r.Body = http.MaxBytesReader(w, r.Body, limits.MaxBytes+attachmentMultipartOverhead)
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
	filename := part.FileName()
	staged, err := os.CreateTemp("", "pandatable-import-*")
	if err != nil {
		_ = part.Close()
		api.fail(w, r, fmt.Errorf("create import staging file: %w", err))
		return
	}
	api.imports.mu.Lock()
	job.uploadPath = staged.Name()
	api.imports.mu.Unlock()
	size, copyErr := io.Copy(staged, io.LimitReader(part, limits.MaxBytes+1))
	closeErr := staged.Close()
	partErr := part.Close()
	if copyErr != nil || size > limits.MaxBytes {
		var tooLarge *http.MaxBytesError
		if size > limits.MaxBytes || errors.As(copyErr, &tooLarge) {
			api.fail(w, r, apperr.New(413, "ERR_UPLOAD_TOO_LARGE", "File exceeds the upload limit"))
		} else {
			api.fail(w, r, fmt.Errorf("stage import upload: %w", copyErr))
		}
		return
	}
	if closeErr != nil {
		api.fail(w, r, fmt.Errorf("close import upload: %w", closeErr))
		return
	}
	if partErr != nil {
		api.fail(w, r, apperr.New(400, "ERR_VALIDATION", "Invalid multipart upload"))
		return
	}
	if next, err := multipart.NextPart(); !errors.Is(err, io.EOF) {
		if next != nil {
			_ = next.Close()
		}
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			api.fail(w, r, apperr.New(413, "ERR_UPLOAD_TOO_LARGE", "File exceeds the upload limit"))
		} else {
			api.fail(w, r, apperr.New(400, "ERR_VALIDATION", "Upload accepts one file per request"))
		}
		return
	}
	file, err := structure.AnalyzeImportFile(r.Context(), staged.Name(), filename, actor)
	if err == nil {
		err = api.structure.PrepareImport(r.Context(), actor, target, file)
	}
	if err != nil {
		api.fail(w, r, err)
		return
	}
	api.imports.mu.Lock()
	job.file, job.status = file, "ready"
	api.imports.mu.Unlock()
	ready = true
	writeJSON(w, http.StatusCreated, struct {
		ID       string                   `json:"id"`
		Analysis structure.ImportAnalysis `json:"analysis"`
	}{job.id, file.Analysis})
}

func (api *authHTTP) authorizedImport(w http.ResponseWriter, r *http.Request, user auth.User) (string, bool) {
	if err := noQuery(r); err != nil {
		api.fail(w, r, err)
		return "", false
	}
	id := chi.URLParam(r, "id")
	target, err := api.imports.target(id, user.ID)
	if err == nil {
		err = api.structure.CheckImportAccess(r.Context(), actorOf(user), target)
	}
	if err != nil {
		api.fail(w, r, err)
		return "", false
	}
	return id, true
}

func (api *authHTTP) startImport(w http.ResponseWriter, r *http.Request, user auth.User) {
	id, ok := api.authorizedImport(w, r, user)
	if !ok {
		return
	}
	var request structure.ImportRequest
	if err := decodeJSONLimit(w, r, &request, importRequestLimit); err != nil {
		api.fail(w, r, err)
		return
	}
	file, target, err := api.imports.start(id, user.ID)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	actor, event := actorOf(user), eventID(r)
	go api.runImport(id, actor, target, file, request, event)
	writeJSON(w, http.StatusAccepted, importJobStatus{ID: id, Status: "running"})
}

func (api *authHTTP) runImport(id string, actor structure.Actor, target structure.ImportTarget,
	file *structure.ImportFile, request structure.ImportRequest, eventID string,
) {
	ctx, cancel := context.WithTimeout(events.WithID(context.Background(), eventID), importJobTimeout)
	defer cancel()
	result, runErr := api.structure.RunImport(ctx, actor, target, file, request,
		func(result structure.ImportResult) { api.imports.progress(id, result) })
	if runErr != nil && target.Kind == "book" {
		result.Rows, result.TableID, result.ViewID = 0, "", ""
	}
	report, reportErr := writeImportReport(result.Issues, actor.Locale)
	code := ""
	if runErr != nil || reportErr != nil {
		code = "ERR_INTERNAL"
		var app *apperr.Error
		if reportErr == nil && errors.As(runErr, &app) && app.Status < 500 {
			code = app.Code
		}
		api.logger.Error("import job failed", "job_id", id, "error_code", code)
	}
	api.imports.finish(id, result, report, code)
}

func writeImportReport(issues []structure.ImportIssue, locale string) (string, error) {
	if len(issues) == 0 {
		return "", nil
	}
	file, err := os.CreateTemp("", "pandatable-import-report-*")
	if err != nil {
		return "", err
	}
	path := file.Name()
	if _, err = file.WriteString("\xef\xbb\xbf"); err == nil {
		writer := csv.NewWriter(file)
		header := []string{"Row", "Column", "Value"}
		if locale == "zh-CN" {
			header = []string{"行", "列", "值"}
		}
		err = writer.Write(header)
		for _, issue := range issues {
			if err != nil {
				break
			}
			err = writer.Write([]string{strconv.Itoa(issue.Row), issue.Column, issue.Value})
		}
		writer.Flush()
		if err == nil {
			err = writer.Error()
		}
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return "", errors.Join(err, removeErr)
		}
		return "", err
	}
	return path, nil
}

func (api *authHTTP) importStatus(w http.ResponseWriter, r *http.Request, user auth.User) {
	id, ok := api.authorizedImport(w, r, user)
	if !ok {
		return
	}
	status, _, err := api.imports.status(id, user.ID)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (api *authHTTP) importIssues(w http.ResponseWriter, r *http.Request, user auth.User) {
	id, ok := api.authorizedImport(w, r, user)
	if !ok {
		return
	}
	_, path, err := api.imports.status(id, user.ID)
	if err != nil || path == "" {
		if err == nil {
			err = acl.ErrNotFound()
		}
		api.fail(w, r, err)
		return
	}
	file, err := os.Open(path)
	if err != nil {
		api.fail(w, r, fmt.Errorf("open import issue report: %w", err))
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="import-issues.csv"`)
	w.Header().Set("Cache-Control", "no-store")
	if _, err := io.Copy(w, file); err != nil {
		api.logger.Error("import issue report download failed", "job_id", id)
	}
}

func (api *authHTTP) discardImport(w http.ResponseWriter, r *http.Request, user auth.User) {
	id, ok := api.authorizedImport(w, r, user)
	if !ok {
		return
	}
	if err := api.imports.discard(id, user.ID); err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct{}{})
}
