package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"pandatable/internal/apperr"
	"pandatable/internal/auth"
	"pandatable/internal/structure"
)

func (api *authHTTP) listTrash(w http.ResponseWriter, r *http.Request, user auth.User) {
	if r.URL.RawQuery != "" {
		api.fail(w, r, apperr.New(400, "ERR_VALIDATION", "Trash does not accept query parameters"))
		return
	}
	result, err := api.structure.ListTrash(r.Context(), actorOf(user))
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (api *authHTTP) restoreTrash(w http.ResponseWriter, r *http.Request, user auth.User) {
	var input structure.TrashRestoreOptions
	if err := decodeJSON(w, r, &input); err != nil {
		api.fail(w, r, err)
		return
	}
	if err := api.structure.RestoreTrash(r.Context(), actorOf(user),
		chi.URLParam(r, "kind"), chi.URLParam(r, "id"), input); err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"eventId": eventID(r)})
}
