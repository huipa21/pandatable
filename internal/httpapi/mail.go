package httpapi

import (
	"encoding/json"
	"net/http"

	"pandatable/internal/admin"
	"pandatable/internal/apperr"
	"pandatable/internal/auth"
)

func (api *authHTTP) resetRequestIP(w http.ResponseWriter, r *http.Request) bool {
	ip, err := clientIP(r, api.proxies)
	if err != nil {
		api.fail(w, r, apperr.New(400, "ERR_VALIDATION", "Invalid client IP chain"))
		return false
	}
	if err := api.service.CheckResetIP(ip); err != nil {
		api.fail(w, r, err)
		return false
	}
	return true
}

func (api *authHTTP) requestPasswordReset(w http.ResponseWriter, r *http.Request) {
	if !api.resetRequestIP(w, r) {
		return
	}
	if err := noQuery(r); err != nil {
		api.fail(w, r, err)
		return
	}
	var input struct {
		Email string `json:"email"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		api.fail(w, r, err)
		return
	}
	email, err := auth.NormalizeEmail(input.Email)
	if err == nil {
		err = api.service.CheckResetEmail(email)
	}
	if err != nil {
		api.fail(w, r, err)
		return
	}
	api.mailer.QueueReset(email)
	writeJSON(w, http.StatusOK, struct {
		Accepted bool `json:"accepted"`
	}{true})
}

func (api *authHTTP) completePasswordReset(w http.ResponseWriter, r *http.Request) {
	if !api.resetRequestIP(w, r) {
		return
	}
	if err := noQuery(r); err != nil {
		api.fail(w, r, err)
		return
	}
	var input struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		api.fail(w, r, err)
		return
	}
	if err := api.service.CompletePasswordReset(r.Context(), input.Token, input.Password); err != nil {
		api.fail(w, r, err)
		return
	}
	api.clearCookie(w)
	writeJSON(w, http.StatusOK, map[string]string{"eventId": eventID(r)})
}

func (api *authHTTP) getSMTP(w http.ResponseWriter, r *http.Request, _ auth.User, token string) {
	if err := noQuery(r); err != nil {
		api.fail(w, r, err)
		return
	}
	result, err := api.admin.SMTPSettings(r.Context(), token)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (api *authHTTP) updateSMTP(w http.ResponseWriter, r *http.Request, _ auth.User, token string) {
	if err := noQuery(r); err != nil {
		api.fail(w, r, err)
		return
	}
	var input struct {
		Enabled     json.RawMessage `json:"enabled"`
		Host        json.RawMessage `json:"host"`
		Port        json.RawMessage `json:"port"`
		Security    json.RawMessage `json:"security"`
		Username    json.RawMessage `json:"username"`
		Password    json.RawMessage `json:"password"`
		FromAddress json.RawMessage `json:"fromAddress"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		api.fail(w, r, err)
		return
	}
	var patch admin.SMTPPatch
	for _, field := range []struct {
		raw  json.RawMessage
		dest **string
	}{
		{input.Host, &patch.Host}, {input.Security, &patch.Security},
		{input.Username, &patch.Username}, {input.Password, &patch.Password},
		{input.FromAddress, &patch.FromAddress},
	} {
		value, err := patchString(field.raw, false)
		if err != nil {
			api.fail(w, r, err)
			return
		}
		*field.dest = value
	}
	if len(input.Enabled) != 0 {
		var value bool
		if string(input.Enabled) == "null" || json.Unmarshal(input.Enabled, &value) != nil {
			api.fail(w, r, apperr.New(400, "ERR_VALIDATION", "Expected a boolean setting"))
			return
		}
		patch.Enabled = &value
	}
	if len(input.Port) != 0 {
		var value int
		if string(input.Port) == "null" || json.Unmarshal(input.Port, &value) != nil {
			api.fail(w, r, apperr.New(400, "ERR_VALIDATION", "Expected an integer setting"))
			return
		}
		patch.Port = &value
	}
	result, err := api.admin.UpdateSMTPSettings(r.Context(), token, patch)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Settings admin.SMTPSettings `json:"settings"`
		EventID  string             `json:"eventId"`
	}{result, eventID(r)})
}

func (api *authHTTP) testSMTP(w http.ResponseWriter, r *http.Request, user auth.User, token string) {
	if err := noQuery(r); err != nil {
		api.fail(w, r, err)
		return
	}
	var input struct {
		To string `json:"to"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		api.fail(w, r, err)
		return
	}
	to, err := auth.NormalizeEmail(input.To)
	if err == nil {
		_, err = api.admin.SMTPSettings(r.Context(), token)
	}
	if err == nil {
		err = api.mailer.Test(r.Context(), to, user.Locale)
	}
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"eventId": eventID(r)})
}
