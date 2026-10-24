package httpapi

import (
	"context"
	"crypto/rand"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := rand.Text()
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), middleware.RequestIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func accessLog(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			wrapped := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			defer func() {
				status := wrapped.Status()
				if status == 0 {
					status = http.StatusOK
				}
				route := ""
				if routeContext := chi.RouteContext(r.Context()); routeContext != nil {
					route = routeContext.RoutePattern()
				}
				logger.InfoContext(r.Context(), "HTTP request",
					"request_id", middleware.GetReqID(r.Context()),
					"method", r.Method, "route", route,
					"status", status, "bytes", wrapped.BytesWritten(),
					"duration_ms", float64(time.Since(start).Microseconds())/1000)
			}()
			next.ServeHTTP(wrapped, r)
		})
	}
}

func recoverPanic(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			wrapped := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			defer func() {
				if value := recover(); value != nil {
					if value == http.ErrAbortHandler {
						panic(value)
					}
					// Panic values and stacks can contain request or database secrets.
					logger.ErrorContext(r.Context(), "HTTP handler panic",
						"request_id", middleware.GetReqID(r.Context()))
					if wrapped.Status() != 0 {
						panic(http.ErrAbortHandler)
					}
					writeError(wrapped, http.StatusInternalServerError, "ERR_INTERNAL", "Internal server error")
				}
			}()
			next.ServeHTTP(wrapped, r)
		})
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}
