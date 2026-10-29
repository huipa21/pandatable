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

func TestAdminSettingsWorkspacesHTTP(t *testing.T) {
	f := newHTTPAuthFixture(t)
	service, err := auth.New(f.pool)
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := acl.NewResolver(f.pool)
	if err != nil {
		t.Fatal(err)
	}
	management, err := admin.NewService(f.pool, service, resolver)
	if err != nil {
		t.Fatal(err)
	}
	f.handler, err = New(slog.New(slog.NewJSONHandler(&f.logs, nil)), testAssets(),
		AuthOptions{Service: service, Admin: management, BaseURL: "https://table.example"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if _, err := f.pool.Exec(cleanup, "DELETE FROM meta.settings WHERE key IN ('app_name','default_timezone','upload_max_mb','image_max_megapixels')"); err != nil {
			t.Error(err)
		}
	})
	public := f.request("GET", "/meta/app", "", nil)
	requireStatus(t, public, 200)
	var keys map[string]any
	if err := json.Unmarshal(public.Body.Bytes(), &keys); err != nil || len(keys) != 2 ||
		keys["appName"] != "PandaTable" || keys["defaultLocale"] != "en" {
		t.Fatal("public DTO must contain only branding and locale", err)
	}
	cookie := f.login(t, f.username, f.password)
	for _, route := range []struct{ method, path, body string }{
		{"GET", "/admin/settings", ""}, {"PATCH", "/admin/settings", `{"appName":"Denied"}`},
		{"GET", "/admin/workspaces", ""},
	} {
		requireStatus(t, f.request(route.method, route.path, route.body, nil), 401)
		for _, state := range []struct {
			role, status, want string
			forced             bool
		}{
			{"user", "active", "ERR_PERMISSION_DENIED", false},
			{"admin", "active", "ERR_PASSWORD_CHANGE_REQUIRED", true},
			{"admin", "disabled", "ERR_UNAUTHENTICATED", false},
		} {
			if _, err := f.pool.Exec(ctx, "UPDATE meta.users SET role=$2,status=$3,must_change_password=$4 WHERE id=$1",
				f.id, state.role, state.status, state.forced); err != nil {
				t.Fatal(err)
			}
			response := f.request(route.method, route.path, route.body, cookie)
			want := 403
			if state.status == "disabled" {
				want = 401
			}
			requireStatus(t, response, want)
			assertError(t, response, state.want)
		}
	}
	if _, err := f.pool.Exec(ctx, "UPDATE meta.users SET role='admin',status='active',must_change_password=false WHERE id=$1", f.id); err != nil {
		t.Fatal(err)
	}
	requireStatus(t, f.request("GET", "/admin/settings", "", cookie), 200)
	for _, body := range []string{`{}`, `null`, `{"appName":null}`, `{"defaultLocale":null}`, `{"defaultTimezone":null}`,
		`{"defaultCurrency":null}`, `{"uploadMaxMb":null}`, `{"imageMaxMegapixels":null}`, `{"historyRetentionDays":null}`, `{"trashRetentionDays":null}`,
		`{"uploadMaxMb":"20"}`, `{"uploadMaxMb":1.1}`, `{"uploadMaxMb":1e100}`, `{"uploadMaxMb":101}`,
		`{"setup_completed":true}`, `{"unknown":1}`, `{"appName":"ok"} {}`, `{"appName":"` + strings.Repeat("x", 4096) + `"}`,
		`{"appName":"Rollback","defaultTimezone":"Invalid/Zone"}`} {
		requireStatus(t, f.request("PATCH", "/admin/settings", body, cookie), 400)
	}
	for _, headers := range []map[string]string{
		{}, {"Origin": "https://foreign.example", "X-PT-CSRF": "1"},
		{"Origin": "https://table.example"}, {"X-PT-CSRF": "1"},
	} {
		request := httptest.NewRequest("PATCH", "/api/v1/admin/settings", strings.NewReader(`{"appName":"Denied"}`))
		request.AddCookie(cookie)
		request.Header.Set("Content-Type", "application/json")
		for key, value := range headers {
			request.Header.Set(key, value)
		}
		response := httptest.NewRecorder()
		f.handler.ServeHTTP(response, request)
		requireStatus(t, response, 403)
		assertError(t, response, "ERR_CSRF")
	}
	response := f.request("PATCH", "/admin/settings", `{"appName":" Team <plain> ","uploadMaxMb":7,"defaultTimezone":"Europe/London"}`, cookie)
	requireStatus(t, response, 200)
	var mutation struct {
		Settings admin.Settings `json:"settings"`
		EventID  string         `json:"eventId"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &mutation); err != nil || mutation.EventID == "" ||
		mutation.Settings.AppName != "Team <plain>" || mutation.Settings.UploadMaxMb != 7 {
		t.Fatal("invalid settings mutation", err)
	}
	requireStatus(t, f.request("GET", "/meta/app", "", nil), 200)
	response = f.request("GET", "/auth/me", "", cookie)
	requireStatus(t, response, 200)
	if !strings.Contains(response.Body.String(), `"effectiveTimezone":"Europe/London"`) {
		t.Fatal("actual auth reader not wired")
	}
	response = f.request("GET", "/admin/workspaces", "", cookie)
	requireStatus(t, response, 200)
	if strings.TrimSpace(response.Body.String()) != `{"items":[],"total":0}` {
		t.Fatal("empty inventory shape", response.Body)
	}
	for _, query := range []string{"limit=501", "offset=-1", "limit=x", "status=active", "q=a&q=b", "unknown=1", "q=%ZZ", "q=bad;value"} {
		requireStatus(t, f.request("GET", "/admin/workspaces?"+query, "", cookie), 400)
	}
	for _, path := range []string{"/admin/settings?unknown=1", "/meta/app?unknown=1", "/meta/currencies?unknown=1"} {
		requireStatus(t, f.request("GET", path, "", cookie), 400)
	}
	requireStatus(t, f.request("GET", "/meta/currencies", "", nil), 401)
	requireStatus(t, f.request("GET", "/meta/currencies", "", cookie), 200)
	for _, secret := range []string{f.password, f.storedPHC, cookie.Value, "postgres://", "storage_path"} {
		if strings.Contains(response.Body.String(), secret) || strings.Contains(f.logs.String(), secret) {
			t.Fatal("secret in settings/inventory DTO or logs")
		}
	}

}

func TestAdminInventoryPreservesOwnerOnlyResourceAccess(t *testing.T) {
	f := newHTTPAuthFixture(t)
	service, err := auth.New(f.pool)
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := acl.NewResolver(f.pool)
	if err != nil {
		t.Fatal(err)
	}
	grants, err := acl.NewService(f.pool, resolver)
	if err != nil {
		t.Fatal(err)
	}
	management, err := admin.NewService(f.pool, service, resolver)
	if err != nil {
		t.Fatal(err)
	}
	f.handler, err = New(slog.New(slog.NewJSONHandler(&f.logs, nil)), testAssets(),
		AuthOptions{Service: service, Admin: management, ACL: grants, BaseURL: "https://table.example"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	alice := f.login(t, f.username, f.password)
	var workspace, adminID string
	if err := f.pool.QueryRow(ctx, `INSERT INTO meta.users(username,display_name,password_hash,role,avatar_color)
			VALUES ($1,'Inventory admin',$2,'admin','teal') RETURNING id::text`, "admin-"+f.username, f.storedPHC).Scan(&adminID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		for _, query := range []struct {
			sql string
			id  string
		}{{"DELETE FROM meta.grants WHERE resource_id=$1", workspace},
			{"DELETE FROM meta.workspaces WHERE id=$1", workspace}, {"DELETE FROM meta.users WHERE id=$1", adminID}} {
			if _, err := f.pool.Exec(cleanup, query.sql, query.id); err != nil {
				t.Error(err)
			}
		}
	})
	if err := f.pool.QueryRow(ctx, "INSERT INTO meta.workspaces(name,created_by) VALUES ('Alice workspace',$1) RETURNING id::text", f.id).Scan(&workspace); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO meta.grants(resource_type,resource_id,principal_type,principal_id,level)
			VALUES ('workspace',$1,'user',$2,'owner')`, workspace, f.id); err != nil {
		t.Fatal(err)
	}
	response := f.request("POST", "/auth/login", `{"login":"admin-`+f.username+`","password":"`+f.password+`"}`, nil)
	requireStatus(t, response, 200)
	cookie := response.Result().Cookies()[0]
	response = f.request("GET", "/admin/workspaces", "", cookie)
	requireStatus(t, response, 200)
	var inventory admin.WorkspaceListing
	if err := json.Unmarshal(response.Body.Bytes(), &inventory); err != nil || len(inventory.Items) != 1 ||
		inventory.Items[0].Name != "Alice workspace" || inventory.Items[0].Orphaned ||
		len(inventory.Items[0].Owners) != 1 || inventory.Items[0].Owners[0].ID != f.id {
		t.Fatal("admin inventory owner metadata failed", err)
	}
	for _, test := range []struct {
		cookie *http.Cookie
		status int
	}{{cookie, 404}, {alice, 200}} {
		requireStatus(t, f.request("GET", "/grants?resource=workspace:"+workspace, "", test.cookie), test.status)
	}
	resource, err := acl.NewResource("workspace", workspace)
	if err != nil {
		t.Fatal(err)
	}
	if err := resolver.Require(ctx, adminID, acl.Read, resource); err == nil {
		t.Fatal("listing conferred data access")
	}
	requireStatus(t, f.request("GET", "/admin/workspaces", "", alice), 403)
}
