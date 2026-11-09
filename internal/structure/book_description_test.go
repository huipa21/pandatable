package structure

import (
	"strings"
	"testing"

	"pandatable/internal/acl"
)

func TestBookDescriptionValidation(t *testing.T) {
	tests := []struct {
		name, input, want string
		valid             bool
	}{
		{"empty", "", "", true},
		{"trim and newlines", " \n first\r\nsecond \r ", "first\r\nsecond", true},
		{"unicode boundary", strings.Repeat("界😀", 250), strings.Repeat("界😀", 250), true},
		{"too long", strings.Repeat("界😀", 250) + "界", "", false},
		{"nul", "before\x00after", "", false},
		{"tab inside", "before\tafter", "", false},
		{"tab at boundary", "\tbefore", "", false},
		{"unicode control", "before\u0085", "", false},
		{"invalid utf8", "before\xff", "", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := cleanBookDescription(test.input)
			if test.valid {
				if err != nil || got != test.want {
					t.Fatalf("description = %q, %v; want %q", got, err, test.want)
				}
			} else {
				expectError(t, err, 400, "ERR_VALIDATION")
			}
		})
	}
}

func TestWorkspaceDescriptionRetainsExistingBoundaryTrimming(t *testing.T) {
	got, err := cleanDescription("\t notes\t")
	if err != nil || got != "notes" {
		t.Fatalf("workspace description = %q, %v", got, err)
	}
}

func TestBookDescriptionServiceLifecycleAndAccess(t *testing.T) {
	f := newFixture(t)
	owner := f.user("user", "active", "en")
	workspace := f.workspace(owner)
	icon := "link"
	book, err := f.service.CreateBookWithDetails(f.ctx, owner, workspace.ID, BookInput{
		Name: "Notes", Icon: &icon, Description: "  project\r\nnotes  ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if book.Description != "project\r\nnotes" || book.Icon == nil || *book.Icon != "link" {
		t.Fatalf("created book = %+v", book)
	}
	empty := f.book(owner, workspace.ID)
	if empty.Description != "" {
		t.Fatalf("omitted description = %q", empty.Description)
	}

	reader := f.user("user", "active", "en")
	writer := f.user("user", "active", "en")
	pathOnly := f.user("user", "active", "en")
	stranger := f.user("user", "active", "en")
	f.grant(acl.Workspace, workspace.ID, "user", reader.ID, acl.Read)
	f.grant(acl.Workspace, workspace.ID, "user", writer.ID, acl.Write)
	table := f.table(owner, book.ID)
	f.grant(acl.Table, table.Table.ID, "user", pathOnly.ID, acl.Read)

	assertDescription := func(actor Actor, want string, path bool) {
		t.Helper()
		nav, err := f.service.Nav(f.ctx, actor)
		if err != nil {
			t.Fatal(err)
		}
		for _, space := range nav.Workspaces {
			for _, node := range space.Books {
				if node.ID == book.ID {
					if node.Description != want || node.PathOnly != path {
						t.Fatalf("book navigation = %+v, want description %q pathOnly %v", node, want, path)
					}
					return
				}
			}
		}
		t.Fatalf("book %s missing from navigation: %+v", book.ID, nav)
	}
	assertDescription(owner, book.Description, false)
	assertDescription(reader, book.Description, false)
	assertDescription(pathOnly, "", true)

	next := "  new notes 😀  "
	updated, err := f.service.UpdateBook(f.ctx, owner, book.ID, BookPatch{Description: &next})
	if err != nil || updated.Description != "new notes 😀" || updated.Icon == nil ||
		*updated.Icon != "link" || updated.FallbackIcon != book.FallbackIcon {
		t.Fatalf("description update = %+v, %v", updated, err)
	}
	assertDescription(reader, updated.Description, false)
	assertDescription(pathOnly, "", true)

	_, err = f.service.UpdateBook(f.ctx, writer, book.ID, BookPatch{Description: ptr("denied")})
	expectError(t, err, 403, "ERR_PERMISSION_DENIED")
	_, err = f.service.UpdateBook(f.ctx, stranger, book.ID, BookPatch{Description: ptr("hidden")})
	expectError(t, err, 404, "ERR_NOT_FOUND")
	_, err = f.service.UpdateBook(f.ctx, owner, book.ID, BookPatch{Description: ptr("\tinvalid")})
	expectError(t, err, 400, "ERR_VALIDATION")

	cleared, err := f.service.UpdateBook(f.ctx, owner, book.ID, BookPatch{Description: ptr("")})
	if err != nil || cleared.Description != "" || cleared.FallbackIcon != book.FallbackIcon {
		t.Fatalf("description clear = %+v, %v", cleared, err)
	}
	assertDescription(reader, "", false)
}
