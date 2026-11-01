package httpapi

import (
	"errors"
	"fmt"
	"mime"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"pandatable/internal/auth"
)

const (
	backupPreflightTimeout = 2 * time.Minute
	backupDownloadTimeout  = 30 * time.Minute
)

func (api *authHTTP) listBackups(w http.ResponseWriter, r *http.Request, _ auth.User, _ string) {
	if err := noQuery(r); err != nil {
		api.fail(w, r, err)
		return
	}
	listing, err := api.backups.List(r.Context())
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listing)
}

func (api *authHTTP) createBackup(w http.ResponseWriter, r *http.Request, user auth.User, _ string) {
	if err := noQuery(r); err != nil {
		api.fail(w, r, err)
		return
	}
	var input struct{}
	if err := decodeJSON(w, r, &input); err != nil {
		api.fail(w, r, err)
		return
	}
	record, err := api.backups.Start(r.Context(), user.ID)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, record)
}

func (api *authHTTP) getBackup(w http.ResponseWriter, r *http.Request, _ auth.User, _ string) {
	if err := noQuery(r); err != nil {
		api.fail(w, r, err)
		return
	}
	record, err := api.backups.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (api *authHTTP) downloadBackup(w http.ResponseWriter, r *http.Request, _ auth.User, _ string) {
	if err := noQuery(r); err != nil {
		api.fail(w, r, err)
		return
	}
	file, record, err := api.backups.Open(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		api.fail(w, r, err)
		return
	}
	defer func() {
		if err := file.Close(); err != nil {
			api.logger.ErrorContext(r.Context(), "backup download close failed", "backup_id", record.ID)
		}
	}()
	info, err := file.Stat()
	if err != nil {
		api.fail(w, r, fmt.Errorf("stat backup archive: %w", err))
		return
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": record.FileName})
	if disposition == "" {
		api.fail(w, r, errors.New("backup filename cannot be encoded"))
		return
	}
	if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(backupDownloadTimeout)); err != nil &&
		!errors.Is(err, http.ErrNotSupported) {
		api.fail(w, r, fmt.Errorf("set backup download deadline: %w", err))
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", disposition)
	http.ServeContent(w, r, record.FileName, info.ModTime(), file)
}

func (api *authHTTP) deleteBackup(w http.ResponseWriter, r *http.Request, _ auth.User, _ string) {
	if err := noQuery(r); err != nil {
		api.fail(w, r, err)
		return
	}
	if err := api.backups.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"eventId": eventID(r)})
}
