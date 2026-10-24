package httpapi

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func New(logger *slog.Logger, assets fs.FS, options ...AuthOptions) (http.Handler, error) {
	if len(options) > 1 {
		return nil, errors.New("only one auth configuration is supported")
	}
	spa, err := newSPA(logger, assets)
	if err != nil {
		return nil, err
	}
	router := chi.NewRouter()
	router.Use(requestID, accessLog(logger), securityHeaders,
		middleware.SetHeader("Vary", "Accept-Encoding"), middleware.Compress(5), recoverPanic(logger))
	router.NotFound(notFound)
	router.MethodNotAllowed(methodNotAllowed)

	health := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodHead {
			_, _ = w.Write([]byte("{\"status\":\"ok\"}\n"))
		}
	}
	router.Handle("/healthz", http.HandlerFunc(methodNotAllowed))
	router.Get("/healthz", health)
	router.Head("/healthz", health)
	router.Handle("/healthz/*", http.HandlerFunc(notFound))

	api := chi.NewRouter()
	api.NotFound(notFound)
	api.MethodNotAllowed(apiMethodNotAllowed(api))
	if len(options) == 1 {
		authAPI, err := newAuthHTTP(logger, options[0])
		if err != nil {
			return nil, err
		}
		authAPI.mount(api)
	}
	router.Mount("/api/v1", api)
	router.Handle("/api", http.HandlerFunc(notFound))
	router.Handle("/api/*", http.HandlerFunc(notFound))

	router.Handle("/*", spa)
	return router, nil
}

func notFound(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotFound, "ERR_NOT_FOUND", "Not found")
}

func methodNotAllowed(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Allow", "GET, HEAD")
	writeError(w, http.StatusMethodNotAllowed, "ERR_METHOD_NOT_ALLOWED", "Method not allowed")
}

func apiMethodNotAllowed(router *chi.Mux) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if route := chi.RouteContext(r.Context()); route != nil && route.RoutePath != "" {
			path = route.RoutePath
		}
		var allowed []string
		for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
			http.MethodPatch, http.MethodDelete, http.MethodConnect, http.MethodOptions, http.MethodTrace} {
			if router.Match(chi.NewRouteContext(), method, path) {
				allowed = append(allowed, method)
			}
		}
		w.Header().Set("Allow", strings.Join(allowed, ", "))
		writeError(w, http.StatusMethodNotAllowed, "ERR_METHOD_NOT_ALLOWED", "Method not allowed")
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	type detail struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	_ = json.NewEncoder(w).Encode(struct {
		Error detail `json:"error"`
	}{Error: detail{Code: code, Message: message}})
}
