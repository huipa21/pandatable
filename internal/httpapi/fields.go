package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"pandatable/internal/apperr"
	"pandatable/internal/auth"
	"pandatable/internal/structure"
)

// fieldBodyLimit bounds field create/patch bodies so 500-choice options stay
// reachable (D-125); every other route keeps the 4096-byte limit.
const fieldBodyLimit = 512 << 10

type fieldResponse struct {
	structure.FieldResult
	EventID string `json:"eventId"`
}

type relationResponse struct {
	structure.RelationResult
	EventID string `json:"eventId"`
}

func (api *authHTTP) createRelation(w http.ResponseWriter, r *http.Request, user auth.User) {
	var input struct {
		Name          string `json:"name"`
		ReverseName   string `json:"reverseName"`
		TargetTableID string `json:"targetTableId"`
		Kind          string `json:"kind"`
		SourceSide    string `json:"sourceSide"`
		OnDelete      string `json:"onDelete"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		api.fail(w, r, err)
		return
	}
	result, err := api.structure.CreateRelation(r.Context(), actorOf(user), chi.URLParam(r, "id"),
		structure.RelationInput{Name: input.Name, ReverseName: input.ReverseName,
			TargetTableID: input.TargetTableID, Kind: input.Kind, SourceSide: input.SourceSide, OnDelete: input.OnDelete})
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, relationResponse{result, eventID(r)})
}

// ifMatch reads the required resource version: a positive integer, optionally
// quoted as an entity tag.
func ifMatch(r *http.Request) (int32, error) {
	values := r.Header.Values("If-Match")
	invalid := apperr.New(400, "ERR_VALIDATION", "If-Match must carry the resource version")
	if len(values) != 1 {
		return 0, invalid
	}
	text := strings.TrimSpace(values[0])
	if len(text) >= 2 && text[0] == '"' && text[len(text)-1] == '"' {
		text = text[1 : len(text)-1]
	}
	version, err := strconv.ParseInt(text, 10, 32)
	if err != nil || version < 1 || strconv.FormatInt(version, 10) != text {
		return 0, invalid
	}
	return int32(version), nil
}

func (api *authHTTP) createField(w http.ResponseWriter, r *http.Request, user auth.User) {
	var input struct {
		Name        *string         `json:"name"`
		UIType      *string         `json:"uiType"`
		Description *string         `json:"description"`
		Options     json.RawMessage `json:"options"`
		IsRequired  bool            `json:"isRequired"`
		IsUnique    bool            `json:"isUnique"`
		Default     json.RawMessage `json:"default"`
		FillEmpty   bool            `json:"fillEmpty"`
	}
	if err := decodeJSONLimit(w, r, &input, fieldBodyLimit); err != nil {
		api.fail(w, r, err)
		return
	}
	if input.Name == nil || input.UIType == nil {
		api.fail(w, r, apperr.New(400, "ERR_VALIDATION", "Field name and type are required"))
		return
	}
	description := ""
	if input.Description != nil {
		description = *input.Description
	}
	result, err := api.structure.CreateField(r.Context(), actorOf(user), chi.URLParam(r, "id"), structure.FieldInput{
		Name: *input.Name, UIType: *input.UIType, Description: description, Options: input.Options,
		IsRequired: input.IsRequired, IsUnique: input.IsUnique, Default: input.Default, FillEmpty: input.FillEmpty})
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, fieldResponse{result, eventID(r)})
}

func (api *authHTTP) updateField(w http.ResponseWriter, r *http.Request, user auth.User) {
	version, err := ifMatch(r)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	var input struct {
		Name        *string         `json:"name"`
		Description *string         `json:"description"`
		Options     json.RawMessage `json:"options"`
		IsRequired  *bool           `json:"isRequired"`
		IsUnique    *bool           `json:"isUnique"`
		Default     json.RawMessage `json:"default"`
		Index       *int            `json:"index"`
		FillEmpty   bool            `json:"fillEmpty"`
	}
	if err := decodeJSONLimit(w, r, &input, fieldBodyLimit); err != nil {
		api.fail(w, r, err)
		return
	}
	result, err := api.structure.UpdateField(r.Context(), actorOf(user), chi.URLParam(r, "id"), structure.FieldPatch{
		Version: version, Name: input.Name, Description: input.Description, Options: input.Options,
		IsRequired: input.IsRequired, IsUnique: input.IsUnique, Default: input.Default, Index: input.Index,
		FillEmpty: input.FillEmpty})
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, fieldResponse{result, eventID(r)})
}

func (api *authHTTP) dryRunFieldType(w http.ResponseWriter, r *http.Request, user auth.User) {
	version, err := ifMatch(r)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	var input struct {
		UIType  string          `json:"uiType"`
		Options json.RawMessage `json:"options"`
	}
	if err := decodeJSONLimit(w, r, &input, fieldBodyLimit); err != nil {
		api.fail(w, r, err)
		return
	}
	result, err := api.structure.DryRunTypeChange(r.Context(), actorOf(user), chi.URLParam(r, "id"), version,
		structure.TypeChangeTarget{UIType: input.UIType, Options: input.Options})
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (api *authHTTP) convertFieldType(w http.ResponseWriter, r *http.Request, user auth.User) {
	version, err := ifMatch(r)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	var input struct {
		UIType           string          `json:"uiType"`
		Options          json.RawMessage `json:"options"`
		Default          json.RawMessage `json:"default"`
		SetFailingToNull bool            `json:"setFailingToNull"`
	}
	if err := decodeJSONLimit(w, r, &input, fieldBodyLimit); err != nil {
		api.fail(w, r, err)
		return
	}
	result, err := api.structure.ConvertFieldType(r.Context(), actorOf(user), chi.URLParam(r, "id"), version,
		structure.TypeChangeTarget{UIType: input.UIType, Options: input.Options, Default: input.Default,
			SetFailingToNull: input.SetFailingToNull})
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, fieldResponse{result, eventID(r)})
}

func (api *authHTTP) deleteField(w http.ResponseWriter, r *http.Request, user auth.User) {
	version, err := ifMatch(r)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	result, err := api.structure.DeleteField(r.Context(), actorOf(user), chi.URLParam(r, "id"), version)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, fieldResponse{result, eventID(r)})
}
