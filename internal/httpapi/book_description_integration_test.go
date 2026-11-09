package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"pandatable/internal/acl"
	"pandatable/internal/structure"
)

func TestBookDescriptionAPIAndNavigation(t *testing.T) {
	f := newACLFixture(t)
	owner, reader := f.user("user", false), f.user("user", false)
	writer, pathOnly, stranger := f.user("user", false), f.user("user", false), f.user("user", false)
	workspace := f.newWorkspace(owner, "Book descriptions")
	workspaceWithLink := decode[workspaceBody](f, f.api(owner, "PATCH", "/workspaces/"+workspace.ID,
		`{"icon":"link"}`), http.StatusOK).Workspace
	if workspaceWithLink.Icon == nil || *workspaceWithLink.Icon != "link" ||
		workspaceWithLink.FallbackIcon != workspace.FallbackIcon {
		t.Fatalf("custom link workspace icon = %+v", workspaceWithLink)
	}

	emptyResponse := f.api(owner, "POST", "/workspaces/"+workspace.ID+"/books", `{"name":"Empty"}`)
	empty := decode[bookBody](f, emptyResponse, http.StatusCreated)
	if empty.Book.Description != "" || empty.EventID == "" {
		t.Fatalf("empty book response = %+v", empty)
	}
	assertBookDescriptionResponse(t, emptyResponse.Body.Bytes(), "")

	createdResponse := f.api(owner, "POST", "/workspaces/"+workspace.ID+"/books",
		mustJSON(t, map[string]string{"name": "Details", "icon": "link", "description": "  说明 😀\r\nsecond line  "}))
	created := decode[bookBody](f, createdResponse, http.StatusCreated)
	if created.Book.Description != "说明 😀\r\nsecond line" || created.Book.Icon == nil || *created.Book.Icon != "link" ||
		created.EventID == "" {
		t.Fatalf("created book response = %+v", created)
	}
	assertBookDescriptionResponse(t, createdResponse.Body.Bytes(), created.Book.Description)

	limit := strings.Repeat("界😀", 250)
	atLimit := decode[bookBody](f, f.api(owner, "POST", "/workspaces/"+workspace.ID+"/books",
		mustJSON(t, map[string]string{"name": "Unicode limit", "description": limit})), http.StatusCreated)
	if atLimit.Book.Description != limit {
		t.Fatalf("500-code-point description = %q", atLimit.Book.Description)
	}
	for _, invalid := range []any{nil, 7, strings.Repeat("界😀", 250) + "界", "bad\x00value", "bad\tvalue", "\tboundary"} {
		body := mustJSON(t, map[string]any{"name": "Invalid", "description": invalid})
		f.expectStatus(f.api(owner, "POST", "/workspaces/"+workspace.ID+"/books", body),
			http.StatusBadRequest, "ERR_VALIDATION")
	}

	f.grantOK(owner, acl.Workspace, workspace.ID, "user", reader.id, acl.Read)
	f.grantOK(owner, acl.Workspace, workspace.ID, "user", writer.id, acl.Write)
	table := f.newTable(owner, created.Book.ID, "Child")
	f.grantOK(owner, acl.Table, table.Table.ID, "user", pathOnly.id, acl.Read)

	assertNavDescription := func(user *aclUser, want string, path bool) {
		t.Helper()
		response := f.api(user, "GET", "/nav", "")
		nav := decode[structure.Nav](f, response, http.StatusOK)
		var wire struct {
			Workspaces []struct {
				Books []map[string]json.RawMessage `json:"books"`
			} `json:"workspaces"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &wire); err != nil {
			t.Fatal(err)
		}
		for i, space := range nav.Workspaces {
			for j, book := range space.Books {
				if book.ID == created.Book.ID {
					if book.Description != want || book.PathOnly != path {
						t.Fatalf("book nav = %+v, want %q pathOnly %v", book, want, path)
					}
					assertDescriptionField(t, wire.Workspaces[i].Books[j], want)
					return
				}
			}
		}
		t.Fatalf("book missing from nav: %+v", nav)
	}
	assertNavDescription(owner, created.Book.Description, false)
	assertNavDescription(reader, created.Book.Description, false)
	assertNavDescription(pathOnly, "", true)

	patchResponse := f.api(owner, "PATCH", "/books/"+created.Book.ID, `{"description":"  changed 😀  "}`)
	patched := decode[bookBody](f, patchResponse, http.StatusOK)
	if patched.Book.Description != "changed 😀" || patched.EventID == "" {
		t.Fatalf("patched response = %+v", patched)
	}
	assertBookDescriptionResponse(t, patchResponse.Body.Bytes(), "changed 😀")
	assertNavDescription(reader, "changed 😀", false)
	assertNavDescription(pathOnly, "", true)
	renamed := decode[bookBody](f, f.api(owner, "PATCH", "/books/"+created.Book.ID, `{"name":"Renamed"}`), http.StatusOK)
	if renamed.Book.Description != "changed 😀" {
		t.Fatalf("omitted description changed during rename: %+v", renamed)
	}

	for _, invalid := range []any{nil, 7, strings.Repeat("😀", 501), "bad\x00value", "bad\tvalue", "boundary\t"} {
		body := mustJSON(t, map[string]any{"description": invalid})
		f.expectStatus(f.api(owner, "PATCH", "/books/"+created.Book.ID, body),
			http.StatusBadRequest, "ERR_VALIDATION")
	}
	f.expectStatus(f.api(owner, "PATCH", "/books/"+created.Book.ID, `{}`), http.StatusBadRequest, "ERR_VALIDATION")
	f.expectStatus(f.api(reader, "PATCH", "/books/"+created.Book.ID, `{"description":"denied"}`),
		http.StatusForbidden, "ERR_PERMISSION_DENIED")
	f.expectStatus(f.api(writer, "PATCH", "/books/"+created.Book.ID, `{"description":"denied"}`),
		http.StatusForbidden, "ERR_PERMISSION_DENIED")
	f.expectStatus(f.api(pathOnly, "PATCH", "/books/"+created.Book.ID, `{"description":"denied"}`),
		http.StatusForbidden, "ERR_PERMISSION_DENIED")
	f.expectStatus(f.api(stranger, "PATCH", "/books/"+created.Book.ID, `{"description":"hidden"}`),
		http.StatusNotFound, "ERR_NOT_FOUND")

	clearResponse := f.api(owner, "PATCH", "/books/"+created.Book.ID, `{"description":""}`)
	cleared := decode[bookBody](f, clearResponse, http.StatusOK)
	if cleared.Book.Description != "" || cleared.Book.FallbackIcon != created.Book.FallbackIcon {
		t.Fatalf("cleared book = %+v", cleared.Book)
	}
	assertBookDescriptionResponse(t, clearResponse.Body.Bytes(), "")
	assertNavDescription(reader, "", false)
}

func assertBookDescriptionResponse(t *testing.T, body []byte, want string) {
	t.Helper()
	var response struct {
		Book map[string]json.RawMessage `json:"book"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatal(err)
	}
	assertDescriptionField(t, response.Book, want)
}

func assertDescriptionField(t *testing.T, fields map[string]json.RawMessage, want string) {
	t.Helper()
	raw, ok := fields["description"]
	if !ok || string(raw) == "null" {
		t.Fatalf("missing or null description: %s", raw)
	}
	var got string
	if err := json.Unmarshal(raw, &got); err != nil || got != want {
		t.Fatalf("description = %q, %v; want %q", got, err, want)
	}
}
