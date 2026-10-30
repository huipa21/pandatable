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

func (api *authHTTP) createView(w http.ResponseWriter, r *http.Request, user auth.User) {
	var input structure.ViewInput
	if err := decodeJSON(w, r, &input); err != nil {
		api.fail(w, r, err)
		return
	}
	view, err := api.structure.CreateView(r.Context(), actorOf(user), chi.URLParam(r, "id"), input)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, struct {
		View    structure.View `json:"view"`
		EventID string         `json:"eventId"`
	}{view, rand.Text()})
}

func (api *authHTTP) duplicateView(w http.ResponseWriter, r *http.Request, user auth.User) {
	var input struct {
		Name   string          `json:"name"`
		Scope  string          `json:"scope"`
		Config json.RawMessage `json:"config"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		api.fail(w, r, err)
		return
	}
	var config *structure.ViewColumns
	if len(input.Config) > 0 {
		parsed, err := decodeViewConfig(input.Config)
		if err != nil {
			api.fail(w, r, err)
			return
		}
		config = &parsed
	}
	view, err := api.structure.DuplicateView(r.Context(), actorOf(user), chi.URLParam(r, "id"),
		input.Name, input.Scope, config)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, struct {
		View    structure.View `json:"view"`
		EventID string         `json:"eventId"`
	}{view, rand.Text()})
}

func (api *authHTTP) updateView(w http.ResponseWriter, r *http.Request, user auth.User) {
	version, err := ifMatch(r)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	var input struct {
		Name   *string         `json:"name"`
		Locked *bool           `json:"locked"`
		Config json.RawMessage `json:"config"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		api.fail(w, r, err)
		return
	}
	if len(input.Config) == 0 {
		view, err := api.structure.UpdateViewMetadata(r.Context(), actorOf(user), chi.URLParam(r, "id"), version,
			structure.ViewPatch{Name: input.Name, Locked: input.Locked})
		if err != nil {
			api.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, struct {
			View    structure.View `json:"view"`
			EventID string         `json:"eventId"`
		}{view, rand.Text()})
		return
	}
	if input.Name != nil || input.Locked != nil {
		api.fail(w, r, apperr.New(400, "ERR_VALIDATION", "Update either view metadata or config at a time"))
		return
	}
	config, err := decodeViewConfig(input.Config)
	if err != nil {
		api.fail(w, r, err)
		return
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

func (api *authHTTP) deleteView(w http.ResponseWriter, r *http.Request, user auth.User) {
	version, err := ifMatch(r)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	if err := api.structure.DeleteView(r.Context(), actorOf(user), chi.URLParam(r, "id"), version); err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"eventId": rand.Text()})
}

func (api *authHTTP) moveView(w http.ResponseWriter, r *http.Request, user auth.User) {
	version, err := ifMatch(r)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	var input struct {
		BeforeID *string `json:"beforeId"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		api.fail(w, r, err)
		return
	}
	view, err := api.structure.MoveView(r.Context(), actorOf(user), chi.URLParam(r, "id"), version, input.BeforeID)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		View    structure.View `json:"view"`
		EventID string         `json:"eventId"`
	}{view, rand.Text()})
}

func decodeViewConfig(raw json.RawMessage) (structure.ViewColumns, error) {
	invalid := apperr.New(400, "ERR_VALIDATION", "View configuration is invalid")
	var input struct {
		Fields *[]struct {
			ID      *string         `json:"id"`
			Visible *bool           `json:"visible"`
			Width   json.RawMessage `json:"width"`
		} `json:"fields"`
		Frozen     *int            `json:"frozen"`
		Filter     json.RawMessage `json:"filter"`
		RowHeight  json.RawMessage `json:"rowHeight"`
		Sort       json.RawMessage `json:"sort"`
		GroupBy    json.RawMessage `json:"groupBy"`
		Aggregates json.RawMessage `json:"aggregates"`
		Highlight  json.RawMessage `json:"highlight"`
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return structure.ViewColumns{}, invalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || input.Fields == nil || input.Frozen == nil {
		return structure.ViewColumns{}, invalid
	}
	config := structure.ViewColumns{
		Fields:     make([]structure.ViewColumn, 0, len(*input.Fields)),
		Frozen:     *input.Frozen,
		Filter:     input.Filter,
		FilterSet:  len(input.Filter) > 0,
		Sort:       []structure.ViewSort{},
		GroupBy:    []string{},
		Aggregates: []structure.ViewAggregate{},
		Highlight:  []structure.HighlightRule{},
	}
	if len(input.RowHeight) > 0 {
		if bytes.Equal(bytes.TrimSpace(input.RowHeight), []byte("null")) ||
			json.Unmarshal(input.RowHeight, &config.RowHeight) != nil {
			return structure.ViewColumns{}, invalid
		}
	}
	if len(input.GroupBy) > 0 {
		if bytes.Equal(bytes.TrimSpace(input.GroupBy), []byte("null")) ||
			json.Unmarshal(input.GroupBy, &config.GroupBy) != nil || config.GroupBy == nil {
			return structure.ViewColumns{}, invalid
		}
	}
	for _, item := range []struct {
		raw    json.RawMessage
		target any
	}{
		{input.Sort, &config.Sort},
		{input.Aggregates, &config.Aggregates},
		{input.Highlight, &config.Highlight},
	} {
		if len(item.raw) == 0 {
			continue
		}
		if bytes.Equal(bytes.TrimSpace(item.raw), []byte("null")) {
			return structure.ViewColumns{}, invalid
		}
		nested := json.NewDecoder(bytes.NewReader(item.raw))
		nested.DisallowUnknownFields()
		if err := nested.Decode(item.target); err != nil {
			return structure.ViewColumns{}, invalid
		}
	}
	if config.FilterSet && !bytes.Equal(bytes.TrimSpace(config.Filter), []byte("null")) {
		var filter structure.FilterNode
		nested := json.NewDecoder(bytes.NewReader(config.Filter))
		nested.DisallowUnknownFields()
		if err := nested.Decode(&filter); err != nil {
			return structure.ViewColumns{}, invalid
		}
	}
	for _, column := range *input.Fields {
		if column.ID == nil || column.Visible == nil {
			return structure.ViewColumns{}, invalid
		}
		var width *int
		if len(column.Width) > 0 {
			var value int
			if err := json.Unmarshal(column.Width, &value); err != nil ||
				value < structure.MinColumnWidth || value > structure.MaxColumnWidth {
				return structure.ViewColumns{}, invalid
			}
			width = &value
		}
		config.Fields = append(config.Fields, structure.ViewColumn{
			ID: *column.ID, Visible: *column.Visible, Width: width,
		})
	}
	return config, nil
}
