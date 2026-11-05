package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"

	"pandatable/internal/acl"
	"pandatable/internal/auth"
	"pandatable/internal/structure"
)

type workspacePreferenceInput struct {
	WorkspaceID string
}

func (input *workspacePreferenceInput) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') || !decoder.More() {
		return errors.New("expected a workspace preference object")
	}
	key, err := decoder.Token()
	if err != nil || key != "workspaceId" {
		return errors.New("expected workspaceId")
	}
	var value json.RawMessage
	if err := decoder.Decode(&value); err != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		return errors.New("expected a workspace ID")
	}
	if err := json.Unmarshal(value, &input.WorkspaceID); err != nil || decoder.More() {
		return errors.New("invalid workspace preference")
	}
	if _, err := decoder.Token(); err != nil {
		return err
	}
	return nil
}

func (api *authHTTP) getWorkspacePreference(w http.ResponseWriter, r *http.Request, user auth.User) {
	if r.URL.RawQuery != "" {
		api.fail(w, r, acl.ErrValidation())
		return
	}
	result, err := api.structure.WorkspacePreference(r.Context(), actorOf(user))
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (api *authHTTP) resolveWorkspacePreference(w http.ResponseWriter, r *http.Request, user auth.User) {
	if r.URL.RawQuery != "" {
		api.fail(w, r, acl.ErrValidation())
		return
	}
	var input map[string]json.RawMessage
	if err := decodeJSON(w, r, &input); err != nil {
		api.fail(w, r, err)
		return
	}
	if input == nil || len(input) != 0 {
		api.fail(w, r, acl.ErrValidation())
		return
	}
	result, err := api.structure.ResolveWorkspacePreference(r.Context(), actorOf(user))
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeWorkspacePreferenceMutation(w, r, result)
}

func (api *authHTTP) setWorkspacePreference(w http.ResponseWriter, r *http.Request, user auth.User) {
	if r.URL.RawQuery != "" {
		api.fail(w, r, acl.ErrValidation())
		return
	}
	var input workspacePreferenceInput
	if err := decodeJSON(w, r, &input); err != nil {
		api.fail(w, r, err)
		return
	}
	result, err := api.structure.SetWorkspacePreference(r.Context(), actorOf(user), input.WorkspaceID)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeWorkspacePreferenceMutation(w, r, result)
}

func writeWorkspacePreferenceMutation(w http.ResponseWriter, r *http.Request, preference structure.WorkspacePreference) {
	writeJSON(w, http.StatusOK, struct {
		structure.WorkspacePreference
		EventID string `json:"eventId"`
	}{preference, eventID(r)})
}
