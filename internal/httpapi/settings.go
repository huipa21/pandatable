package httpapi

import (
	"crypto/rand"
	"encoding/json"
	"net/http"

	"pandatable/internal/admin"
	"pandatable/internal/apperr"
	"pandatable/internal/auth"
)

func noQuery(r *http.Request) error {
	if r.URL.RawQuery != "" {
		return apperr.New(400, "ERR_VALIDATION", "This endpoint does not accept query parameters")
	}
	return nil
}

func (api *authHTTP) appSettings(w http.ResponseWriter, r *http.Request) {
	if err := noQuery(r); err != nil {
		api.fail(w, r, err)
		return
	}
	result, err := api.admin.AppSettings(r.Context())
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, 200, result)
}

func (api *authHTTP) currencies(w http.ResponseWriter, r *http.Request) {
	session, token, ok := api.session(w, r, false)
	if !ok {
		return
	}
	if err := noQuery(r); err != nil {
		api.fail(w, r, err)
		return
	}
	api.cookie(w, token, session.ExpiresAt)
	writeJSON(w, 200, struct {
		Items []string `json:"items"`
	}{admin.Currencies()})
}

func (api *authHTTP) getSettings(w http.ResponseWriter, r *http.Request, _ auth.User, token string) {
	if err := noQuery(r); err != nil {
		api.fail(w, r, err)
		return
	}
	result, err := api.admin.Settings(r.Context(), token)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, 200, result)
}

func (api *authHTTP) updateSettings(w http.ResponseWriter, r *http.Request, _ auth.User, token string) {
	if err := noQuery(r); err != nil {
		api.fail(w, r, err)
		return
	}
	var input struct {
		AppName              json.RawMessage `json:"appName"`
		DefaultTimezone      json.RawMessage `json:"defaultTimezone"`
		DefaultCurrency      json.RawMessage `json:"defaultCurrency"`
		DefaultLocale        json.RawMessage `json:"defaultLocale"`
		UploadMaxMb          json.RawMessage `json:"uploadMaxMb"`
		HistoryRetentionDays json.RawMessage `json:"historyRetentionDays"`
		TrashRetentionDays   json.RawMessage `json:"trashRetentionDays"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		api.fail(w, r, err)
		return
	}
	var patch admin.SettingsPatch
	for _, field := range []struct {
		raw  json.RawMessage
		dest **string
	}{{input.AppName, &patch.AppName}, {input.DefaultTimezone, &patch.DefaultTimezone},
		{input.DefaultCurrency, &patch.DefaultCurrency}, {input.DefaultLocale, &patch.DefaultLocale}} {
		value, err := patchString(field.raw, false)
		if err != nil {
			api.fail(w, r, err)
			return
		}
		*field.dest = value
	}
	for _, field := range []struct {
		raw  json.RawMessage
		dest **int
	}{{input.UploadMaxMb, &patch.UploadMaxMb}, {input.HistoryRetentionDays, &patch.HistoryRetentionDays},
		{input.TrashRetentionDays, &patch.TrashRetentionDays}} {
		if len(field.raw) != 0 {
			var value int
			if string(field.raw) == "null" || json.Unmarshal(field.raw, &value) != nil {
				api.fail(w, r, apperr.New(400, "ERR_VALIDATION", "Expected an integer setting"))
				return
			}
			*field.dest = &value
		}
	}
	result, err := api.admin.UpdateSettings(r.Context(), token, patch)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, 200, struct {
		Settings admin.Settings `json:"settings"`
		EventID  string         `json:"eventId"`
	}{result, rand.Text()})
}

func (api *authHTTP) listWorkspaces(w http.ResponseWriter, r *http.Request, _ auth.User, token string) {
	input, err := groupQuery(r)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	result, err := api.admin.Workspaces(r.Context(), token, input)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, 200, result)
}
