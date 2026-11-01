package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pandatable/internal/acl"
	"pandatable/internal/admin"
	"pandatable/internal/auth"
)

func TestAdminSystemHTTPAuthorizationAndContract(t *testing.T) {
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
	dir := t.TempDir()
	for _, name := range []string{"files", "backups"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	logger := slog.New(slog.NewJSONHandler(&f.logs, nil))
	monitor, err := admin.NewSystemMonitor(context.Background(), management, dir, "http-test-build", time.Now(), logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(monitor.Close)
	f.handler, err = New(logger, testAssets(), AuthOptions{
		Service: service, Admin: management, System: monitor, BaseURL: "https://table.example",
	})
	if err != nil {
		t.Fatal(err)
	}
	cookie := f.login(t, f.username, f.password)
	requireStatus(t, f.request("GET", "/admin/system", "", nil), 401)
	response := f.request("GET", "/admin/system", "", cookie)
	requireStatus(t, response, 403)
	assertError(t, response, "ERR_PERMISSION_DENIED")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for _, state := range []struct {
		role, status, forced, code string
		want                       int
	}{
		{"admin", "active", "true", "ERR_PASSWORD_CHANGE_REQUIRED", 403},
		{"admin", "disabled", "false", "ERR_UNAUTHENTICATED", 401},
		{"admin", "active", "false", "", 200},
	} {
		if _, err := f.pool.Exec(ctx, "UPDATE meta.users SET role=$2,status=$3,must_change_password=$4 WHERE id=$1",
			f.id, state.role, state.status, state.forced == "true"); err != nil {
			t.Fatal(err)
		}
		response = f.request("GET", "/admin/system", "", cookie)
		requireStatus(t, response, state.want)
		if state.code != "" {
			assertError(t, response, state.code)
		}
	}
	var result admin.SystemSnapshot
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.CollectedAt.IsZero() ||
		result.Database.Value == nil || result.Versions.Migration.Value == nil || *result.Versions.Migration.Value != 5 ||
		result.FilesBytes.Value == nil || *result.FilesBytes.Value != 0 {
		t.Fatal("invalid actual system metrics DTO", err)
	}
	if cached := f.request("GET", "/admin/system", "", cookie); cached.Body.String() != response.Body.String() {
		t.Fatal("snapshot cache must be shared across requests")
	}
	for _, secret := range []string{dir, cookie.Value, f.password, f.storedPHC, "postgres://", "password_hash", "id_hash"} {
		if strings.Contains(response.Body.String(), secret) || strings.Contains(f.logs.String(), secret) {
			t.Fatal("metrics leaked secrets or host paths")
		}
	}
	requireStatus(t, f.request("GET", "/admin/system?refresh=1", "", cookie), 400)
	requireStatus(t, f.request("POST", "/admin/system", "{}", cookie), http.StatusMethodNotAllowed)
	monitor.Close()
	response = f.request("GET", "/admin/system", "", cookie)
	requireStatus(t, response, 503)
	assertError(t, response, "ERR_SYSTEM_UNAVAILABLE")
	if _, err := f.pool.Exec(ctx, "UPDATE meta.users SET role='user' WHERE id=$1", f.id); err != nil {
		t.Fatal(err)
	}
	response = f.request("GET", "/admin/system", "", cookie)
	requireStatus(t, response, 403)
	assertError(t, response, "ERR_PERMISSION_DENIED")
}
