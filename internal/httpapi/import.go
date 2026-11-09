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
	removing   bool
}

type importJobs struct {
	mu           sync.Mutex
	items        map[string]*importJob
	logger       *slog.Logger
	ctx          context.Context
	cancel       context.CancelFunc
	active       sync.WaitGroup
	closed       bool
	shutdownOnce sync.Once
	shutdownDone chan struct{}
	shutdownErr  error
}

type importJobStatus struct {
	ID        string                 `json:"id"`
	Status    string                 `json:"status"`
	Result    structure.ImportResult `json:"result"`
	ErrorCode string                 `json:"errorCode,omitempty"`
	HasReport bool                   `json:"hasReport"`
}

func newImportJobs(logger *slog.Logger) *importJobs {
	ctx, cancel := context.WithCancel(context.Background())
	return &importJobs{items: make(map[string]*importJob), logger: logger, ctx: ctx, cancel: cancel,
		shutdownDone: make(chan struct{})}
}

func (jobs *importJobs) reserve(userID string, target structure.ImportTarget) (*importJob, error) {
	jobs.mu.Lock()
	defer jobs.mu.Unlock()
	if jobs.closed {
		return nil, apperr.New(503, "ERR_IMPORT_BUSY", "Imports are shutting down")
	}
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
	jobs.active.Add(1)
	job.timer = time.AfterFunc(importJobTTL, func() { jobs.expire(job.id) })
	return job, nil
}

func (jobs *importJobs) expire(id string) {
	jobs.mu.Lock()
	if jobs.closed {
		jobs.mu.Unlock()
		return
	}
	job := jobs.items[id]
	if job == nil {
		jobs.mu.Unlock()
		return
	}
	if job.removing {
		jobs.mu.Unlock()
		return
	}
	if job.status == "running" || job.status == "uploading" {
		job.timer = time.AfterFunc(importJobTTL, func() { jobs.expire(id) })
		jobs.mu.Unlock()
		return
	}
	job.timer.Stop()
	job.removing = true
	jobs.active.Add(1)
	jobs.mu.Unlock()
	defer jobs.active.Done()
	jobs.removeJob(job)
}

func (jobs *importJobs) removeFiles(id string, paths ...string) error {
	var result error
	for _, path := range paths {
		if path != "" {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				jobs.logger.Error("import temporary file cleanup failed", "job_id", id)
				result = errors.Join(result, fmt.Errorf("remove import temporary file: %w", err))
			}
		}
	}
	return result
}

func (jobs *importJobs) removeJob(job *importJob) error {
	uploadErr := jobs.removeFiles(job.id, job.uploadPath)
	reportErr := jobs.removeFiles(job.id, job.reportPath)
	err := errors.Join(uploadErr, reportErr)
	jobs.mu.Lock()
	defer jobs.mu.Unlock()
	job.removing = false
	if uploadErr == nil {
		job.uploadPath = ""
	}
	if reportErr == nil {
		job.reportPath = ""
	}
	if err == nil {
		delete(jobs.items, job.id)
		return nil
	}
	if job.status == "uploading" {
		job.status, job.errorCode = "failed", "ERR_INTERNAL"
	}
	job.timer = time.AfterFunc(importJobTTL, func() { jobs.expire(job.id) })
	return err
}

func (jobs *importJobs) shutdown(ctx context.Context) error {
	jobs.shutdownOnce.Do(func() {
		jobs.mu.Lock()
		jobs.closed = true
		jobs.cancel()
		jobs.mu.Unlock()
		go func() {
			jobs.active.Wait()
			jobs.mu.Lock()
			items := jobs.items
			jobs.items = make(map[string]*importJob)
			for _, job := range items {
				job.timer.Stop()
			}
			jobs.mu.Unlock()
			for id, job := range items {
				jobs.shutdownErr = errors.Join(jobs.shutdownErr, jobs.removeFiles(id, job.uploadPath, job.reportPath))
			}
			close(jobs.shutdownDone)
		}()
	})
	select {
	case <-jobs.shutdownDone:
		return jobs.shutdownErr
	case <-ctx.Done():
		return fmt.Errorf("wait for import jobs to shut down: %w", ctx.Err())
	}
}

func (jobs *importJobs) discard(id, userID string) error {
	jobs.mu.Lock()
	if jobs.closed {
		jobs.mu.Unlock()
		return apperr.New(503, "ERR_IMPORT_BUSY", "Imports are shutting down")
	}
	job := jobs.items[id]
	if job == nil || job.userID != userID {
		jobs.mu.Unlock()
		return acl.ErrNotFound()
	}
	if job.removing {
		jobs.mu.Unlock()
		return apperr.New(409, "ERR_IMPORT_BUSY", "Import cleanup is in progress")
	}
	if job.status == "running" {
		jobs.mu.Unlock()
		return apperr.New(409, "ERR_IMPORT_RUNNING", "Import is still running")
	}
	job.timer.Stop()
	job.removing = true
	jobs.active.Add(1)
	jobs.mu.Unlock()
	defer jobs.active.Done()
	return jobs.removeJob(job)
}

func (jobs *importJobs) target(id, userID string) (structure.ImportTarget, error) {
	jobs.mu.Lock()
	defer jobs.mu.Unlock()
	job := jobs.items[id]
	if job == nil || job.userID != userID || job.status == "uploading" {
		return structure.ImportTarget{}, acl.ErrNotFound()
	}
	if job.removing {
		return structure.ImportTarget{}, apperr.New(409, "ERR_IMPORT_BUSY", "Import cleanup is in progress")
	}
	return job.target, nil
}

func (jobs *importJobs) start(id, userID string) (*structure.ImportFile, structure.ImportTarget, error) {
	jobs.mu.Lock()
	defer jobs.mu.Unlock()
	if jobs.closed {
		return nil, structure.ImportTarget{}, apperr.New(503, "ERR_IMPORT_BUSY", "Imports are shutting down")
	}
	job := jobs.items[id]
	if job == nil || job.userID != userID {
		return nil, structure.ImportTarget{}, acl.ErrNotFound()
	}
	if job.removing {
		return nil, structure.ImportTarget{}, apperr.New(409, "ERR_IMPORT_BUSY", "Import cleanup is in progress")
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
	jobs.active.Add(1)
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
	jobs.mu.Unlock()
	uploadErr := jobs.removeFiles(id, upload)
	jobs.mu.Lock()
	if uploadErr == nil {
		job.uploadPath = ""
	}
	job.file = nil
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
}

func (jobs *importJobs) status(id, userID string) (importJobStatus, string, error) {
	jobs.mu.Lock()
	defer jobs.mu.Unlock()
	job := jobs.items[id]
	if job == nil || job.userID != userID || job.status == "uploading" {
		return importJobStatus{}, "", acl.ErrNotFound()
	}
	if job.removing {
		return importJobStatus{}, "", apperr.New(409, "ERR_IMPORT_BUSY", "Import cleanup is in progress")
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
	defer api.imports.active.Done()
	ready := false
	defer func() {
		if !ready {
			if err := api.imports.discard(job.id, user.ID); err != nil {
				var app *apperr.Error
				// Shutdown takes ownership of reservations that are still in flight.
				if !errors.As(err, &app) || app.Code != "ERR_IMPORT_BUSY" {
					api.logger.Error("import reservation cleanup failed", "job_id", job.id)
				}
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
	defer api.imports.active.Done()
	ctx, cancel := context.WithTimeout(events.WithID(api.imports.ctx, eventID), importJobTimeout)
	defer cancel()
	if api.attachments != nil {
		file.UseAttachmentStore(api.attachments)
	}
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

// importIssueText localizes an issue reason for the CSV report. Row 0 holds
// workbook notes that do not belong to a data row.
func importIssueText(reason, locale string) string {
	texts := map[string][2]string{
		"":                                    {"Invalid value", "值无效"},
		structure.ImportReasonMetaEdited:      {"Metadata edited outside PandaTable", "元数据已在 PandaTable 之外被修改"},
		structure.ImportReasonMetaInvalid:     {"Invalid metadata ignored", "已忽略无效元数据"},
		structure.ImportReasonFileUnavailable: {"File not available", "文件不可用"},
		structure.ImportReasonLinkUnmatched:   {"Linked record not found", "未找到关联记录"},
	}
	text, ok := texts[reason]
	if !ok {
		return reason
	}
	if locale == "zh-CN" {
		return text[1]
	}
	return text[0]
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
		header := []string{"Row", "Column", "Value", "Issue"}
		if locale == "zh-CN" {
			header = []string{"行", "列", "值", "问题"}
		}
		err = writer.Write(header)
		for _, issue := range issues {
			if err != nil {
				break
			}
			err = writer.Write([]string{strconv.Itoa(issue.Row), issue.Column, issue.Value,
				importIssueText(issue.Reason, locale)})
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
