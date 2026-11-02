package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestAuthOriginPolicy(t *testing.T) {
	for _, test := range []struct {
		base, origin string
		secure       bool
	}{
		{"https://Table.Example:443/path", "https://table.example", true},
		{"https://table.example:8443", "https://table.example:8443", true},
		{"http://localhost:5173", "http://localhost:5173", false},
		{"http://127.0.0.1:8080", "http://127.0.0.1:8080", false},
		{"http://[::1]:80", "http://[::1]", false},
	} {
		origin, secure, err := authOrigin(test.base)
		if err != nil || origin != test.origin || secure != test.secure {
			t.Errorf("origin: got %q secure=%t err=%v", origin, secure, err)
		}
	}
	for _, base := range []string{"", "ftp://table.example", "http://table.example",
		"http://192.168.1.2", "http://localhost.example", "http://localhost.",
		"https://user:pass@table.example", "https://table.example?x=1", "https://table.example#x"} {
		if _, _, err := authOrigin(base); err == nil {
			t.Errorf("unsafe auth base accepted: %s", base)
		}
	}
}

func TestCSRFRequiresBothHeaderAndOrigin(t *testing.T) {
	api := &authHTTP{origin: "https://table.example", logger: slog.New(slog.NewJSONHandler(io.Discard, nil))}
	for _, test := range []struct {
		method, origin, csrf string
		status               int
	}{
		{"POST", "", "", 403}, {"POST", "https://table.example", "", 403},
		{"POST", "", "1", 403}, {"POST", "https://evil.example", "1", 403},
		{"POST", "null", "1", 403}, {"POST", "https://table.example/", "1", 403},
		{"POST", "https://table.example:443", "1", 403}, {"POST", "https://table.example", "true", 403},
		{"POST", "https://table.example", "1", 204}, {"PATCH", "https://table.example", "1", 204},
		{"DELETE", "https://evil.example", "1", 403}, {"GET", "", "", 204}, {"HEAD", "", "", 204},
	} {
		request := httptest.NewRequest(test.method, "/api/v1/auth/login", nil)
		if test.origin != "" {
			request.Header.Set("Origin", test.origin)
		}
		if test.csrf != "" {
			request.Header.Set("X-PT-CSRF", test.csrf)
		}
		request.Header.Set("X-Forwarded-Host", "evil.example")
		request.Header.Set("X-Forwarded-Proto", "http")
		response := httptest.NewRecorder()
		api.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := r.Context().Deadline(); !ok {
				t.Error("auth handler lacks request timeout")
			}
			w.WriteHeader(204)
		})).ServeHTTP(response, request)
		if response.Code != test.status || response.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("%s origin=%q csrf=%q: status=%d", test.method, test.origin, test.csrf, response.Code)
		}
	}
	for _, header := range []string{"Origin", "X-PT-CSRF"} {
		request := httptest.NewRequest("POST", "/api/v1/auth/login", nil)
		request.Header.Set("Origin", api.origin)
		request.Header.Set("X-PT-CSRF", "1")
		request.Header.Add(header, request.Header.Get(header))
		response := httptest.NewRecorder()
		api.protect(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("duplicate security header accepted") })).ServeHTTP(response, request)
		if response.Code != 403 {
			t.Fatal("duplicate security header should fail")
		}
	}
}

func TestCSRFLocalDevOriginIsExact(t *testing.T) {
	api := &authHTTP{origin: "http://127.0.0.1:5173", logger: slog.New(slog.NewJSONHandler(io.Discard, nil))}
	for _, test := range []struct {
		origin string
		status int
	}{
		{"http://127.0.0.1:5173", http.StatusNoContent},
		{"http://localhost:5173", http.StatusForbidden},
		{"http://127.0.0.1:8080", http.StatusForbidden},
		{"https://evil.example", http.StatusForbidden},
	} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/setup", nil)
		request.Header.Set("Origin", test.origin)
		request.Header.Set("X-PT-CSRF", "1")
		response := httptest.NewRecorder()
		api.protect(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})).ServeHTTP(response, request)
		if response.Code != test.status {
			t.Errorf("origin=%q: status=%d, want=%d", test.origin, response.Code, test.status)
		}
	}
}

func TestTrustedProxyChains(t *testing.T) {
	proxies := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("2001:db8:1::/48")}
	for _, test := range []struct {
		remote, forwarded, want string
		none                    bool
	}{
		{"192.0.2.10:12", "198.51.100.1", "192.0.2.10", false},
		{"10.0.0.1:12", "198.51.100.1", "10.0.0.1", true},
		{"10.0.0.1:12", "198.51.100.1, 10.0.0.2", "198.51.100.1", false},
		{"10.0.0.1:12", "203.0.113.1, 192.0.2.5, 10.0.0.2", "192.0.2.5", false},
		{"10.0.0.1:12", "", "10.0.0.1", false},
		{"[::ffff:192.0.2.10]:12", "198.51.100.1", "192.0.2.10", false},
		{"[2001:db8:1::1]:12", "2001:db8:2::1, 2001:db8:1::2", "2001:db8:2::1", false},
		{"192.0.2.10:12", "invalid", "192.0.2.10", false},
	} {
		request := httptest.NewRequest("GET", "/", nil)
		request.RemoteAddr = test.remote
		request.Header.Set("X-Forwarded-For", test.forwarded)
		request.Header.Set("X-Real-IP", "203.0.113.3")
		trust := proxies
		if test.none {
			trust = nil
		}
		got, err := clientIP(request, trust)
		if err != nil || got.String() != test.want {
			t.Errorf("chain %s: got=%s err=%v want=%s", test.forwarded, got, err, test.want)
		}
	}
	for _, forwarded := range []string{"garbage", "198.51.100.1:123", "198.51.100.1,", strings.Repeat("10.0.0.1,", 33), "[::1]", "fe80::1%zone"} {
		request := httptest.NewRequest("GET", "/", nil)
		request.RemoteAddr = "10.0.0.1:123"
		request.Header.Set("X-Forwarded-For", forwarded)
		if _, err := clientIP(request, proxies); err == nil {
			t.Errorf("malformed trusted chain accepted: %s", forwarded)
		}
	}
}

func TestCookieFlagsAndDeletion(t *testing.T) {
	for _, secure := range []bool{true, false} {
		api := &authHTTP{secure: secure}
		response := httptest.NewRecorder()
		api.cookie(response, "test-token", time.Now().Add(7*24*time.Hour))
		cookies := response.Result().Cookies()
		if len(cookies) != 1 {
			t.Fatal("expected one session cookie")
		}
		cookie := cookies[0]
		if cookie.Name != sessionCookie || cookie.Value != "test-token" || cookie.Domain != "" ||
			cookie.Path != "/" || !cookie.HttpOnly || cookie.Secure != secure ||
			cookie.SameSite != http.SameSiteLaxMode || cookie.MaxAge < 604795 || cookie.MaxAge > 604800 {
			t.Fatal("incorrect session cookie flags")
		}
		response = httptest.NewRecorder()
		api.clearCookie(response)
		cookie = response.Result().Cookies()[0]
		if cookie.MaxAge != -1 || cookie.Value != "" || cookie.Path != "/" || !cookie.HttpOnly ||
			cookie.Secure != secure || cookie.SameSite != http.SameSiteLaxMode || cookie.Expires.After(time.Now()) {
			t.Fatal("cookie deletion must preserve security scope")
		}
	}
}
