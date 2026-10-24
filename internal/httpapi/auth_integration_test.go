package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"pandatable/internal/auth"
	"pandatable/internal/db"
	"pandatable/internal/testdb"
)

var authDatabase testdb.Suite

func TestMain(m *testing.M) { os.Exit(authDatabase.Run(m)) }

type httpAuthFixture struct {
	pool      *db.Pool
	handler   http.Handler
	logs      bytes.Buffer
	username  string
	email     string
	id        string
	password  string
	storedPHC string
}

func newHTTPAuthFixture(t *testing.T) *httpAuthFixture {
	t.Helper()
	pool := authDatabase.Open(t)
	service, err := auth.New(pool)
	if err != nil {
		t.Fatal(err)
	}
	fixture := &httpAuthFixture{pool: pool, username: strings.ToLower(rand.Text()), password: "Fixture-password-123"}
	fixture.email = fixture.username + "@example.test"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	fixture.storedPHC, err = auth.NewPasswords().Hash(ctx, fixture.password)
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO meta.users
		(username,email,display_name,password_hash,role,avatar_color)
		VALUES ($1,$2,'HTTP fixture',$3,'user','blue') RETURNING id::text`,
		fixture.username, fixture.email, fixture.storedPHC).Scan(&fixture.id); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := pool.Exec(ctx, "DELETE FROM meta.users WHERE id=$1", fixture.id); err != nil {
			t.Error(err)
		}
	})
	fixture.handler, err = New(slog.New(slog.NewJSONHandler(&fixture.logs, nil)), testAssets(),
		AuthOptions{Service: service, BaseURL: "https://table.example", TrustedProxies: nil})
	if err != nil {
		t.Fatal(err)
	}
	return fixture
}

func (fixture *httpAuthFixture) request(method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(body))
	request.RemoteAddr = "192.0.2.1:12345"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://table.example")
	request.Header.Set("X-PT-CSRF", "1")
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	fixture.handler.ServeHTTP(response, request)
	return response
}

func requireStatus(t *testing.T, response *httptest.ResponseRecorder, status int) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("HTTP status=%d want=%d body=%s", response.Code, status, response.Body)
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("auth response must never be cached")
	}
}

func (fixture *httpAuthFixture) login(t *testing.T, login, password string) *http.Cookie {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"login": login, "password": password})
	if err != nil {
		t.Fatal(err)
	}
	response := fixture.request("POST", "/auth/login", string(payload), nil)
	requireStatus(t, response, 200)
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatal("AT-803: HTTPS login must set HttpOnly, Secure, SameSite=Lax session cookie")
	}
	for _, secret := range []string{password, fixture.storedPHC, cookies[0].Value, "password_hash", "id_hash"} {
		if strings.Contains(response.Body.String(), secret) {
			t.Fatal("auth DTO exposes sensitive data")
		}
	}
	var result struct {
		User    auth.User `json:"user"`
		EventID string    `json:"eventId"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.User.ID != fixture.id || result.EventID == "" {
		t.Fatal("incorrect login DTO")
	}
	return cookies[0]
}

func TestAuthHTTPPostgreSQL(t *testing.T) {
	f := newHTTPAuthFixture(t)
	requireStatus(t, f.request("GET", "/auth/me", "", nil), 401)
	for _, test := range []struct{ method, path, allow string }{
		{"GET", "/auth/login", "POST"}, {"POST", "/auth/me", "GET, PATCH"},
		{"PUT", "/auth/password", "POST"},
	} {
		response := f.request(test.method, test.path, "{}", nil)
		requireStatus(t, response, 405)
		assertError(t, response, "ERR_METHOD_NOT_ALLOWED")
		if response.Header().Get("Allow") != test.allow {
			t.Fatalf("Allow=%q, want %q", response.Header().Get("Allow"), test.allow)
		}
	}
	first := f.login(t, f.username, f.password)
	second := f.login(t, f.email, f.password)
	response := f.request("GET", "/auth/me", "", first)
	requireStatus(t, response, 200)
	if !strings.Contains(response.Body.String(), `"effectiveTimezone":"Asia/Shanghai"`) ||
		!strings.Contains(response.Body.String(), `"timezone":null`) ||
		!strings.Contains(response.Header().Get("Content-Security-Policy"), "script-src 'self'") ||
		response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("incorrect auth profile/security headers")
	}
	response = f.request("PATCH", "/auth/me", `{"displayName":"Alice","locale":"zh-CN","theme":"dark","timezone":"Europe/London"}`, first)
	requireStatus(t, response, 200)
	response = f.request("GET", "/auth/me", "", second)
	requireStatus(t, response, 200)
	for _, fragment := range []string{`"displayName":"Alice"`, `"locale":"zh-CN"`, `"theme":"dark"`, `"effectiveTimezone":"Europe/London"`} {
		if !strings.Contains(response.Body.String(), fragment) {
			t.Fatal("profile update not shared by user's sessions")
		}
	}
	for _, body := range []string{`{"timezone":"Invalid/Zone"}`, `{"theme":null}`, `{"locale":null}`,
		`{"displayName":null}`, `{}`, `{"unknown":true}`, `{"timezone":12}`} {
		requireStatus(t, f.request("PATCH", "/auth/me", body, first), 400)
	}
	requireStatus(t, f.request("PATCH", "/auth/me", `{"timezone":null}`, first), 200)
	response = f.request("GET", "/meta/timezones", "", first)
	requireStatus(t, response, 200)
	if !strings.Contains(response.Body.String(), `"Asia/Shanghai"`) || !strings.Contains(response.Body.String(), `"Europe/London"`) {
		t.Fatal("timezone picker data must come from real PG18 catalog")
	}
	requireStatus(t, f.request("POST", "/auth/password", `{"current":"wrong-password","new":"Next-password-123"}`, first), 401)
	response = f.request("POST", "/auth/password", `{"current":"Fixture-password-123","new":"Next-password-123"}`, first)
	requireStatus(t, response, 200)
	deleted := response.Result().Cookies()
	if len(deleted) != 1 || deleted[0].MaxAge != -1 || !deleted[0].HttpOnly || !deleted[0].Secure {
		t.Fatal("password change must clear current cookie")
	}
	requireStatus(t, f.request("GET", "/auth/me", "", first), 401)
	requireStatus(t, f.request("GET", "/auth/me", "", second), 401)
	requireStatus(t, f.request("POST", "/auth/login", `{"login":"`+f.username+`","password":"Fixture-password-123"}`, nil), 401)
	current := f.login(t, f.email, "Next-password-123")
	response = f.request("POST", "/auth/logout", "", current)
	requireStatus(t, response, 200)
	if response.Result().Cookies()[0].MaxAge != -1 {
		t.Fatal("logout must expire cookie")
	}
	requireStatus(t, f.request("GET", "/auth/me", "", current), 401)
	requireStatus(t, f.request("POST", "/auth/logout", "", current), 200)
	for _, secret := range []string{first.Value, second.Value, current.Value, f.password, "Next-password-123", f.storedPHC, f.email} {
		if strings.Contains(f.logs.String(), secret) {
			t.Fatal("auth access/error logs exposed credentials or identity")
		}
	}
	t.Log("real PG18 HTTP login (username/email), logout, profile/password persistence and all-session revocation pass; this is API evidence, not browser AT-102/116")
}

func TestAuthCSRFAndLimitsHTTPPostgreSQL(t *testing.T) {
	f := newHTTPAuthFixture(t)
	for _, test := range []struct{ origin, csrf string }{
		{"", ""}, {"https://table.example", ""}, {"", "1"}, {"https://evil.example", "1"},
	} {
		request := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(`{"login":"`+f.username+`","password":"`+f.password+`"}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", test.origin)
		request.Header.Set("X-PT-CSRF", test.csrf)
		response := httptest.NewRecorder()
		f.handler.ServeHTTP(response, request)
		requireStatus(t, response, 403)
		assertError(t, response, "ERR_CSRF")
	}
	for index := range 5 {
		login := f.username
		if index%2 == 1 {
			login = strings.ToUpper(f.email)
		}
		response := f.request("POST", "/auth/login", `{"login":"`+login+`","password":"wrong-password"}`, nil)
		requireStatus(t, response, 401)
	}
	response := f.request("POST", "/auth/login", `{"login":"`+f.email+`","password":"`+f.password+`"}`, nil)
	requireStatus(t, response, 429)
	assertError(t, response, "ERR_RATE_LIMITED")
	if response.Header().Get("Retry-After") != "60" {
		t.Fatal("locked login must report retry guidance")
	}
	t.Log("AT-103/803 actual HTTP surface: five alias failures reject correct password; missing header/missing or foreign Origin rejected; HTTPS cookie/header checks in companion test")
}

func TestForcedChangeAndDisabledHTTPPostgreSQL(t *testing.T) {
	f := newHTTPAuthFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := f.pool.Exec(ctx, "UPDATE meta.users SET must_change_password=true WHERE id=$1", f.id); err != nil {
		t.Fatal(err)
	}
	cookie := f.login(t, f.username, f.password)
	response := f.request("GET", "/auth/me", "", cookie)
	requireStatus(t, response, 200)
	if !strings.Contains(response.Body.String(), `"mustChangePassword":true`) {
		t.Fatal("forced-change flag missing")
	}
	for _, response := range []*httptest.ResponseRecorder{
		f.request("PATCH", "/auth/me", `{"theme":"dark"}`, cookie),
		f.request("GET", "/meta/timezones", "", cookie),
	} {
		requireStatus(t, response, 403)
		assertError(t, response, "ERR_PASSWORD_CHANGE_REQUIRED")
	}
	requireStatus(t, f.request("POST", "/auth/password", `{"current":"Fixture-password-123","new":"Next-password-123"}`, cookie), 200)
	cookie = f.login(t, f.username, "Next-password-123")
	if _, err := f.pool.Exec(ctx, "UPDATE meta.users SET status='disabled' WHERE id=$1", f.id); err != nil {
		t.Fatal(err)
	}
	requireStatus(t, f.request("GET", "/auth/me", "", cookie), 401)
	requireStatus(t, f.request("POST", "/auth/login", `{"login":"`+f.username+`","password":"Next-password-123"}`, nil), 401)
}

func TestMalformedLoginIPLimitHTTPPostgreSQL(t *testing.T) {
	f := newHTTPAuthFixture(t)
	for attempt := range 10 {
		request := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader("{"))
		request.RemoteAddr = "192.0.2.1:12345"
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", "https://table.example")
		request.Header.Set("X-PT-CSRF", "1")
		request.Header.Set("X-Forwarded-For", "198.51.100."+strconv.Itoa(attempt+1))
		response := httptest.NewRecorder()
		f.handler.ServeHTTP(response, request)
		requireStatus(t, response, 400)
	}
	response := f.request("POST", "/auth/login", `{"login":"`+f.username+`","password":"`+f.password+`"}`, nil)
	requireStatus(t, response, 429)
	assertError(t, response, "ERR_RATE_LIMITED")
	t.Log("ten malformed login requests consume the same socket-IP budget; untrusted forwarding cannot bypass it")
}

func TestAuthInputAndInternalErrorsHTTPPostgreSQL(t *testing.T) {
	f := newHTTPAuthFixture(t)
	for _, body := range []string{`{"unknown":true}`, `{`, `{} {}`, `{"login":12}`, strings.Repeat(" ", 4097)} {
		requireStatus(t, f.request("POST", "/auth/login", body, nil), 400)
	}
	request := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader("{}"))
	request.Header.Set("Origin", "https://table.example")
	request.Header.Set("X-PT-CSRF", "1")
	request.Header.Set("Content-Type", "text/plain")
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, request)
	requireStatus(t, response, 415)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := f.pool.Exec(ctx, "UPDATE meta.users SET password_hash='secret-invalid-PHC' WHERE id=$1", f.id); err != nil {
		t.Fatal(err)
	}
	response = f.request("POST", "/auth/login", `{"login":"`+f.username+`","password":"`+f.password+`"}`, nil)
	requireStatus(t, response, 500)
	assertError(t, response, "ERR_INTERNAL")
	if !strings.Contains(f.logs.String(), "auth operation failed") || strings.Contains(f.logs.String(), "secret-invalid-PHC") {
		t.Fatal("internal auth error must be explicit and redacted")
	}
	body, err := io.ReadAll(response.Result().Body)
	if err != nil || bytes.Contains(body, []byte("secret-invalid-PHC")) {
		t.Fatal("internal auth details leaked")
	}
}
