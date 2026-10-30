package httpapi

import (
	"bytes"
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
			Frozen     *int            `json:"frozen"`
			Sort       json.RawMessage `json:"sort"`
			GroupBy    json.RawMessage `json:"groupBy"`
			Aggregates json.RawMessage `json:"aggregates"`
			Highlight  json.RawMessage `json:"highlight"`
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
	var groupBy []string
	if len(input.Config.GroupBy) > 0 {
		if bytes.Equal(bytes.TrimSpace(input.Config.GroupBy), []byte("null")) ||
			json.Unmarshal(input.Config.GroupBy, &groupBy) != nil || groupBy == nil {
			api.fail(w, r, invalid)
			return
		}
	}
	var aggregates []structure.ViewAggregate
	if len(input.Config.Aggregates) > 0 {
		if bytes.Equal(bytes.TrimSpace(input.Config.Aggregates), []byte("null")) {
			api.fail(w, r, invalid)
			return
		}
		decoder := json.NewDecoder(bytes.NewReader(input.Config.Aggregates))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&aggregates); err != nil || aggregates == nil {
			api.fail(w, r, invalid)
			return
		}
	}
	var sort []structure.ViewSort
	if len(input.Config.Sort) > 0 {
		if bytes.Equal(bytes.TrimSpace(input.Config.Sort), []byte("null")) {
			api.fail(w, r, invalid)
			return
		}
		decoder := json.NewDecoder(bytes.NewReader(input.Config.Sort))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&sort); err != nil || sort == nil {
			api.fail(w, r, invalid)
			return
		}
	}
	var highlight []structure.HighlightRule
	if len(input.Config.Highlight) > 0 {
		if bytes.Equal(bytes.TrimSpace(input.Config.Highlight), []byte("null")) {
			api.fail(w, r, invalid)
			return
		}
		decoder := json.NewDecoder(bytes.NewReader(input.Config.Highlight))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&highlight); err != nil || highlight == nil {
			api.fail(w, r, invalid)
			return
		}
	}
	config := structure.ViewColumns{
		Fields:     make([]structure.ViewColumn, 0, len(*input.Config.Fields)),
		Frozen:     *input.Config.Frozen,
		Sort:       sort,
		GroupBy:    groupBy,
		Aggregates: aggregates,
		Highlight:  highlight,
	}
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
