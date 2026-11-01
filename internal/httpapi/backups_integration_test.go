package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"pandatable/internal/acl"
	"pandatable/internal/admin"
	"pandatable/internal/auth"
	"pandatable/internal/backup"
)

func TestBackupHTTPAuthorizationDownloadAndDelete(t *testing.T) {
	f := newHTTPAuthFixture(t)
	authService, err := auth.New(f.pool)
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := acl.NewResolver(f.pool)
	if err != nil {
		t.Fatal(err)
	}
	adminService, err := admin.NewService(f.pool, authService, resolver)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	manager, err := backup.New(ctx, f.pool, authDatabase.URL(t), dir, "0.0.0-dev",
		slog.New(slog.NewJSONHandler(&f.logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := manager.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	f.handler, err = New(slog.New(slog.NewJSONHandler(&f.logs, nil)), testAssets(),
		AuthOptions{Service: authService, Admin: adminService, Backups: manager, BaseURL: "https://table.example"})
	if err != nil {
		t.Fatal(err)
	}
	const fileName = "pandatable-20261101-010203.zip"
	content := []byte("backup archive fixture")
	if err := os.WriteFile(filepath.Join(dir, "backups", fileName), content, 0o600); err != nil {
		t.Fatal(err)
	}
	var id string
	if err := f.pool.QueryRow(ctx, `INSERT INTO meta.backups(file_name,size_bytes,status,created_by,finished_at)
		VALUES ($1,$2,'done',$3,now()) RETURNING id::text`, fileName, len(content), f.id).Scan(&id); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := f.pool.Exec(context.Background(), "DELETE FROM meta.backups WHERE id=$1", id); err != nil {
			t.Error(err)
		}
	})
	cookie := f.login(t, f.username, f.password)
	for _, route := range []struct{ method, path, body string }{
		{"GET", "/admin/backups", ""},
		{"POST", "/admin/backups", "{}"},
		{"GET", "/admin/backups/" + id, ""},
		{"GET", "/admin/backups/" + id + "/download", ""},
		{"DELETE", "/admin/backups/" + id, ""},
	} {
		requireStatus(t, f.request(route.method, route.path, route.body, nil), http.StatusUnauthorized)
		response := f.request(route.method, route.path, route.body, cookie)
		requireStatus(t, response, http.StatusForbidden)
		assertError(t, response, "ERR_PERMISSION_DENIED")
	}
	if _, err := f.pool.Exec(ctx, "UPDATE meta.users SET role='admin' WHERE id=$1", f.id); err != nil {
		t.Fatal(err)
	}
	response := f.request("GET", "/admin/backups", "", cookie)
	requireStatus(t, response, http.StatusOK)
	var listing backup.Listing
	if err := json.Unmarshal(response.Body.Bytes(), &listing); err != nil || len(listing.Items) != 1 ||
		listing.Items[0].ID != id || listing.Items[0].CreatedByName != "HTTP fixture" || listing.FreeBytes == 0 {
		t.Fatalf("invalid backup listing: %+v %v", listing, err)
	}
	response = f.request("GET", "/admin/backups/"+id+"/download", "", cookie)
	requireStatus(t, response, http.StatusOK)
	if response.Body.String() != string(content) || response.Header().Get("Content-Length") != strconv.Itoa(len(content)) ||
		response.Header().Get("Content-Type") != "application/zip" ||
		!strings.Contains(response.Header().Get("Content-Disposition"), fileName) {
		t.Fatalf("invalid backup download headers or body: %v", response.Header())
	}
	if err := os.WriteFile(filepath.Join(dir, "backups", fileName), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	response = f.request("GET", "/admin/backups/"+id+"/download", "", cookie)
	requireStatus(t, response, http.StatusConflict)
	assertError(t, response, "ERR_BACKUP_UNAVAILABLE")
	if err := os.WriteFile(filepath.Join(dir, "backups", fileName), content, 0o600); err != nil {
		t.Fatal(err)
	}
	badCSRF := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/backups/"+id, nil)
	badCSRF.AddCookie(cookie)
	response = httptest.NewRecorder()
	f.handler.ServeHTTP(response, badCSRF)
	requireStatus(t, response, http.StatusForbidden)
	assertError(t, response, "ERR_CSRF")
	response = f.request("DELETE", "/admin/backups/"+id, "", cookie)
	requireStatus(t, response, http.StatusOK)
	if _, err := os.Stat(filepath.Join(dir, "backups", fileName)); !os.IsNotExist(err) {
		t.Fatalf("deleted archive remains: %v", err)
	}
	requireStatus(t, f.request("GET", "/admin/backups/"+id, "", cookie), http.StatusNotFound)
}
