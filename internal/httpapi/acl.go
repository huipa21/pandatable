package httpapi

import (
	"context"
	"crypto/rand"
	"net/http"

	"github.com/go-chi/chi/v5"

	"pandatable/internal/acl"
	"pandatable/internal/apperr"
	"pandatable/internal/auth"
)

type userContextKey struct{}

// UserFromContext returns the active, password-change-cleared user that
// authenticated or require placed on the request.
func UserFromContext(ctx context.Context) (auth.User, bool) {
	user, ok := ctx.Value(userContextKey{}).(auth.User)
	return user, ok
}

// authenticated enforces the session, disabled-user and forced-password-change
// rules for protected routes and refreshes the sliding cookie.
func (api *authHTTP) authenticated(next func(http.ResponseWriter, *http.Request, auth.User)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, token, ok := api.session(w, r, false)
		if !ok {
			return
		}
		api.cookie(w, token, session.ExpiresAt)
		next(w, r.WithContext(context.WithValue(r.Context(), userContextKey{}, session.User)), session.User)
	}
}

// require authenticates and then demands at least the level on the resource
// whose UUID is the chi URL parameter: 404 when hidden, 403 when visible but
// insufficient. Later resource routes mount it per workspace/book/table.
func (api *authHTTP) require(level acl.Level, kind acl.ResourceType, param string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return api.authenticated(func(w http.ResponseWriter, r *http.Request, user auth.User) {
			resource, err := acl.NewResource(string(kind), chi.URLParam(r, param))
			if err != nil {
				// A malformed id can never name a resource.
				api.fail(w, r, acl.ErrNotFound())
				return
			}
			if err := api.acl.Resolver().Require(r.Context(), user.ID, level, resource); err != nil {
				api.fail(w, r, err)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (api *authHTTP) mountGrants(router chi.Router) {
	router.Get("/grants", api.authenticated(api.listGrants))
	router.Put("/grants", api.authenticated(api.putGrant))
	router.Delete("/grants/{id}", api.authenticated(api.deleteGrant))
}

func (api *authHTTP) listGrants(w http.ResponseWriter, r *http.Request, user auth.User) {
	values := r.URL.Query()["resource"]
	if len(values) != 1 {
		api.fail(w, r, apperr.New(400, "ERR_VALIDATION", "Exactly one resource query value is required"))
		return
	}
	resource, err := acl.ParseResourceRef(values[0])
	if err != nil {
		api.fail(w, r, err)
		return
	}
	listing, err := api.acl.List(r.Context(), user.ID, resource)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listing)
}

func (api *authHTTP) putGrant(w http.ResponseWriter, r *http.Request, user auth.User) {
	var input struct {
		ResourceType  string `json:"resourceType"`
		ResourceID    string `json:"resourceId"`
		PrincipalType string `json:"principalType"`
		PrincipalID   string `json:"principalId"`
		Level         string `json:"level"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		api.fail(w, r, err)
		return
	}
	result, err := api.acl.Put(r.Context(), user.ID, acl.PutInput{ResourceType: input.ResourceType,
		ResourceID: input.ResourceID, PrincipalType: input.PrincipalType, PrincipalID: input.PrincipalID,
		Level: input.Level})
	if err != nil {
		api.fail(w, r, err)
		return
	}
	status := http.StatusOK
	if result.Created {
		status = http.StatusCreated
	}
	writeJSON(w, status, struct {
		acl.PutResult
		EventID string `json:"eventId"`
	}{result, rand.Text()})
}

func (api *authHTTP) deleteGrant(w http.ResponseWriter, r *http.Request, user auth.User) {
	if err := api.acl.Delete(r.Context(), user.ID, chi.URLParam(r, "id")); err != nil {
		api.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"eventId": rand.Text()})
}
