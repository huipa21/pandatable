package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"pandatable/internal/auth"
)

func (api *authHTTP) listRowHistory(w http.ResponseWriter, r *http.Request, user auth.User) {
	offset, _, err := commentPageQuery(r, false)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	result, err := api.structure.ListRowHistory(r.Context(), actorOf(user), chi.URLParam(r, "id"),
		chi.URLParam(r, "rowId"), offset)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
