package httpapi

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"

	"pandatable/internal/acl"
	"pandatable/internal/auth"
	"pandatable/internal/structure"
)

func commentPageQuery(r *http.Request, allowFocus bool) (int, string, error) {
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return 0, "", acl.ErrValidation()
	}
	for key, values := range query {
		if len(values) != 1 || key != "offset" && (!allowFocus || key != "focus") {
			return 0, "", acl.ErrValidation()
		}
	}
	offset := 0
	if values, ok := query["offset"]; ok {
		offset, err = strconv.Atoi(values[0])
		if err != nil {
			return 0, "", acl.ErrValidation()
		}
	}
	return offset, query.Get("focus"), nil
}

func (api *authHTTP) listComments(w http.ResponseWriter, r *http.Request, user auth.User) {
	offset, focus, err := commentPageQuery(r, true)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	result, err := api.structure.ListComments(r.Context(), actorOf(user), chi.URLParam(r, "id"),
		chi.URLParam(r, "rowId"), offset, focus)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (api *authHTTP) createComment(w http.ResponseWriter, r *http.Request, user auth.User) {
	var input struct {
		Body string `json:"body"`
	}
	if err := decodeJSONLimit(w, r, &input, 32*1024); err != nil {
		api.fail(w, r, err)
		return
	}
	comment, err := api.structure.CreateComment(r.Context(), actorOf(user), chi.URLParam(r, "id"),
		chi.URLParam(r, "rowId"), input.Body)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, struct {
		Comment structure.Comment `json:"comment"`
		EventID string            `json:"eventId"`
	}{comment, eventID(r)})
}

func (api *authHTTP) updateComment(w http.ResponseWriter, r *http.Request, user auth.User) {
	var input struct {
		Body string `json:"body"`
	}
	if err := decodeJSONLimit(w, r, &input, 32*1024); err != nil {
		api.fail(w, r, err)
		return
	}
	comment, err := api.structure.UpdateComment(r.Context(), actorOf(user), chi.URLParam(r, "id"), input.Body)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Comment structure.Comment `json:"comment"`
		EventID string            `json:"eventId"`
	}{comment, eventID(r)})
}

func (api *authHTTP) deleteComment(w http.ResponseWriter, r *http.Request, user auth.User) {
	if err := api.structure.DeleteComment(r.Context(), actorOf(user), chi.URLParam(r, "id")); err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"eventId": eventID(r)})
}

func (api *authHTTP) listNotifications(w http.ResponseWriter, r *http.Request, user auth.User) {
	offset, _, err := commentPageQuery(r, false)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	result, err := api.structure.ListNotifications(r.Context(), actorOf(user), offset)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (api *authHTTP) readNotification(w http.ResponseWriter, r *http.Request, user auth.User) {
	var input struct {
		ID string `json:"id"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		api.fail(w, r, err)
		return
	}
	if err := api.structure.ReadNotification(r.Context(), actorOf(user), input.ID); err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"eventId": eventID(r)})
}
