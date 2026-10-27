package httpapi

import (
	"crypto/rand"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"pandatable/internal/apperr"
	"pandatable/internal/auth"
	"pandatable/internal/structure"
)

func (api *authHTTP) updateViewColumns(w http.ResponseWriter, r *http.Request, user auth.User) {
	version, err := ifMatch(r)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	var input struct {
		Config *struct {
			Fields *[]struct {
				ID      *string         `json:"id"`
				Visible *bool           `json:"visible"`
				Width   json.RawMessage `json:"width"`
			} `json:"fields"`
			Frozen *int `json:"frozen"`
		} `json:"config"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		api.fail(w, r, err)
		return
	}
	invalid := apperr.New(400, "ERR_VALIDATION", "Column fields, visibility and frozen count are required")
	if input.Config == nil || input.Config.Fields == nil || input.Config.Frozen == nil {
		api.fail(w, r, invalid)
		return
	}
	config := structure.ViewColumns{Fields: make([]structure.ViewColumn, 0, len(*input.Config.Fields)), Frozen: *input.Config.Frozen}
	for _, column := range *input.Config.Fields {
		if column.ID == nil || column.Visible == nil {
			api.fail(w, r, invalid)
			return
		}
		var width *int
		if column.Width != nil {
			var value int
			if err := json.Unmarshal(column.Width, &value); err != nil || value < structure.MinColumnWidth || value > structure.MaxColumnWidth {
				api.fail(w, r, invalid)
				return
			}
			width = &value
		}
		config.Fields = append(config.Fields, structure.ViewColumn{ID: *column.ID, Visible: *column.Visible, Width: width})
	}
	view, err := api.structure.UpdateViewColumns(r.Context(), actorOf(user), chi.URLParam(r, "id"), version, config)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		View    structure.View `json:"view"`
		EventID string         `json:"eventId"`
	}{view, rand.Text()})
}
