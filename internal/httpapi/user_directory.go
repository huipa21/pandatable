package httpapi

import (
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"

	"pandatable/internal/acl"
	"pandatable/internal/auth"
)

func (api *authHTTP) tableUsers(w http.ResponseWriter, r *http.Request, user auth.User) {
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(query) != 1 || len(query["q"]) != 1 {
		api.fail(w, r, acl.ErrValidation())
		return
	}
	listing, err := api.acl.TableUsers(r.Context(), user.ID, chi.URLParam(r, "id"), query.Get("q"))
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listing)
}

func (api *authHTTP) tableUserLabels(w http.ResponseWriter, r *http.Request, user auth.User) {
	var input struct {
		IDs []string `json:"ids"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		api.fail(w, r, err)
		return
	}
	labels, err := api.acl.TableUserLabels(r.Context(), user.ID, chi.URLParam(r, "id"), input.IDs)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Items []acl.UserLabel `json:"items"`
	}{Items: labels})
}
