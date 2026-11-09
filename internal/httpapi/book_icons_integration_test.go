package httpapi

import (
	"encoding/json"
	"testing"

	"pandatable/internal/acl"
	"pandatable/internal/structure"
)

func TestBookIconAPIAndNavigation(t *testing.T) {
	f := newACLFixture(t)
	owner := f.user("user", false)
	workspace := f.newWorkspace(owner, "Icon workspace")
	response := f.api(owner, "POST", "/workspaces/"+workspace.ID+"/books", `{"name":"Name-only"}`)
	book := decode[bookBody](f, response, 201).Book
	if book.Icon != nil || !isFallbackBookIcon(book.FallbackIcon) {
		t.Fatalf("name-only book = %+v", book)
	}
	var createdWire struct {
		Book map[string]json.RawMessage `json:"book"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &createdWire); err != nil {
		t.Fatal(err)
	}
	assertBookIconWire(t, createdWire.Book, nil, book.FallbackIcon)

	customCreation := f.api(owner, "POST", "/workspaces/"+workspace.ID+"/books", `{"name":"Custom","icon":"link"}`)
	created := decode[bookBody](f, customCreation, 201).Book
	if created.Icon == nil || *created.Icon != "link" || !isFallbackBookIcon(created.FallbackIcon) {
		t.Fatalf("custom book creation = %+v", created)
	}
	f.expectStatus(f.api(owner, "POST", "/workspaces/"+workspace.ID+"/books", `{"name":"Invalid","icon":"not-an-icon"}`), 400, "ERR_VALIDATION")

	set := decode[bookBody](f, f.api(owner, "PATCH", "/books/"+book.ID, `{"icon":"star"}`), 200).Book
	if set.Icon == nil || *set.Icon != "star" || set.FallbackIcon != book.FallbackIcon {
		t.Fatalf("setting custom icon = %+v", set)
	}
	rename := decode[bookBody](f, f.api(owner, "PATCH", "/books/"+book.ID, `{"name":"Renamed","index":1}`), 200).Book
	if rename.Name != "Renamed" || rename.Icon == nil || *rename.Icon != "star" ||
		rename.FallbackIcon != book.FallbackIcon {
		t.Fatalf("name/index patch changed icon = %+v", rename)
	}
	cleared := decode[bookBody](f, f.api(owner, "PATCH", "/books/"+book.ID, `{"icon":null}`), 200).Book
	if cleared.Icon != nil || cleared.FallbackIcon != book.FallbackIcon {
		t.Fatalf("clearing custom icon changed fallback = %+v", cleared)
	}
	f.expectStatus(f.api(owner, "PATCH", "/books/"+book.ID, `{"icon":"not-an-icon"}`), 400, "ERR_VALIDATION")
	f.expectStatus(f.api(owner, "PATCH", "/books/"+book.ID, `{"icon":7}`), 400, "ERR_VALIDATION")

	reader, stranger := f.user("user", false), f.user("user", false)
	f.grantOK(owner, acl.Workspace, workspace.ID, "user", reader.id, acl.Read)
	navResponse := f.api(reader, "GET", "/nav", "")
	nav := decode[structure.Nav](f, navResponse, 200)
	if len(nav.Workspaces) != 1 || len(nav.Workspaces[0].Books) != 2 {
		t.Fatalf("reader navigation = %+v", nav)
	}
	var navWire struct {
		Workspaces []struct {
			Books []map[string]json.RawMessage `json:"books"`
		} `json:"workspaces"`
	}
	if err := json.Unmarshal(navResponse.Body.Bytes(), &navWire); err != nil {
		t.Fatal(err)
	}
	found, foundCustom := false, false
	for _, visible := range navWire.Workspaces[0].Books {
		var id string
		if err := json.Unmarshal(visible["id"], &id); err != nil {
			t.Fatal(err)
		}
		if id == book.ID {
			assertBookIconWire(t, visible, nil, book.FallbackIcon)
			found = true
		}
		if id == created.ID {
			assertBookIconWire(t, visible, created.Icon, created.FallbackIcon)
			foundCustom = true
		}
	}
	if !found || !foundCustom {
		t.Fatal("reader navigation omitted a book")
	}
	f.expectStatus(f.api(reader, "PATCH", "/books/"+book.ID, `{"icon":"calendar"}`), 403, "ERR_PERMISSION_DENIED")
	f.expectStatus(f.api(stranger, "PATCH", "/books/"+book.ID, `{"icon":"calendar"}`), 404, "ERR_NOT_FOUND")
	if nav := f.nav(stranger); len(nav.Workspaces) != 0 {
		t.Fatalf("unauthorized user saw book icon metadata: %+v", nav)
	}
}

func assertBookIconWire(t *testing.T, fields map[string]json.RawMessage, wantIcon *string, wantFallback string) {
	t.Helper()
	rawIcon, ok := fields["icon"]
	if !ok {
		t.Fatal("book JSON omitted icon")
	}
	var icon *string
	if err := json.Unmarshal(rawIcon, &icon); err != nil {
		t.Fatal(err)
	}
	if (icon == nil) != (wantIcon == nil) || icon != nil && *icon != *wantIcon {
		t.Fatalf("book JSON icon = %v, want %v", icon, wantIcon)
	}
	rawFallback, ok := fields["fallbackIcon"]
	if !ok {
		t.Fatal("book JSON omitted fallbackIcon")
	}
	var fallback string
	if err := json.Unmarshal(rawFallback, &fallback); err != nil {
		t.Fatal(err)
	}
	if fallback != wantFallback {
		t.Fatalf("book JSON fallbackIcon = %q, want %q", fallback, wantFallback)
	}
}

func isFallbackBookIcon(icon string) bool {
	switch icon {
	case "book", "table", "view", "fields", "calendar", "star", "group":
		return true
	default:
		return false
	}
}
