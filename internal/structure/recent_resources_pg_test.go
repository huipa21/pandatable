package structure

import (
	"testing"
	"time"

	"pandatable/internal/acl"
)

func TestRecentResourcesUpsertIsolationAndNavigation(t *testing.T) {
	f := newFixture(t)
	owner, reader, other := f.user("user", "active", "en"), f.user("user", "active", "en"), f.user("user", "active", "en")
	workspace := f.workspace(owner)
	book := f.book(owner, workspace.ID)
	table := f.table(owner, book.ID)
	f.grant(acl.Table, table.Table.ID, "user", reader.ID, acl.Read)

	if err := f.service.RecordRecent(f.ctx, reader, "workspace", workspace.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.service.RecordRecent(f.ctx, reader, "book", book.ID); err != nil {
		t.Fatal(err)
	}
	f.exec("UPDATE meta.recent_resources SET opened_at=now()-interval '1 hour' WHERE user_id=$1 AND workspace_id=$2",
		reader.ID, workspace.ID)
	if err := f.service.RecordRecent(f.ctx, reader, "workspace", workspace.ID); err != nil {
		t.Fatal(err)
	}

	readerNav, err := f.service.Nav(f.ctx, reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(readerNav.Workspaces) != 1 {
		t.Fatalf("reader nav = %+v", readerNav)
	}
	gotWorkspace := readerNav.Workspaces[0]
	if !gotWorkspace.PathOnly || gotWorkspace.LastOpenedAt == nil ||
		gotWorkspace.LastOpenedAt.Before(time.Now().Add(-time.Minute)) {
		t.Fatalf("workspace recent state = %+v", gotWorkspace)
	}
	if len(gotWorkspace.Books) != 1 || !gotWorkspace.Books[0].PathOnly ||
		gotWorkspace.Books[0].LastOpenedAt == nil {
		t.Fatalf("book recent state = %+v", gotWorkspace.Books)
	}

	ownerNav, err := f.service.Nav(f.ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(ownerNav.Workspaces) != 1 || ownerNav.Workspaces[0].LastOpenedAt != nil ||
		ownerNav.Workspaces[0].Books[0].LastOpenedAt != nil {
		t.Fatalf("another user's recents leaked into owner nav: %+v", ownerNav)
	}
	otherNav, err := f.service.Nav(f.ctx, other)
	if err != nil {
		t.Fatal(err)
	}
	if len(otherNav.Workspaces) != 0 {
		t.Fatalf("unrelated user nav = %+v", otherNav)
	}
}

func TestRecentResourcesHideMissingTrashedAndUnreachableTargets(t *testing.T) {
	f := newFixture(t)
	owner, stranger := f.user("user", "active", "en"), f.user("user", "active", "en")
	workspace := f.workspace(owner)
	book := f.book(owner, workspace.ID)
	trashedWorkspace := f.workspace(owner)
	trashedBook := f.book(owner, workspace.ID)
	f.exec("UPDATE meta.workspaces SET deleted_at=now() WHERE id=$1", trashedWorkspace.ID)
	f.exec("UPDATE meta.books SET deleted_at=now() WHERE id=$1", trashedBook.ID)

	for _, target := range []struct {
		actor    Actor
		kind, id string
	}{
		{stranger, "workspace", workspace.ID},
		{stranger, "book", book.ID},
		{owner, "workspace", "00000000-0000-4000-8000-000000000000"},
		{owner, "workspace", trashedWorkspace.ID},
		{owner, "book", trashedBook.ID},
	} {
		err := f.service.RecordRecent(f.ctx, target.actor, target.kind, target.id)
		expectError(t, err, 404, "ERR_NOT_FOUND")
	}
	if count := scanOne[int](f, "SELECT count(*) FROM meta.recent_resources WHERE user_id=$1", stranger.ID); count != 0 {
		t.Fatalf("failed recent writes left %d rows", count)
	}
	if count := scanOne[int](f, "SELECT count(*) FROM meta.recent_resources WHERE user_id=$1", owner.ID); count != 0 {
		t.Fatalf("trashed/missing targets left %d recent rows", count)
	}
	if err := f.service.RecordRecent(f.ctx, owner, "resource", workspace.ID); err == nil {
		t.Fatal("unknown resource kind was accepted")
	} else {
		expectError(t, err, 400, "ERR_VALIDATION")
	}
}

func TestRecentResourcesPruneToTwoHundredInTransaction(t *testing.T) {
	f := newFixture(t)
	owner := f.user("user", "active", "en")
	rows, err := f.pool.Query(f.ctx, `INSERT INTO meta.workspaces (name, created_by)
		SELECT 'Recent ' || n, $1 FROM generate_series(1, 201) n RETURNING id::text`, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		t.Fatal(err)
	}
	rows.Close()
	if len(ids) != 201 {
		t.Fatalf("created %d workspaces, want 201", len(ids))
	}
	f.spaces = append(f.spaces, ids...)
	f.exec(`INSERT INTO meta.grants (resource_type, resource_id, principal_type, principal_id, level, granted_by)
		SELECT 'workspace', id, 'user', $1, 'owner', $1 FROM unnest($2::uuid[]) AS id`, owner.ID, ids)
	f.exec(`INSERT INTO meta.recent_resources (user_id, workspace_id, opened_at)
		SELECT $1, target_id, now() - (position::text || ' seconds')::interval
		FROM unnest($2::uuid[]) WITH ORDINALITY AS targets(target_id, position)`, owner.ID, ids)

	reopened, pruned := ids[len(ids)-1], ids[len(ids)-2]
	if err := f.service.RecordRecent(f.ctx, owner, "workspace", reopened); err != nil {
		t.Fatal(err)
	}
	if count := scanOne[int](f, "SELECT count(*) FROM meta.recent_resources WHERE user_id=$1", owner.ID); count != 200 {
		t.Fatalf("recent rows = %d, want 200", count)
	}
	if exists := scanOne[bool](f, "SELECT EXISTS(SELECT 1 FROM meta.recent_resources WHERE user_id=$1 AND workspace_id=$2)",
		owner.ID, reopened); !exists {
		t.Fatal("the opened target was pruned")
	}
	if exists := scanOne[bool](f, "SELECT EXISTS(SELECT 1 FROM meta.recent_resources WHERE user_id=$1 AND workspace_id=$2)",
		owner.ID, pruned); exists {
		t.Fatal("the oldest target was retained instead of pruning")
	}
}

func TestRecentResourcesCascadeOnUserAndWorkspaceDelete(t *testing.T) {
	f := newFixture(t)
	owner, reader := f.user("user", "active", "en"), f.user("user", "active", "en")
	workspace := f.workspace(owner)
	book := f.book(owner, workspace.ID)
	if err := f.service.RecordRecent(f.ctx, owner, "workspace", workspace.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.service.RecordRecent(f.ctx, owner, "book", book.ID); err != nil {
		t.Fatal(err)
	}
	f.grant(acl.Workspace, workspace.ID, "user", reader.ID, acl.Read)
	if err := f.service.RecordRecent(f.ctx, reader, "workspace", workspace.ID); err != nil {
		t.Fatal(err)
	}
	f.exec("DELETE FROM meta.users WHERE id=$1", reader.ID)
	if count := scanOne[int](f, "SELECT count(*) FROM meta.recent_resources WHERE user_id=$1", reader.ID); count != 0 {
		t.Fatalf("user deletion left %d recent rows", count)
	}
	f.exec("DELETE FROM meta.books WHERE id=$1", book.ID)
	if count := scanOne[int](f, "SELECT count(*) FROM meta.recent_resources WHERE book_id=$1", book.ID); count != 0 {
		t.Fatalf("book deletion left %d recent rows", count)
	}
	f.exec("DELETE FROM meta.workspaces WHERE id=$1", workspace.ID)
	if count := scanOne[int](f, "SELECT count(*) FROM meta.recent_resources WHERE workspace_id=$1", workspace.ID); count != 0 {
		t.Fatalf("workspace deletion left %d recent rows", count)
	}
}
