package httpapi

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"
)

func newSPA(logger *slog.Logger, assets fs.FS) (http.Handler, error) {
	var index []byte
	var err error
	if assets != nil {
		index, err = fs.ReadFile(assets, "index.html")
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("read SPA index: %w", err)
		}
	}
	if len(bytes.TrimSpace(index)) == 0 {
		logger.Warn("SPA assets unavailable; build the frontend before serving the UI")
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if reservedSPAPath(r.URL.Path) {
				notFound(w, r)
				return
			}
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				methodNotAllowed(w, r)
				return
			}
			writeError(w, http.StatusServiceUnavailable, "ERR_SPA_UNAVAILABLE", "Frontend assets unavailable")
		}), nil
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if reservedSPAPath(r.URL.Path) {
			notFound(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			methodNotAllowed(w, r)
			return
		}
		name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/"), "/")
		if name == "" {
			name = "index.html"
		}
		if !fs.ValidPath(name) || strings.Contains(name, "\\") || hasHiddenSegment(name) {
			notFound(w, r)
			return
		}
		if name == "index.html" {
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(index))
			return
		}
		info, err := fs.Stat(assets, name)
		switch {
		case err == nil && info.Mode().IsRegular():
			http.ServeFileFS(w, r, assets, name)
			return
		case err != nil && !errors.Is(err, fs.ErrNotExist):
			logger.ErrorContext(r.Context(), "SPA asset lookup failed")
			writeError(w, http.StatusInternalServerError, "ERR_INTERNAL", "Internal server error")
			return
		}
		if path.Ext(name) != "" || name == "assets" || strings.HasPrefix(name, "assets/") {
			notFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(index))
	}), nil
}

func reservedSPAPath(name string) bool {
	return name == "/api" || strings.HasPrefix(name, "/api/") ||
		name == "/healthz" || strings.HasPrefix(name, "/healthz/")
}

func hasHiddenSegment(name string) bool {
	for segment := range strings.SplitSeq(name, "/") {
		if strings.HasPrefix(segment, ".") {
			return true
		}
	}
	return false
}
