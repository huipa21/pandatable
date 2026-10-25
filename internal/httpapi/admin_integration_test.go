package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"pandatable/internal/acl"
	"pandatable/internal/admin"
	"pandatable/internal/auth"
)

func TestAdminUsersHTTPAuthorizationAndContract(t *testing.T) {
	f := newHTTPAuthFixture(t)
	service, err := auth.New(f.pool)
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := acl.NewResolver(f.pool)
	if err != nil {
		t.Fatal(err)
	}
	adminService, err := admin.NewService(f.pool, service, resolver)
	if err != nil {
		t.Fatal(err)
	}
	f.handler, err = New(slog.New(slog.NewJSONHandler(&f.logs, nil)), testAssets(),
		AuthOptions{Service: service, Admin: adminService, BaseURL: "https://table.example"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	userCookie := f.login(t, f.username, f.password)
	for _, route := range []struct{ method, path, body string }{
		{"GET", "/admin/users", ""},
		{"POST", "/admin/users", "{}"},
		{"PATCH", "/admin/users/" + f.id, "{}"},
		{"POST", "/admin/users/" + f.id + "/reset-password", "{}"},
		{"GET", "/admin/future", ""},
	} {
		requireStatus(t, f.request(route.method, route.path, route.body, nil), 401)
		response := f.request(route.method, route.path, route.body, userCookie)
		requireStatus(t, response, 403)
		assertError(t, response, "ERR_PERMISSION_DENIED")
	}
	if _, err := f.pool.Exec(ctx, "UPDATE meta.users SET role='admin',must_change_password=true WHERE id=$1", f.id); err != nil {
		t.Fatal(err)
	}
	response := f.request("GET", "/admin/users", "", userCookie)
	requireStatus(t, response, 403)
	assertError(t, response, "ERR_PASSWORD_CHANGE_REQUIRED")
	if _, err := f.pool.Exec(ctx, "UPDATE meta.users SET must_change_password=false WHERE id=$1", f.id); err != nil {
		t.Fatal(err)
	}
	response = f.request("GET", "/admin/users?limit=500", "", userCookie)
	requireStatus(t, response, 200)
	var listing admin.Listing
	if err := json.Unmarshal(response.Body.Bytes(), &listing); err != nil || listing.Total != 1 || len(listing.Items) != 1 {
		t.Fatal("invalid admin listing", err)
	}
	for _, query := range []string{"limit=501", "limit=0", "offset=-1", "limit=x", "status=bad", "q=a&q=b", "unknown=1",
		"q=%ZZ", "q=bad;value"} {
		requireStatus(t, f.request("GET", "/admin/users?"+query, "", userCookie), 400)
	}
	for _, body := range []string{`{}`, `{"username":null}`, `{"role":null}`, `{"email":123}`,
		`{"displayName":null}`, `{"password":"forbidden"}`, `{"status":null}`} {
		requireStatus(t, f.request("PATCH", "/admin/users/"+f.id, body, userCookie), 400)
	}
	for _, path := range []string{"/admin/users", "/admin/users/" + f.id, "/admin/users/" + f.id + "/reset-password"} {
		method := "POST"
		if path == "/admin/users/"+f.id {
			method = "PATCH"
		}
		request := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader("{}"))
		request.AddCookie(userCookie)
		request.Header.Set("Content-Type", "application/json")
		bad := httptest.NewRecorder()
		f.handler.ServeHTTP(bad, request)
		requireStatus(t, bad, 403)
		assertError(t, bad, "ERR_CSRF")
	}
	payload := `{"username":"http-alice","email":" Alice@Example.Test ","displayName":"Alice","role":"user","password":"Temporary-password-123"}`
	response = f.request("POST", "/admin/users", payload, userCookie)
	requireStatus(t, response, 201)
	var created struct {
		User    admin.User `json:"user"`
		EventID string     `json:"eventId"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil || created.EventID == "" ||
		!created.User.MustChangePassword || created.User.Email == nil || *created.User.Email != "alice@example.test" {
		t.Fatal("incorrect create DTO", err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if _, err := f.pool.Exec(cleanup, "DELETE FROM meta.users WHERE id=$1", created.User.ID); err != nil {
			t.Error(err)
		}
	})
	for _, secret := range []string{"Temporary-password-123", "password_hash", "id_hash", "$argon2id$"} {
		if strings.Contains(response.Body.String(), secret) || strings.Contains(f.logs.String(), secret) {
			t.Fatal("admin API leaked password/session metadata")
		}
	}
	response = f.request("POST", "/admin/users", payload, userCookie)
	requireStatus(t, response, 409)
	assertError(t, response, "ERR_IDENTITY_CONFLICT")
	response = f.request("PATCH", "/admin/users/"+created.User.ID, `{"email":null,"displayName":"New name","status":"disabled"}`, userCookie)
	requireStatus(t, response, 200)
	if !strings.Contains(response.Body.String(), `"sessionsRevoked":true`) || !strings.Contains(response.Body.String(), `"email":null`) {
		t.Fatal("invalid PATCH response")
	}
	requireStatus(t, f.request("POST", "/admin/users/"+created.User.ID+"/reset-password",
		`{"password":"Reset-password-123"}`, userCookie), 200)
	response = f.request("PATCH", "/admin/users/"+f.id, `{"status":"disabled"}`, userCookie)
	requireStatus(t, response, 409)
	assertError(t, response, "ERR_LAST_ADMIN")
	for _, body := range []string{strings.Repeat("x", 4097), `{"displayName":"Good"} {}`, `{"unknown":true}`} {
		requireStatus(t, f.request("PATCH", "/admin/users/"+f.id, body, userCookie), 400)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users", strings.NewReader(payload))
	request.AddCookie(userCookie)
	request.Header.Set("Origin", "https://table.example")
	request.Header.Set("X-PT-CSRF", "1")
	response = httptest.NewRecorder()
	f.handler.ServeHTTP(response, request)
	requireStatus(t, response, 415)
}
