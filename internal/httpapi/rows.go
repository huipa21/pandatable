package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"pandatable/internal/apperr"
	"pandatable/internal/auth"
	"pandatable/internal/structure"
)

const (
	bulkRowsBodyLimit      = 512 << 10
	guardedRowsBodyLimit   = 2 << 20
	bulkDeleteBodyLimit    = 64 << 10
	restoreRowsBodyLimit   = 8 << 20
	attachmentRowBodyLimit = 64 << 10
	formSubmitBodyLimit    = 64 << 10
)

func (api *authHTTP) submitForm(w http.ResponseWriter, r *http.Request, user auth.User) {
	var input struct {
		Fields json.RawMessage `json:"fields"`
	}
	if err := decodeJSONLimit(w, r, &input, formSubmitBodyLimit); err != nil {
		api.fail(w, r, err)
		return
	}
	if len(input.Fields) == 0 || bytes.Equal(bytes.TrimSpace(input.Fields), []byte("null")) {
		api.fail(w, r, apperr.New(400, "ERR_VALIDATION", "Form fields must be an object"))
		return
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(input.Fields, &fields); err != nil || fields == nil {
		api.fail(w, r, apperr.New(400, "ERR_VALIDATION", "Form fields must be an object"))
		return
	}
	row, err := api.structure.SubmitForm(r.Context(), actorOf(user), chi.URLParam(r, "id"), fields)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, struct {
		Row     structure.RowRecord `json:"row"`
		EventID string              `json:"eventId"`
	}{row, eventID(r)})
}

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

func (api *authHTTP) queryGroups(w http.ResponseWriter, r *http.Request, user auth.User) {
	var input structure.RowGroupRequest
	if err := decodeJSON(w, r, &input); err != nil {
		api.fail(w, r, err)
		return
	}
	result, err := api.structure.QueryGroups(r.Context(), actorOf(user), chi.URLParam(r, "id"), input)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (api *authHTTP) queryAggregates(w http.ResponseWriter, r *http.Request, user auth.User) {
	var input structure.RowAggregateRequest
	if err := decodeJSON(w, r, &input); err != nil {
		api.fail(w, r, err)
		return
	}
	result, err := api.structure.QueryAggregates(r.Context(), actorOf(user), chi.URLParam(r, "id"), input)
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
	}{rows, eventID(r)})
}

func (api *authHTTP) patchRows(w http.ResponseWriter, r *http.Request, user auth.User) {
	api.patchRowsWithLimit(w, r, user, bulkRowsBodyLimit, false)
}

func (api *authHTTP) patchGuardedRows(w http.ResponseWriter, r *http.Request, user auth.User) {
	api.patchRowsWithLimit(w, r, user, guardedRowsBodyLimit, true)
}

func (api *authHTTP) patchRowsWithLimit(w http.ResponseWriter, r *http.Request, user auth.User, limit int64, guarded bool) {
	var input struct {
		Rows []structure.RowPatch `json:"rows"`
	}
	if err := decodeJSONLimit(w, r, &input, limit); err != nil {
		api.fail(w, r, err)
		return
	}
	if guarded {
		for _, row := range input.Rows {
			if row.Expected == nil {
				api.fail(w, r, apperr.New(400, "ERR_VALIDATION", "Expected values are required"))
				return
			}
		}
	}
	rows, err := api.structure.PatchRows(r.Context(), actorOf(user), chi.URLParam(r, "id"), input.Rows)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Rows     []structure.RowRecord `json:"rows"`
		Previous []structure.RowPatch  `json:"previous"`
		EventID  string                `json:"eventId"`
	}{rows, previousRows(rows), eventID(r)})
}

func (api *authHTTP) patchRow(w http.ResponseWriter, r *http.Request, user auth.User) {
	var input struct {
		Fields map[string]json.RawMessage `json:"fields"`
	}
	if err := decodeJSONLimit(w, r, &input, attachmentRowBodyLimit); err != nil {
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
		Row      structure.RowRecord        `json:"row"`
		Previous map[string]json.RawMessage `json:"previous"`
		EventID  string                     `json:"eventId"`
	}{row, row.Previous, eventID(r)})
}

func previousRows(rows []structure.RowRecord) []structure.RowPatch {
	result := make([]structure.RowPatch, len(rows))
	for index, row := range rows {
		result[index] = structure.RowPatch{ID: row.ID, Fields: row.Previous}
	}
	return result
}

func (api *authHTTP) changeLinks(w http.ResponseWriter, r *http.Request, user auth.User) {
	var input struct {
		Add     []string `json:"add"`
		Remove  []string `json:"remove"`
		Replace bool     `json:"replace"`
	}
	if err := decodeJSONLimit(w, r, &input, bulkDeleteBodyLimit); err != nil {
		api.fail(w, r, err)
		return
	}
	row, err := api.structure.ChangeLinks(r.Context(), actorOf(user), chi.URLParam(r, "id"),
		chi.URLParam(r, "rowId"), chi.URLParam(r, "fieldId"),
		structure.LinkChange{Add: input.Add, Remove: input.Remove, Replace: input.Replace})
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Row     structure.RowRecord       `json:"row"`
		Changes []structure.RelationDelta `json:"changes"`
		EventID string                    `json:"eventId"`
	}{row, row.LinkChanges, eventID(r)})
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
	}{rows, eventID(r)})
}

func (api *authHTTP) restoreRows(w http.ResponseWriter, r *http.Request, user auth.User) {
	var input struct {
		Rows []structure.RowRecord `json:"rows"`
	}
	if err := decodeJSONLimit(w, r, &input, restoreRowsBodyLimit); err != nil {
		api.fail(w, r, err)
		return
	}
	rows, err := api.structure.RestoreRows(r.Context(), actorOf(user), chi.URLParam(r, "id"), input.Rows)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Rows    []structure.RowRecord `json:"rows"`
		EventID string                `json:"eventId"`
	}{rows, eventID(r)})
}
