package httpapi

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"

	"pandatable/internal/admin"
	"pandatable/internal/apperr"
	"pandatable/internal/auth"
)

func groupQuery(r *http.Request) (admin.ListInput, error) {
	input := admin.ListInput{Limit: 50}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return input, apperr.New(400, "ERR_VALIDATION", "Malformed group query")
	}
	for name, values := range query {
		if len(values) != 1 {
			return input, apperr.New(400, "ERR_VALIDATION", "Duplicate group parameter")
		}
		switch name {
		case "q":
			input.Q = values[0]
		case "offset", "limit":
			value, err := strconv.ParseInt(values[0], 10, 32)
			if err != nil {
				return input, apperr.New(400, "ERR_VALIDATION", "Invalid group pagination")
			}
			if name == "offset" {
				input.Offset = int32(value)
			} else {
				input.Limit = int32(value)
			}
		default:
			return input, apperr.New(400, "ERR_VALIDATION", "Unknown group parameter")
		}
	}
	return input, nil
}

func (api *authHTTP) groupList(w http.ResponseWriter, r *http.Request, token, userID string, members bool) {
	input, err := groupQuery(r)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	var result any
	if members {
		result, err = api.admin.Members(r.Context(), token, chi.URLParam(r, "id"), input)
	} else {
		result, err = api.admin.Groups(r.Context(), token, userID, input)
	}
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, 200, result)
}

func (api *authHTTP) listGroups(w http.ResponseWriter, r *http.Request, _ auth.User, token string) {
	api.groupList(w, r, token, "", false)
}

func (api *authHTTP) listUserGroups(w http.ResponseWriter, r *http.Request, _ auth.User, token string) {
	api.groupList(w, r, token, chi.URLParam(r, "id"), false)
}

func (api *authHTTP) listGroupMembers(w http.ResponseWriter, r *http.Request, _ auth.User, token string) {
	api.groupList(w, r, token, "", true)
}

func (api *authHTTP) getManagedUser(w http.ResponseWriter, r *http.Request, _ auth.User, token string) {
	user, err := api.admin.ManagedUser(r.Context(), token, chi.URLParam(r, "id"))
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, 200, struct {
		User admin.User `json:"user"`
	}{user})
}

func (api *authHTTP) getGroup(w http.ResponseWriter, r *http.Request, _ auth.User, token string) {
	group, err := api.admin.Group(r.Context(), token, chi.URLParam(r, "id"))
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, 200, struct {
		Group admin.Group `json:"group"`
	}{group})
}

func (api *authHTTP) createGroup(w http.ResponseWriter, r *http.Request, _ auth.User, token string) {
	var input struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		api.fail(w, r, err)
		return
	}
	group, err := api.admin.CreateGroup(r.Context(), token, input.Name, input.Description)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, 201, struct {
		Group   admin.Group `json:"group"`
		EventID string      `json:"eventId"`
	}{group, eventID(r)})
}

func (api *authHTTP) updateGroup(w http.ResponseWriter, r *http.Request, _ auth.User, token string) {
	var input struct {
		Name        json.RawMessage `json:"name"`
		Description json.RawMessage `json:"description"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		api.fail(w, r, err)
		return
	}
	name, err := patchString(input.Name, false)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	description, err := patchString(input.Description, false)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	group, err := api.admin.UpdateGroup(r.Context(), token, chi.URLParam(r, "id"), admin.GroupPatch{Name: name, Description: description})
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, 200, struct {
		Group   admin.Group `json:"group"`
		EventID string      `json:"eventId"`
	}{group, eventID(r)})
}

func (api *authHTTP) deleteGroup(w http.ResponseWriter, r *http.Request, _ auth.User, token string) {
	if err := api.admin.DeleteGroup(r.Context(), token, chi.URLParam(r, "id")); err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]string{"eventId": eventID(r)})
}

func (api *authHTTP) addGroupMember(w http.ResponseWriter, r *http.Request, _ auth.User, token string) {
	var input struct {
		UserID string `json:"userId"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		api.fail(w, r, err)
		return
	}
	if err := api.admin.ChangeMember(r.Context(), token, chi.URLParam(r, "id"), input.UserID, true); err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, 201, map[string]string{"eventId": eventID(r)})
}

func (api *authHTTP) removeGroupMember(w http.ResponseWriter, r *http.Request, _ auth.User, token string) {
	if err := api.admin.ChangeMember(r.Context(), token, chi.URLParam(r, "id"), chi.URLParam(r, "userId"), false); err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]string{"eventId": eventID(r)})
}
