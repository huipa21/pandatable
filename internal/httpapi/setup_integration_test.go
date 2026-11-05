package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"pandatable/internal/auth"
)

func newHTTPSetupFixture(t *testing.T) (*httpAuthFixture, string) {
	t.Helper()
	pool := authDatabase.Open(t)
	service, err := auth.New(pool)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	token, needed, err := service.InitializeSetup(ctx)
	if err != nil || !needed || token == "" {
		t.Fatalf("initialize HTTP setup: needed=%t token=%t err=%v", needed, token != "", err)
	}
	f := &httpAuthFixture{pool: pool}
	handler, err := New(slog.New(slog.NewJSONHandler(&f.logs, nil)), testAssets(), AuthOptions{
		Service: service, BaseURL: "https://table.example",
	})
	if err != nil {
		t.Fatal(err)
	}
	f.handler = handler
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM meta.grants WHERE resource_type='workspace'
			AND resource_id IN (SELECT w.id FROM meta.workspaces w JOIN meta.users u ON u.id=w.created_by
				WHERE u.email LIKE '%@http-setup.test');
			DELETE FROM meta.workspaces WHERE created_by IN (SELECT id FROM meta.users WHERE email LIKE '%@http-setup.test')`); err != nil {
			t.Error("cleanup HTTP setup workspaces:", err)
		}
		if _, err := pool.Exec(cleanupCtx, "DELETE FROM meta.users WHERE email LIKE '%@http-setup.test'"); err != nil {
			t.Error("cleanup HTTP setup user:", err)
		}
		if _, err := pool.Exec(cleanupCtx, "DELETE FROM meta.settings WHERE key='setup_completed'"); err != nil {
			t.Error("cleanup HTTP setup marker:", err)
		}
	})
	return f, token
}

func TestSetupHTTPPostgreSQL(t *testing.T) {
	f, token := newHTTPSetupFixture(t)
	status := f.request("GET", "/setup/status", "", nil)
	requireStatus(t, status, http.StatusOK)
	if strings.TrimSpace(status.Body.String()) != `{"needed":true}` {
		t.Fatalf("empty setup status=%s", status.Body.String())
	}
	wrongBody := `{"token":"wrong","username":"setup-admin","email":"owner@http-setup.test","displayName":"First Admin","password":"12345678","locale":"en"}`
	wrong := f.request("POST", "/setup", wrongBody, nil)
	requireStatus(t, wrong, http.StatusUnauthorized)
	assertError(t, wrong, "ERR_SETUP_TOKEN")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var partial int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM meta.users`).Scan(&partial); err != nil || partial != 0 {
		t.Fatalf("wrong token created a user: count=%d err=%v", partial, err)
	}
	body, err := json.Marshal(map[string]string{
		"token": token, "username": " Setup-Admin ", "email": " OWNER@HTTP-SETUP.TEST ",
		"displayName": "First Admin", "password": "12345678", "locale": "en",
	})
	if err != nil {
		t.Fatal(err)
	}
	created := f.request("POST", "/setup", string(body), nil)
	requireStatus(t, created, http.StatusOK)
	cookies := created.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != sessionCookie || cookies[0].Value == "" ||
		!cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].SameSite != http.SameSiteLaxMode || cookies[0].Path != "/" {
		t.Fatal("setup did not issue the existing secure session cookie")
	}
	for _, secret := range []string{token, "12345678", cookies[0].Value, "password_hash", "id_hash"} {
		if strings.Contains(created.Body.String(), secret) {
			t.Fatal("setup response exposed a token, password, or storage field")
		}
	}
	me := f.request("GET", "/auth/me", "", cookies[0])
	requireStatus(t, me, http.StatusOK)
	for _, value := range []string{`"username":"setup-admin"`, `"email":"owner@http-setup.test"`, `"role":"admin"`} {
		if !strings.Contains(me.Body.String(), value) {
			t.Fatalf("setup session identity missing %s: %s", value, me.Body.String())
		}
	}
	status = f.request("GET", "/setup/status", "", nil)
	requireStatus(t, status, http.StatusOK)
	if strings.TrimSpace(status.Body.String()) != `{"needed":false}` {
		t.Fatalf("completed setup status=%s", status.Body.String())
	}
	again := f.request("POST", "/setup", string(body), nil)
	requireStatus(t, again, http.StatusGone)
	assertError(t, again, "ERR_SETUP_COMPLETE")
	for _, secret := range []string{token, "12345678", cookies[0].Value, "owner@http-setup.test"} {
		if strings.Contains(f.logs.String(), secret) {
			t.Fatal("setup request logs exposed a token, password, session or email")
		}
	}
	t.Log("AT-101 HTTP surface: status, wrong-token atomicity, first-admin session cookie, identity and completed 410 verified")
}

func TestSetupHTTPRequiresCSRFAndOrigin(t *testing.T) {
	f, _ := newHTTPSetupFixture(t)
	for _, test := range []struct{ origin, csrf string }{
		{"", ""}, {"https://table.example", ""}, {"", "1"}, {"https://evil.example", "1"},
	} {
		request := httptest.NewRequest("POST", "/api/v1/setup", strings.NewReader(`{}`))
		request.RemoteAddr = netip.MustParseAddr("192.0.2.21").String() + ":1234"
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", test.origin)
		request.Header.Set("X-PT-CSRF", test.csrf)
		response := httptest.NewRecorder()
		f.handler.ServeHTTP(response, request)
		requireStatus(t, response, http.StatusForbidden)
		assertError(t, response, "ERR_CSRF")
	}
}
