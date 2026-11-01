package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"pandatable/internal/apperr"
	"pandatable/internal/auth"
)

func (api *authHTTP) listUserOwnerships(w http.ResponseWriter, r *http.Request, _ auth.User, token string) {
	input, err := groupQuery(r)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	result, err := api.admin.Ownerships(r.Context(), token, chi.URLParam(r, "id"), input)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (api *authHTTP) transferUserOwnerships(w http.ResponseWriter, r *http.Request, _ auth.User, token string) {
	var input struct {
		UserID string `json:"userId"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		api.fail(w, r, err)
		return
	}
	count, err := api.admin.TransferOwnerships(r.Context(), token, chi.URLParam(r, "id"), input.UserID)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Transferred int64  `json:"transferred"`
		EventID     string `json:"eventId"`
	}{count, eventID(r)})
}

func (api *authHTTP) rescueWorkspace(w http.ResponseWriter, r *http.Request, _ auth.User, token string) {
	var input struct {
		UserID string `json:"userId"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		api.fail(w, r, err)
		return
	}
	if err := api.admin.RescueWorkspace(r.Context(), token, chi.URLParam(r, "id"), input.UserID); err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"eventId": eventID(r)})
}

func (api *authHTTP) deletionCleanupAvailable(w http.ResponseWriter, r *http.Request) bool {
	if api.structure == nil || api.attachments == nil {
		api.fail(w, r, apperr.New(503, "ERR_LIFECYCLE_UNAVAILABLE", "User deletion cleanup is unavailable"))
		return false
	}
	return true
}

func (api *authHTTP) cleanupFailure(w http.ResponseWriter, r *http.Request) {
	api.logger.ErrorContext(r.Context(), "user deletion attachment cleanup failed",
		"request_id", middleware.GetReqID(r.Context()))
	api.fail(w, r, apperr.New(503, "ERR_USER_CLEANUP_PENDING", "User deletion committed but attachment cleanup remains"))
}

func (api *authHTTP) hardDeleteUser(w http.ResponseWriter, r *http.Request, _ auth.User, token string) {
	if !api.deletionCleanupAvailable(w, r) {
		return
	}
	var input struct {
		Username string `json:"username"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		api.fail(w, r, err)
		return
	}
	id := chi.URLParam(r, "id")
	if err := api.admin.HardDeleteUser(r.Context(), token, id, input.Username); err != nil {
		api.fail(w, r, err)
		return
	}
	if err := api.structure.DrainUserDeletionFiles(r.Context(), api.attachments, id); err != nil {
		api.cleanupFailure(w, r)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"eventId": eventID(r)})
}

func (api *authHTTP) pendingUserDeletionCleanup(w http.ResponseWriter, r *http.Request, _ auth.User, _ string) {
	if !api.deletionCleanupAvailable(w, r) {
		return
	}
	count, err := api.structure.PendingUserDeletionFiles(r.Context())
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Pending int64 `json:"pending"`
	}{count})
}

func (api *authHTTP) retryUserDeletionCleanup(w http.ResponseWriter, r *http.Request, _ auth.User, _ string) {
	if !api.deletionCleanupAvailable(w, r) {
		return
	}
	if err := api.structure.DrainUserDeletionFiles(r.Context(), api.attachments, ""); err != nil {
		api.cleanupFailure(w, r)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"eventId": eventID(r)})
}
