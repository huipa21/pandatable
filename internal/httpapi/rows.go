package httpapi

import (
	"crypto/rand"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"pandatable/internal/auth"
	"pandatable/internal/structure"
)

const (
	bulkRowsBodyLimit   = 512 << 10
	bulkDeleteBodyLimit = 64 << 10
)

func (api *authHTTP) queryRows(w http.ResponseWriter, r *http.Request, user auth.User) {
	var input structure.RowQueryRequest
	if err := decodeJSON(w, r, &input); err != nil {
		api.fail(w, r, err)
		return
	}
	result, err := api.structure.QueryRows(r.Context(), actorOf(user), chi.URLParam(r, "id"), input)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (api *authHTTP) getRow(w http.ResponseWriter, r *http.Request, user auth.User) {
	row, err := api.structure.GetRow(r.Context(), actorOf(user), chi.URLParam(r, "id"), chi.URLParam(r, "rowId"))
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (api *authHTTP) createRows(w http.ResponseWriter, r *http.Request, user auth.User) {
	var input struct {
		Rows []structure.RowInsert `json:"rows"`
	}
	if err := decodeJSONLimit(w, r, &input, bulkRowsBodyLimit); err != nil {
		api.fail(w, r, err)
		return
	}
	rows, err := api.structure.CreateRows(r.Context(), actorOf(user), chi.URLParam(r, "id"), input.Rows)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, struct {
		Rows    []structure.RowRecord `json:"rows"`
		EventID string                `json:"eventId"`
	}{rows, rand.Text()})
}

func (api *authHTTP) patchRows(w http.ResponseWriter, r *http.Request, user auth.User) {
	var input struct {
		Rows []structure.RowPatch `json:"rows"`
	}
	if err := decodeJSONLimit(w, r, &input, bulkRowsBodyLimit); err != nil {
		api.fail(w, r, err)
		return
	}
	rows, err := api.structure.PatchRows(r.Context(), actorOf(user), chi.URLParam(r, "id"), input.Rows)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Rows    []structure.RowRecord `json:"rows"`
		EventID string                `json:"eventId"`
	}{rows, rand.Text()})
}

func (api *authHTTP) patchRow(w http.ResponseWriter, r *http.Request, user auth.User) {
	var input struct {
		Fields map[string]json.RawMessage `json:"fields"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		api.fail(w, r, err)
		return
	}
	row, err := api.structure.PatchRow(r.Context(), actorOf(user), chi.URLParam(r, "id"),
		chi.URLParam(r, "rowId"), input.Fields)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Row     structure.RowRecord `json:"row"`
		EventID string              `json:"eventId"`
	}{row, rand.Text()})
}

func (api *authHTTP) deleteRows(w http.ResponseWriter, r *http.Request, user auth.User) {
	var input struct {
		IDs []string `json:"ids"`
	}
	if err := decodeJSONLimit(w, r, &input, bulkDeleteBodyLimit); err != nil {
		api.fail(w, r, err)
		return
	}
	rows, err := api.structure.DeleteRows(r.Context(), actorOf(user), chi.URLParam(r, "id"), input.IDs)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Rows    []structure.RowRecord `json:"rows"`
		EventID string                `json:"eventId"`
	}{rows, rand.Text()})
}
