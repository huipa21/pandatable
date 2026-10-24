package httpapi

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

const testIndex = "<!doctype html><html><body>PandaTable fixture</body></html>"

func testAssets() fs.FS {
	return fstest.MapFS{
		"index.html":    {Data: []byte(testIndex)},
		"assets/app.js": {Data: []byte("console.log('fixture');")},
		".gitkeep":      {},
	}
}

func testRouter(t *testing.T, output io.Writer, level slog.Level, assets fs.FS) *chi.Mux {
	t.Helper()
	handler, err := New(slog.New(slog.NewJSONHandler(output, &slog.HandlerOptions{Level: level})), assets)
	if err != nil {
		t.Fatal(err)
	}
	router, ok := handler.(*chi.Mux)
	if !ok {
		t.Fatalf("handler has unexpected type %T", handler)
	}
	return router
}

func TestRoutesAndSPAFallback(t *testing.T) {
	router := testRouter(t, io.Discard, slog.LevelInfo, testAssets())
	for _, test := range []struct {
		method, path string
		status       int
		body         string
	}{
		{"GET", "/healthz", 200, "{\"status\":\"ok\"}\n"},
		{"HEAD", "/healthz", 200, ""},
		{"GET", "/", 200, testIndex},
		{"GET", "/index.html", 200, testIndex},
		{"GET", "/workspaces/book", 200, testIndex},
		{"GET", "/workspaces/book/", 200, testIndex},
		{"HEAD", "/workspaces/book", 200, ""},
		{"GET", "/assets/app.js", 200, "console.log('fixture');"},
		{"GET", "/assets/missing.js", 404, "ERR_NOT_FOUND"},
		{"GET", "/assets/missing", 404, "ERR_NOT_FOUND"},
		{"GET", "/missing.css", 404, "ERR_NOT_FOUND"},
		{"GET", "/.gitkeep", 404, "ERR_NOT_FOUND"},
		{"GET", "/assets/../index.html", 404, "ERR_NOT_FOUND"},
		{"GET", "/assets%5capp.js", 404, "ERR_NOT_FOUND"},
		{"POST", "/workspaces/book", 405, "ERR_METHOD_NOT_ALLOWED"},
		{"POST", "/healthz", 405, "ERR_METHOD_NOT_ALLOWED"},
	} {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(test.method, test.path, nil))
			if response.Code != test.status {
				t.Fatalf("status=%d, want %d; body=%s", response.Code, test.status, response.Body)
			}
			if test.status == http.StatusOK {
				if response.Body.String() != test.body {
					t.Errorf("body=%q, want %q", response.Body.String(), test.body)
				}
			} else {
				assertError(t, response, test.body)
			}
			if test.body == testIndex && response.Header().Get("Cache-Control") != "no-cache" {
				t.Error("SPA index must be revalidated")
			}
		})
	}
}

func TestReservedPathsNeverServeSPA(t *testing.T) {
	for _, assets := range []fs.FS{nil, testAssets()} {
		router := testRouter(t, io.Discard, slog.LevelInfo, assets)
		for _, path := range []string{
			"/api", "/api/", "/api/v1", "/api/v1/", "/api/v1/missing",
			"/api/v2/unknown", "/healthz/missing", "/healthz/",
			"/api%2fv1/missing", "/%61pi/v1/missing", "/health%7A/unknown",
		} {
			for _, method := range []string{http.MethodGet, http.MethodPost} {
				response := httptest.NewRecorder()
				router.ServeHTTP(response, httptest.NewRequest(method, path, nil))
				if response.Code != http.StatusNotFound {
					t.Errorf("%s %s: status=%d, want 404; body=%s", method, path, response.Code, response.Body)
				}
				assertError(t, response, "ERR_NOT_FOUND")
			}
		}
	}
}

func TestMissingSPAIsExplicitAndHealthIsIndependent(t *testing.T) {
	for _, assets := range []fs.FS{nil, fstest.MapFS{".gitkeep": {}}, fstest.MapFS{"index.html": {}}} {
		var logs bytes.Buffer
		router := testRouter(t, &logs, slog.LevelInfo, assets)
		for _, path := range []string{"/", "/workspaces/book", "/assets/app.js"} {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
			if response.Code != http.StatusServiceUnavailable {
				t.Errorf("%s: status=%d, want 503", path, response.Code)
			}
			assertError(t, response, "ERR_SPA_UNAVAILABLE")
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest("GET", "/healthz", nil))
		if response.Code != http.StatusOK {
			t.Fatalf("health without assets: %d", response.Code)
		}
		if !strings.Contains(logs.String(), "SPA assets unavailable") {
			t.Fatal("missing SPA was not reported")
		}
	}
}

type brokenFS struct{}

func (brokenFS) Open(string) (fs.File, error) { return nil, fs.ErrPermission }

func TestSPAReadErrorsAreNotTreatedAsMissingBuild(t *testing.T) {
	_, err := New(slog.New(slog.NewJSONHandler(io.Discard, nil)), brokenFS{})
	if !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("New() error=%v, want filesystem error", err)
	}
}

func TestRequestIDAccessLogAndSecurityHeaders(t *testing.T) {
	var logs bytes.Buffer
	router := testRouter(t, &logs, slog.LevelInfo, testAssets())
	var contextID string
	router.Get("/inspect/{id}", func(w http.ResponseWriter, r *http.Request) {
		contextID = middleware.GetReqID(r.Context())
		w.WriteHeader(http.StatusNoContent)
	})
	seen := make(map[string]bool)
	for range 2 {
		request := httptest.NewRequest("GET", "/inspect/path-secret?token=query-secret", strings.NewReader("body-secret"))
		request.Header.Set("Cookie", "session=cookie-secret")
		request.Header.Set("Authorization", "Bearer auth-secret")
		request.Header.Set("X-Request-ID", "untrusted-secret")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		id := response.Header().Get("X-Request-ID")
		if id == "" || id != contextID || id == "untrusted-secret" || seen[id] {
			t.Fatalf("invalid request id: header=%q context=%q", id, contextID)
		}
		seen[id] = true
		for header, want := range map[string]string{
			"X-Content-Type-Options": "nosniff",
			"X-Frame-Options":        "DENY",
			"Referrer-Policy":        "no-referrer",
		} {
			if got := response.Header().Get(header); got != want {
				t.Errorf("%s=%q, want %q", header, got, want)
			}
		}
		if !strings.Contains(response.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
			t.Error("missing CSP frame protection")
		}
	}
	events := parseLogs(t, logs.Bytes())
	if len(events) != 2 {
		t.Fatalf("access log events=%d, want 2", len(events))
	}
	for _, event := range events {
		if event["msg"] != "HTTP request" || event["route"] != "/inspect/{id}" ||
			event["method"] != "GET" || event["status"] != float64(204) ||
			event["bytes"] != float64(0) || event["duration_ms"] == nil || !seen[event["request_id"].(string)] {
			t.Errorf("unexpected access log: %v", event)
		}
	}
	if strings.Contains(logs.String(), "secret") {
		t.Fatalf("request secrets leaked into logs: %s", logs.String())
	}
}

func TestAccessLogHonorsConfiguredLevel(t *testing.T) {
	var logs bytes.Buffer
	router := testRouter(t, &logs, slog.LevelWarn, testAssets())
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/healthz", nil))
	if logs.Len() != 0 {
		t.Fatalf("info access log was emitted at warn level: %s", logs.String())
	}
}

func TestRecoveryIsJSONAndDoesNotLeakPanic(t *testing.T) {
	var logs bytes.Buffer
	router := testRouter(t, &logs, slog.LevelInfo, testAssets())
	router.Get("/panic", func(http.ResponseWriter, *http.Request) { panic("panic-secret") })
	response := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/panic", nil)
	request.Header.Set("Accept-Encoding", "gzip")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("panic status=%d, want 500", response.Code)
	}
	body := decodedBody(t, response)
	if !strings.Contains(body, "ERR_INTERNAL") || strings.Contains(body, "panic-secret") ||
		strings.Contains(logs.String(), "panic-secret") {
		t.Fatalf("unsafe panic handling: body=%q logs=%s", body, logs.String())
	}
	if response.Header().Get("X-Frame-Options") != "DENY" || response.Header().Get("X-Request-ID") == "" {
		t.Fatal("recovered response lost security headers or request ID")
	}
	events := parseLogs(t, logs.Bytes())
	if len(events) != 2 || events[0]["level"] != "ERROR" || events[1]["status"] != float64(500) ||
		events[0]["request_id"] != events[1]["request_id"] {
		t.Fatalf("panic/access logs not correlated: %v", events)
	}
}

func TestRecoveryAbortsAfterResponseStarted(t *testing.T) {
	router := testRouter(t, io.Discard, slog.LevelInfo, testAssets())
	router.Get("/panic", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("partial"))
		panic("panic-secret")
	})
	response := httptest.NewRecorder()
	defer func() {
		if got := recover(); got != http.ErrAbortHandler {
			t.Errorf("panic=%v, want http.ErrAbortHandler", got)
		}
		if response.Body.String() != "partial" {
			t.Errorf("recovery appended an error to an already-started response: %s", response.Body)
		}
	}()
	router.ServeHTTP(response, httptest.NewRequest("GET", "/panic", nil))
}

func TestGzipNegotiation(t *testing.T) {
	router := testRouter(t, io.Discard, slog.LevelInfo, testAssets())
	for _, encoding := range []string{"gzip", "identity", ""} {
		for _, path := range []string{"/", "/healthz", "/api/v1/missing", "/assets/app.js"} {
			t.Run(encoding+path, func(t *testing.T) {
				request := httptest.NewRequest("GET", path, nil)
				request.Header.Set("Accept-Encoding", encoding)
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				if got := response.Header().Get("Content-Encoding"); (got == "gzip") != (encoding == "gzip") {
					t.Fatalf("Content-Encoding=%q with Accept-Encoding=%q", got, encoding)
				}
				body := decodedBody(t, response)
				if body == "" || (path == "/" && body != testIndex) {
					t.Fatalf("unexpected decoded body: %q", body)
				}
				if !strings.Contains(response.Header().Get("Vary"), "Accept-Encoding") {
					t.Error("missing Vary: Accept-Encoding")
				}
			})
		}
	}
}

func assertError(t *testing.T, response *httptest.ResponseRecorder, code string) {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if response.Header().Get("Content-Type") != "application/json" {
		t.Errorf("error content type=%q", response.Header().Get("Content-Type"))
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.Error.Code != code {
		t.Errorf("error body=%q, want code %s (decode error %v)", response.Body, code, err)
	}
}

func decodedBody(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	if response.Header().Get("Content-Encoding") != "gzip" {
		return response.Body.String()
	}
	reader, err := gzip.NewReader(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func parseLogs(t *testing.T, data []byte) []map[string]any {
	t.Helper()
	var events []map[string]any
	for line := range bytes.SplitSeq(bytes.TrimSpace(data), []byte("\n")) {
		var event map[string]any
		if err := json.Unmarshal(line, &event); err != nil {
			t.Fatalf("invalid JSON log: %v: %q", err, line)
		}
		events = append(events, event)
	}
	return events
}
