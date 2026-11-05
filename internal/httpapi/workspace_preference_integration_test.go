package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"pandatable/internal/acl"
	"pandatable/internal/structure"
)

const workspacePreferencePath = "/auth/workspace-preference"

type workspacePreferenceResponse struct {
	structure.WorkspacePreference
	EventID string `json:"eventId"`
}

func resolveWorkspacePreference(t *testing.T, f *aclFixture, user *aclUser) string {
	t.Helper()
	response := decode[workspacePreferenceResponse](f, f.api(user, "POST", workspacePreferencePath+"/resolve", `{}`), 200)
	if response.WorkspaceID == nil || response.EventID == "" {
		t.Fatalf("resolve response = %+v", response)
	}
	return *response.WorkspaceID
}

func putWorkspacePreference(f *aclFixture, user *aclUser, id string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"workspaceId": id})
	return f.api(user, "PUT", workspacePreferencePath, string(body))
}

func TestWorkspacePreferenceHTTPContractAndLifecycle(t *testing.T) {
	f := newACLFixture(t)
	owner, reader := f.user("user", false), f.user("user", false)
	admin, forced := f.user("admin", false), f.user("user", true)
	old := f.newWorkspace(owner, "Existing workspace")

	for _, route := range []struct{ method, path, body string }{
		{"GET", workspacePreferencePath, ""},
		{"PUT", workspacePreferencePath, `{"workspaceId":"bad"}`},
		{"POST", workspacePreferencePath + "/resolve", `{}`},
	} {
		f.expectStatus(f.api(nil, route.method, route.path, route.body), 401, "ERR_UNAUTHENTICATED")
		f.expectStatus(f.api(forced, route.method, route.path, route.body), 403, "ERR_PASSWORD_CHANGE_REQUIRED")
	}
	f.expectStatus(f.api(owner, "GET", workspacePreferencePath+"?extra=1", ""), 400, "ERR_VALIDATION")
	for _, body := range []string{`null`, `[]`, `{"extra":true}`, `{} {}`} {
		f.expectStatus(f.api(owner, "POST", workspacePreferencePath+"/resolve", body), 400, "ERR_VALIDATION")
	}
	empty := decode[structure.WorkspacePreference](f, f.api(owner, "GET", workspacePreferencePath, ""), 200)
	if empty.WorkspaceID != nil {
		t.Fatalf("uninitialized preference = %+v", empty)
	}
	request := httptest.NewRequest("POST", "/api/v1"+workspacePreferencePath+"/resolve", strings.NewReader(`{}`))
	request.AddCookie(owner.cookie)
	request.Header.Set("Content-Type", "application/json")
	missingCSRF := httptest.NewRecorder()
	f.handler.ServeHTTP(missingCSRF, request)
	f.expectStatus(missingCSRF, 403, "ERR_CSRF")

	starter := resolveWorkspacePreference(t, f, owner)
	f.spaces = append(f.spaces, starter)
	if starter == old.ID || resolveWorkspacePreference(t, f, owner) != starter {
		t.Fatal("legacy bootstrap did not create exactly one distinct starter")
	}
	nav := f.nav(owner)
	if len(nav.Workspaces) != 2 || nav.Workspaces[1].ID != starter || nav.Workspaces[1].Name != "Default" ||
		nav.Workspaces[1].Level != "owner" {
		t.Fatalf("bootstrapped navigation = %+v", nav)
	}
	f.expectStatus(f.api(owner, "PATCH", "/workspaces/"+starter, `{"name":"Renamed"}`), 200, "")
	if resolveWorkspacePreference(t, f, owner) != starter {
		t.Fatal("renaming the starter caused another bootstrap")
	}
	ownerSelection := decode[workspacePreferenceResponse](f, putWorkspacePreference(f, owner, old.ID), 200)
	if ownerSelection.WorkspaceID == nil || *ownerSelection.WorkspaceID != old.ID || ownerSelection.EventID == "" {
		t.Fatalf("set response = %+v", ownerSelection)
	}

	readerStarter := resolveWorkspacePreference(t, f, reader)
	f.spaces = append(f.spaces, readerStarter)
	grant := f.grantOK(owner, acl.Workspace, old.ID, "user", reader.id, acl.Read)
	selected := decode[workspacePreferenceResponse](f, putWorkspacePreference(f, reader, old.ID), 200)
	if selected.WorkspaceID == nil || *selected.WorkspaceID != old.ID {
		t.Fatalf("reader cannot select shared workspace: %+v", selected)
	}
	if got := decode[structure.WorkspacePreference](f, f.api(owner, "GET", workspacePreferencePath, ""), 200); got.WorkspaceID == nil || *got.WorkspaceID != old.ID {
		t.Fatalf("another user's update changed owner preference: %+v", got)
	}
	loginBody, _ := json.Marshal(map[string]string{"login": reader.login, "password": aclPassword})
	login := f.api(nil, "POST", "/auth/login", string(loginBody))
	f.expectStatus(login, 200, "")
	cookies := login.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal("new session missing cookie")
	}
	secondSession := &aclUser{id: reader.id, cookie: cookies[0]}
	if got := decode[structure.WorkspacePreference](f, f.api(secondSession, "GET", workspacePreferencePath, ""), 200); got.WorkspaceID == nil || *got.WorkspaceID != old.ID {
		t.Fatalf("preference did not survive login: %+v", got)
	}

	hidden := f.newWorkspace(owner, "Hidden")
	pathOnly := f.newWorkspace(owner, "Path only")
	book := f.newBook(owner, pathOnly.ID, "Book")
	table := f.newTable(owner, book.ID, "Table")
	f.grantOK(owner, acl.Table, table.Table.ID, "user", reader.id, acl.Read)
	beforeGrants := f.scan("SELECT count(*)::text FROM meta.grants WHERE principal_type='user' AND principal_id=$1", reader.id)
	for _, test := range []struct {
		id     string
		status int
		code   string
	}{{"not-a-uuid", 404, "ERR_NOT_FOUND"}, {hidden.ID, 404, "ERR_NOT_FOUND"},
		{pathOnly.ID, 403, "ERR_PERMISSION_DENIED"}} {
		f.expectStatus(putWorkspacePreference(f, reader, test.id), test.status, test.code)
	}
	f.expectStatus(putWorkspacePreference(f, admin, old.ID), 404, "ERR_NOT_FOUND")
	for _, body := range []string{`{}`, `null`, `{"workspaceId":null}`, `{"workspaceId":1}`,
		`{"workspaceId":"x","workspaceId":"y"}`, `{"workspaceId":"x","extra":1}`} {
		f.expectStatus(f.api(reader, "PUT", workspacePreferencePath, body), 400, "ERR_VALIDATION")
	}
	if after := f.scan("SELECT count(*)::text FROM meta.grants WHERE principal_type='user' AND principal_id=$1", reader.id); after != beforeGrants {
		t.Fatal("invalid preference changed grants")
	}
	if got := decode[structure.WorkspacePreference](f, f.api(reader, "GET", workspacePreferencePath, ""), 200); got.WorkspaceID == nil || *got.WorkspaceID != old.ID {
		t.Fatalf("invalid preference changed selection: %+v", got)
	}
	f.expectStatus(f.api(owner, "DELETE", "/workspaces/"+hidden.ID, ""), 200, "")
	f.expectStatus(putWorkspacePreference(f, reader, hidden.ID), 404, "ERR_NOT_FOUND")

	f.expectStatus(f.api(owner, "DELETE", "/grants/"+grant.Grant.ID, ""), 200, "")
	stale := f.api(reader, "GET", workspacePreferencePath, "")
	f.expectStatus(stale, 200, "")
	if strings.Contains(stale.Body.String(), old.ID) || strings.TrimSpace(stale.Body.String()) != `{"workspaceId":null}` {
		t.Fatalf("stale preference exposed an inaccessible ID: %s", stale.Body)
	}
	if got := resolveWorkspacePreference(t, f, reader); got != readerStarter {
		t.Fatalf("revocation fallback = %s, want %s", got, readerStarter)
	}
	f.expectStatus(f.api(reader, "DELETE", "/workspaces/"+readerStarter, ""), 200, "")
	replacement := resolveWorkspacePreference(t, f, reader)
	f.spaces = append(f.spaces, replacement)
	if replacement == readerStarter || replacement == pathOnly.ID {
		t.Fatal("resolution selected a deleted or context-only workspace")
	}
}
