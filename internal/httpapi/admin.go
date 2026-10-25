package httpapi

import (
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"

	"pandatable/internal/admin"
	"pandatable/internal/apperr"
	"pandatable/internal/auth"
)

func (api *authHTTP) administrator(next func(http.ResponseWriter, *http.Request, auth.User, string)) http.HandlerFunc {
	return api.authenticated(func(w http.ResponseWriter, r *http.Request, user auth.User) {
		if user.Role != "admin" {
			api.fail(w, r, apperr.New(403, "ERR_PERMISSION_DENIED", "Administrator required"))
			return
		}
		token, err := requestToken(r)
		if err != nil {
			api.fail(w, r, err)
			return
		}
		next(w, r, user, token)
	})
}

func (api *authHTTP) mountAdmin(router chi.Router) {
	router.Route("/admin", func(router chi.Router) {
		// Guard the namespace, including future/unknown admin endpoints.
		router.Use(func(next http.Handler) http.Handler {
			return api.administrator(func(w http.ResponseWriter, r *http.Request, _ auth.User, _ string) {
				next.ServeHTTP(w, r)
			})
		})
		router.Get("/users", api.adminAction(api.listUsers))
		router.Post("/users", api.adminAction(api.createUser))
		router.Patch("/users/{id}", api.adminAction(api.updateUser))
		router.Post("/users/{id}/reset-password", api.adminAction(api.resetUserPassword))
	})
}

func (api *authHTTP) adminAction(next func(http.ResponseWriter, *http.Request, auth.User, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := UserFromContext(r.Context())
		if !ok {
			api.fail(w, r, apperr.New(500, "ERR_INTERNAL", "Missing admin request identity"))
			return
		}
		token, err := requestToken(r)
		if err != nil {
			api.fail(w, r, err)
			return
		}
		next(w, r, user, token)
	}
}

func (api *authHTTP) listUsers(w http.ResponseWriter, r *http.Request, _ auth.User, token string) {
	input := admin.ListInput{Limit: 50}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		api.fail(w, r, apperr.New(400, "ERR_VALIDATION", "Malformed user list query"))
		return
	}
	for name, values := range query {
		if len(values) != 1 {
			api.fail(w, r, apperr.New(400, "ERR_VALIDATION", "Duplicate user list parameter"))
			return
		}
		switch name {
		case "q":
			input.Q = values[0]
		case "status":
			input.Status = values[0]
		case "offset", "limit":
			value, err := strconv.ParseInt(values[0], 10, 32)
			if err != nil {
				api.fail(w, r, apperr.New(400, "ERR_VALIDATION", "Invalid pagination"))
				return
			}
			if name == "offset" {
				input.Offset = int32(value)
			} else {
				input.Limit = int32(value)
			}
		default:
			api.fail(w, r, apperr.New(400, "ERR_VALIDATION", "Unknown user list parameter"))
			return
		}
	}
	result, err := api.admin.List(r.Context(), token, input)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, 200, result)
}

func (api *authHTTP) createUser(w http.ResponseWriter, r *http.Request, _ auth.User, token string) {
	var input struct {
		Username    string  `json:"username"`
		Email       *string `json:"email"`
		DisplayName string  `json:"displayName"`
		Role        string  `json:"role"`
		Password    string  `json:"password"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		api.fail(w, r, err)
		return
	}
	user, err := api.admin.Create(r.Context(), token, admin.CreateInput{Username: input.Username, Email: input.Email,
		DisplayName: input.DisplayName, Role: input.Role, Password: input.Password})
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, 201, struct {
		User    admin.User `json:"user"`
		EventID string     `json:"eventId"`
	}{user, rand.Text()})
}

func (api *authHTTP) updateUser(w http.ResponseWriter, r *http.Request, actor auth.User, token string) {
	var input struct {
		DisplayName json.RawMessage `json:"displayName"`
		Email       json.RawMessage `json:"email"`
		Role        json.RawMessage `json:"role"`
		Status      json.RawMessage `json:"status"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		api.fail(w, r, err)
		return
	}
	patch := admin.UserPatch{EmailSet: len(input.Email) != 0}
	for _, field := range []struct {
		raw      json.RawMessage
		dest     **string
		nullable bool
	}{{input.DisplayName, &patch.DisplayName, false}, {input.Email, &patch.Email, true},
		{input.Role, &patch.Role, false}, {input.Status, &patch.Status, false}} {
		value, err := patchString(field.raw, field.nullable)
		if err != nil {
			api.fail(w, r, err)
			return
		}
		*field.dest = value
	}
	result, err := api.admin.Update(r.Context(), token, chi.URLParam(r, "id"), patch)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	if result.User.ID == actor.ID && result.SessionsRevoked {
		api.clearCookie(w)
	}
	writeJSON(w, 200, struct {
		admin.UpdateResult
		EventID string `json:"eventId"`
	}{result, rand.Text()})
}

func (api *authHTTP) resetUserPassword(w http.ResponseWriter, r *http.Request, actor auth.User, token string) {
	var input struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		api.fail(w, r, err)
		return
	}
	id := chi.URLParam(r, "id")
	if err := api.admin.ResetPassword(r.Context(), token, id, input.Password); err != nil {
		api.fail(w, r, err)
		return
	}
	if id == actor.ID {
		api.clearCookie(w)
	}
	writeJSON(w, 200, map[string]string{"eventId": rand.Text()})
}
