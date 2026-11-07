package structure

import (
	"testing"

	"pandatable/internal/acl"
)

var fallbackBookIcons = [...]string{"book", "table", "view", "fields", "calendar", "star", "group", "link"}

func TestBookIconValidationAndFallbackPalette(t *testing.T) {
	for icon := range bookIconNames {
		if got, err := cleanBookIcon(icon); err != nil || got != icon {
			t.Errorf("cleanBookIcon(%q) = %q, %v", icon, got, err)
		}
	}
	for _, invalid := range []string{"", " ", "not-an-icon", "book\n", "🐼"} {
		if _, err := cleanBookIcon(invalid); err == nil {
			t.Errorf("cleanBookIcon(%q) accepted an unknown icon", invalid)
		}
	}
	for _, icon := range fallbackBookIcons {
		if _, ok := bookIconNames[icon]; !ok {
			t.Errorf("fallback icon %q is outside the local icon vocabulary", icon)
		}
	}
}

func TestBookIconLifecycleAndSharedNavigation(t *testing.T) {
	f := newFixture(t)
	owner := f.user("user", "active", "en")
	workspace := f.workspace(owner)
	book, err := f.service.CreateBook(f.ctx, owner, workspace.ID, "Initial")
	if err != nil {
		t.Fatal(err)
	}
	if book.Icon != nil {
		t.Fatalf("name-only book icon = %v, want null", book.Icon)
	}
	fallback := book.FallbackIcon
	if _, ok := bookIconNames[fallback]; !ok {
		t.Fatalf("fallback icon %q is not in the local icon vocabulary", fallback)
	}

	reader := f.user("user", "active", "en")
	f.grant(acl.Workspace, workspace.ID, "user", reader.ID, acl.Read)
	assertVisible := func(user Actor, wantIcon *string) {
		t.Helper()
		nav, err := f.service.Nav(f.ctx, user)
		if err != nil {
			t.Fatal(err)
		}
		if len(nav.Workspaces) != 1 || len(nav.Workspaces[0].Books) != 1 {
			t.Fatalf("navigation = %+v", nav)
		}
		got := nav.Workspaces[0].Books[0]
		if got.ID != book.ID || got.FallbackIcon != fallback || (got.Icon == nil) != (wantIcon == nil) ||
			got.Icon != nil && *got.Icon != *wantIcon {
			t.Fatalf("book navigation icon = %+v; want icon %v fallback %q", got, wantIcon, fallback)
		}
	}
	assertVisible(owner, nil)
	assertVisible(reader, nil)

	custom := "calendar"
	book, err = f.service.UpdateBook(f.ctx, owner, book.ID, BookPatch{
		Icon: OptionalText{Set: true, Value: &custom},
	})
	if err != nil {
		t.Fatal(err)
	}
	if book.Icon == nil || *book.Icon != custom || book.FallbackIcon != fallback {
		t.Fatalf("custom icon update = %+v", book)
	}
	assertVisible(owner, &custom)
	assertVisible(reader, &custom)

	name, index := "Renamed", 0
	book, err = f.service.UpdateBook(f.ctx, owner, book.ID, BookPatch{Name: &name, Index: &index})
	if err != nil {
		t.Fatal(err)
	}
	if book.Name != name || book.Icon == nil || *book.Icon != custom || book.FallbackIcon != fallback {
		t.Fatalf("rename/reorder changed icon metadata: %+v", book)
	}

	book, err = f.service.UpdateBook(f.ctx, owner, book.ID, BookPatch{Icon: OptionalText{Set: true}})
	if err != nil {
		t.Fatal(err)
	}
	if book.Icon != nil || book.FallbackIcon != fallback {
		t.Fatalf("clearing custom icon changed the fallback: %+v", book)
	}
	assertVisible(reader, nil)

	if _, err := f.service.UpdateBook(f.ctx, reader, book.ID, BookPatch{
		Icon: OptionalText{Set: true, Value: &custom},
	}); err == nil {
		t.Fatal("reader changed book icon")
	}
	if _, err := f.service.UpdateBook(f.ctx, owner, book.ID, BookPatch{
		Icon: OptionalText{Set: true, Value: ptr("not-an-icon")},
	}); err == nil {
		t.Fatal("unknown custom icon was accepted")
	}
}

func TestWorkspaceIconAndDescriptionLifecycle(t *testing.T) {
	f := newFixture(t)
	owner := f.user("user", "active", "en")
	description := "  Shared\nworkspace notes  "
	workspace, err := f.service.CreateWorkspace(f.ctx, owner, WorkspaceInput{
		Name: "Workspace details", Description: description,
	})
	if err != nil {
		t.Fatal(err)
	}
	f.spaces = append(f.spaces, workspace.ID)
	if workspace.Description != "Shared\nworkspace notes" || workspace.Icon != nil {
		t.Fatalf("workspace details = %+v", workspace)
	}
	fallback := workspace.FallbackIcon
	if _, ok := bookIconNames[fallback]; !ok {
		t.Fatalf("fallback icon %q is outside the book vocabulary", fallback)
	}

	reader := f.user("user", "active", "en")
	f.grant(acl.Workspace, workspace.ID, "user", reader.ID, acl.Read)
	assertVisible := func(user Actor, wantIcon *string, wantDescription string) {
		t.Helper()
		nav, err := f.service.Nav(f.ctx, user)
		if err != nil {
			t.Fatal(err)
		}
		if len(nav.Workspaces) != 1 {
			t.Fatalf("navigation = %+v", nav)
		}
		got := nav.Workspaces[0]
		if got.FallbackIcon != fallback || got.Description != wantDescription ||
			(got.Icon == nil) != (wantIcon == nil) || got.Icon != nil && *got.Icon != *wantIcon {
			t.Fatalf("workspace navigation = %+v; want icon %v, fallback %q, description %q",
				got, wantIcon, fallback, wantDescription)
		}
	}
	assertVisible(owner, nil, workspace.Description)
	assertVisible(reader, nil, workspace.Description)

	custom := "calendar"
	updated, err := f.service.UpdateWorkspace(f.ctx, owner, workspace.ID, WorkspacePatch{
		Icon: OptionalText{Set: true, Value: &custom},
	})
	if err != nil || updated.Icon == nil || *updated.Icon != custom || updated.FallbackIcon != fallback {
		t.Fatalf("custom icon update = %+v, %v", updated, err)
	}
	assertVisible(reader, &custom, workspace.Description)

	updated, err = f.service.UpdateWorkspace(f.ctx, owner, workspace.ID, WorkspacePatch{
		Icon: OptionalText{Set: true}, Description: ptr(""),
	})
	if err != nil || updated.Icon != nil || updated.FallbackIcon != fallback || updated.Description != "" {
		t.Fatalf("clearing icon and description = %+v, %v", updated, err)
	}
	assertVisible(reader, nil, "")

	_, err = f.service.UpdateWorkspace(f.ctx, reader, workspace.ID, WorkspacePatch{
		Description: ptr("not allowed"),
	})
	expectError(t, err, 403, "ERR_PERMISSION_DENIED")
	_, err = f.service.UpdateWorkspace(f.ctx, owner, workspace.ID, WorkspacePatch{
		Icon: OptionalText{Set: true, Value: ptr("unknown")},
	})
	expectError(t, err, 400, "ERR_VALIDATION")
}

func TestCreateBookWithCustomIcon(t *testing.T) {
	f := newFixture(t)
	owner := f.user("user", "active", "en")
	workspace := f.workspace(owner)
	icon := "book"
	book, err := f.service.CreateBookWithIcon(f.ctx, owner, workspace.ID, "Custom", &icon)
	if err != nil {
		t.Fatal(err)
	}
	if book.Icon == nil || *book.Icon != icon {
		t.Fatalf("custom icon creation = %+v", book)
	}
	if _, ok := bookIconNames[book.FallbackIcon]; !ok {
		t.Fatalf("fallback icon %q is unknown", book.FallbackIcon)
	}
	if _, err := f.service.CreateBookWithIcon(f.ctx, owner, workspace.ID, "Invalid", ptr("not-an-icon")); err == nil {
		t.Fatal("unknown custom icon was accepted")
	}
}

func TestBookIconConcurrentUpdatesAndServiceRestart(t *testing.T) {
	f := newFixture(t)
	owner := f.user("user", "active", "en")
	workspace := f.workspace(owner)
	book := f.book(owner, workspace.ID)
	fallback := book.FallbackIcon

	type result struct {
		book Book
		err  error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	name, icon := "Concurrent rename", "star"
	go func() {
		<-start
		updated, err := f.service.UpdateBook(f.ctx, owner, book.ID, BookPatch{Name: &name})
		results <- result{updated, err}
	}()
	go func() {
		<-start
		updated, err := f.service.UpdateBook(f.ctx, owner, book.ID, BookPatch{
			Icon: OptionalText{Set: true, Value: &icon},
		})
		results <- result{updated, err}
	}()
	close(start)
	for range 2 {
		got := <-results
		if got.err != nil || got.book.FallbackIcon != fallback {
			t.Fatalf("concurrent update changed fallback: %+v, %v", got.book, got.err)
		}
	}

	restarted, err := New(f.pool, f.resolver)
	if err != nil {
		t.Fatal(err)
	}
	nav, err := restarted.Nav(f.ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(nav.Workspaces) != 1 || len(nav.Workspaces[0].Books) != 1 {
		t.Fatalf("navigation after service recreation = %+v", nav)
	}
	got := nav.Workspaces[0].Books[0]
	if got.ID != book.ID || got.Name != name || got.Icon == nil || *got.Icon != icon || got.FallbackIcon != fallback {
		t.Fatalf("concurrent updates were not persisted: %+v", got)
	}
}

func TestBookIconNavigationHidesUnauthorizedSiblings(t *testing.T) {
	f := newFixture(t)
	owner := f.user("user", "active", "en")
	workspace := f.workspace(owner)
	visible := f.book(owner, workspace.ID)
	icon := "star"
	hidden, err := f.service.CreateBookWithIcon(f.ctx, owner, workspace.ID, "Hidden", &icon)
	if err != nil {
		t.Fatal(err)
	}
	table := f.table(owner, visible.ID)
	reader := f.user("user", "active", "en")
	f.grant(acl.Table, table.Table.ID, "user", reader.ID, acl.Read)

	nav, err := f.service.Nav(f.ctx, reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(nav.Workspaces) != 1 || !nav.Workspaces[0].PathOnly || len(nav.Workspaces[0].Books) != 1 {
		t.Fatalf("table-only navigation leaked a sibling: %+v", nav)
	}
	got := nav.Workspaces[0].Books[0]
	if got.ID != visible.ID || !got.PathOnly || got.Icon != nil || got.FallbackIcon != visible.FallbackIcon {
		t.Fatalf("table-only book icon metadata = %+v", got)
	}
	_, err = f.service.UpdateBook(f.ctx, reader, visible.ID, BookPatch{Icon: OptionalText{Set: true, Value: &icon}})
	expectError(t, err, 403, "ERR_PERMISSION_DENIED")
	_, err = f.service.UpdateBook(f.ctx, reader, hidden.ID, BookPatch{Icon: OptionalText{Set: true, Value: &icon}})
	expectError(t, err, 404, "ERR_NOT_FOUND")

	stranger := f.user("user", "active", "en")
	nav, err = f.service.Nav(f.ctx, stranger)
	if err != nil || len(nav.Workspaces) != 0 {
		t.Fatalf("stranger navigation leaked icon metadata: %+v, %v", nav, err)
	}
}

func TestBookIconVocabularyMatchesDatabaseConstraints(t *testing.T) {
	f := newFixture(t)
	owner := f.user("user", "active", "en")
	workspace := f.workspace(owner)
	book := f.book(owner, workspace.ID)
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(f.ctx) }()
	for icon := range bookIconNames {
		if _, err := tx.Exec(f.ctx, `UPDATE meta.books SET icon=$1 WHERE id=$2`, icon, book.ID); err != nil {
			t.Fatalf("database rejected custom icon %q: %v", icon, err)
		}
	}
	for _, icon := range fallbackBookIcons {
		if _, err := tx.Exec(f.ctx, `UPDATE meta.books SET fallback_icon=$1 WHERE id=$2`, icon, book.ID); err != nil {
			t.Fatalf("database rejected fallback icon %q: %v", icon, err)
		}
	}
	var assigned string
	if err := tx.QueryRow(f.ctx, `INSERT INTO meta.books (workspace_id,name,pg_schema,position)
		VALUES ($1,'Direct insert','b_ux03_default',1) RETURNING fallback_icon`, workspace.ID).Scan(&assigned); err != nil {
		t.Fatal(err)
	}
	for _, icon := range fallbackBookIcons {
		if assigned == icon {
			return
		}
	}
	t.Fatalf("database default assigned unknown fallback icon %q", assigned)
}
