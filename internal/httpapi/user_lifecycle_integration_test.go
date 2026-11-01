package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"pandatable/internal/acl"
	"pandatable/internal/admin"
	"pandatable/internal/attachments"
	"pandatable/internal/structure"
)

func lifecycleCount(f *aclFixture, query string, args ...any) int {
	f.t.Helper()
	var count int
	if err := f.pool.QueryRow(f.ctx, query, args...).Scan(&count); err != nil {
		f.t.Fatal(err)
	}
	return count
}

func disableLifecycleUser(f *aclFixture, actor, target *aclUser) {
	f.t.Helper()
	f.expectStatus(f.api(actor, "PATCH", "/admin/users/"+target.id, `{"status":"disabled"}`), 200, "")
}

func TestUserLifecycleRescueAndTransferAT113AT114(t *testing.T) {
	f := newACLFixture(t)
	alice, bob, manager, disabled := f.user("user", false), f.user("user", false),
		f.user("admin", false), f.user("user", false)
	first := f.newWorkspace(alice, "First ownership")
	book := f.newBook(alice, first.ID, "First book")
	table := f.newTable(alice, book.ID, "First table")
	second := f.newWorkspace(alice, "Second ownership")
	f.grantOK(alice, acl.Workspace, second.ID, "user", bob.id, acl.Read)
	group := f.scan("INSERT INTO meta.groups(name) VALUES ('Lifecycle owners') RETURNING id::text")
	f.groups = append(f.groups, group)
	f.grantOK(alice, acl.Workspace, first.ID, "group", group, acl.Owner)
	third := f.newWorkspace(bob, "Book and table ownerships")
	thirdBook := f.newBook(bob, third.ID, "Delegated book")
	thirdTable := f.newTable(bob, thirdBook.ID, "Delegated table")
	f.grantOK(bob, acl.Book, thirdBook.ID, "user", alice.id, acl.Owner)
	f.grantOK(bob, acl.Table, thirdTable.Table.ID, "user", alice.id, acl.Owner)
	trashed := f.newWorkspace(alice, "Trashed ownership")
	trashedBook := f.newBook(alice, trashed.ID, "Trashed ownership book")
	trashedTable := f.newTable(alice, trashedBook.ID, "Trashed ownership table")
	f.expectStatus(f.api(alice, "DELETE", "/workspaces/"+trashed.ID, ""), 200, "")
	disableLifecycleUser(f, manager, disabled)
	disableLifecycleUser(f, manager, alice)

	f.expectStatus(f.api(manager, "GET", "/tables/"+table.Table.ID+"/schema", ""), 404, "ERR_NOT_FOUND")
	f.expectStatus(f.api(bob, "GET", "/admin/users/"+alice.id+"/ownerships", ""), 403, "ERR_PERMISSION_DENIED")
	f.expectStatus(f.api(bob, "POST", "/admin/workspaces/"+first.ID+"/rescue",
		fmt.Sprintf(`{"userId":%q}`, bob.id)), 403, "ERR_PERMISSION_DENIED")
	listing := decode[admin.WorkspaceListing](f, f.api(manager, "GET", "/admin/workspaces?limit=50", ""), 200)
	var orphaned bool
	for _, item := range listing.Items {
		if item.ID == first.ID {
			orphaned = item.Orphaned
		}
	}
	if !orphaned {
		t.Fatal("disabled user and group owner must not prevent orphan status")
	}
	ownerships := decode[admin.OwnershipListing](f,
		f.api(manager, "GET", "/admin/users/"+alice.id+"/ownerships?limit=2", ""), 200)
	if ownerships.Total != 5 || len(ownerships.Items) != 2 {
		t.Fatalf("paginated ownerships = %+v", ownerships)
	}
	allOwnerships := decode[admin.OwnershipListing](f,
		f.api(manager, "GET", "/admin/users/"+alice.id+"/ownerships?limit=50", ""), 200)
	var foundTrash bool
	for _, item := range allOwnerships.Items {
		if item.ResourceID == trashed.ID {
			foundTrash = item.Trashed
		}
	}
	if !foundTrash {
		t.Fatal("trashed workspace ownership missing from the list")
	}
	f.expectStatus(f.api(manager, "POST", "/admin/workspaces/"+first.ID+"/rescue",
		fmt.Sprintf(`{"userId":%q}`, disabled.id)), 409, "ERR_PRINCIPAL_DISABLED")
	f.expectStatus(f.api(manager, "POST", "/admin/workspaces/"+first.ID+"/rescue",
		fmt.Sprintf(`{"userId":%q}`, bob.id)), 200, "")
	schema := decode[structure.TableSchema](f, f.api(bob, "GET", "/tables/"+table.Table.ID+"/schema", ""), 200)
	if schema.MyLevel != "owner" {
		t.Fatalf("rescued owner level = %q", schema.MyLevel)
	}
	f.expectStatus(f.api(manager, "GET", "/tables/"+table.Table.ID+"/schema", ""), 404, "ERR_NOT_FOUND")
	f.expectStatus(f.api(manager, "POST", "/admin/users/"+alice.id+"/ownerships/transfer",
		fmt.Sprintf(`{"userId":%q}`, disabled.id)), 409, "ERR_PRINCIPAL_DISABLED")
	transfer := decode[struct {
		Transferred int `json:"transferred"`
	}](f, f.api(manager, "POST", "/admin/users/"+alice.id+"/ownerships/transfer",
		fmt.Sprintf(`{"userId":%q}`, bob.id)), 200)
	if transfer.Transferred != 5 {
		t.Fatalf("transferred = %d, want all five direct grants", transfer.Transferred)
	}
	if count := lifecycleCount(f, `SELECT count(*) FROM meta.grants WHERE principal_type='user'
		AND principal_id=$1 AND level='owner'`, alice.id); count != 0 {
		t.Fatalf("old ownerships retained: %d", count)
	}
	for _, pair := range [][2]string{{"workspace", first.ID}, {"workspace", second.ID}, {"workspace", trashed.ID},
		{"book", thirdBook.ID}, {"table", thirdTable.Table.ID}} {
		if count := lifecycleCount(f, `SELECT count(*) FROM meta.grants
			WHERE resource_type=$1 AND resource_id=$2 AND principal_type='user' AND principal_id=$3 AND level='owner'`,
			pair[0], pair[1], bob.id); count != 1 {
			t.Fatalf("recipient missing direct %s ownership %s", pair[0], pair[1])
		}
	}
	f.expectStatus(f.api(bob, "POST", "/trash/workspace/"+trashed.ID+"/restore", `{}`), 200, "")
	restored := decode[structure.TableSchema](f, f.api(bob, "GET", "/tables/"+trashedTable.Table.ID+"/schema", ""), 200)
	if restored.MyLevel != "owner" {
		t.Fatalf("restored trashed ownership level = %q", restored.MyLevel)
	}
	f.expectStatus(f.api(manager, "POST", "/admin/workspaces/"+second.ID+"/rescue",
		fmt.Sprintf(`{"userId":%q}`, bob.id)), 409, "ERR_WORKSPACE_NOT_ORPHANED")
	f.expectStatus(f.api(manager, "DELETE", "/admin/users/"+bob.id,
		fmt.Sprintf(`{"username":%q}`, bob.login)), 409, "ERR_USER_NOT_DISABLED")
}

func lifecycleImage(t *testing.T, f *aclFixture, owner *aclUser, tableID string) (attachments.File, string) {
	t.Helper()
	field := f.newField(owner, tableID, `{"name":"Lifecycle files","uiType":"attachment"}`)
	var content bytes.Buffer
	if err := png.Encode(&content, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	file := decode[attachments.File](f, attachmentUpload(f, owner, tableID, field.ID, "lifecycle.png", content.Bytes()), 201)
	if file.Thumb == nil {
		t.Fatal("expected thumbnail")
	}
	stored := f.scan("SELECT storage_path FROM meta.attachments WHERE id=$1", file.ID)
	return file, filepath.Join(f.dataDir, filepath.FromSlash(stored))
}

func requireLifecycleFile(t *testing.T, path string, exists bool) {
	t.Helper()
	_, err := os.Stat(path)
	if exists && err != nil || !exists && !os.IsNotExist(err) {
		t.Fatalf("file %q exists=%t: %v", path, exists, err)
	}
}

func TestUserLifecycleHardDeleteAtomicPreservesSharedAT115(t *testing.T) {
	f := newACLFixture(t)
	alice, bob, manager := f.user("user", false), f.user("user", false), f.user("admin", false)
	sole := f.newWorkspace(alice, "Sole")
	soleBook := f.newBook(alice, sole.ID, "Sole book")
	soleTable := f.newTable(alice, soleBook.ID, "Sole table")
	soleFile, solePath := lifecycleImage(t, f, alice, soleTable.Table.ID)
	soleSchema := f.scan("SELECT pg_schema FROM meta.books WHERE id=$1", soleBook.ID)
	trashed := f.newWorkspace(alice, "Trashed sole")
	trashBook := f.newBook(alice, trashed.ID, "Trashed book")
	trashSchema := f.scan("SELECT pg_schema FROM meta.books WHERE id=$1", trashBook.ID)
	f.expectStatus(f.api(alice, "DELETE", "/workspaces/"+trashed.ID, ""), 200, "")
	shared := f.newWorkspace(alice, "Shared")
	sharedBook := f.newBook(alice, shared.ID, "Shared book")
	sharedTable := f.newTable(alice, sharedBook.ID, "Shared table")
	f.grantOK(alice, acl.Workspace, shared.ID, "user", bob.id, acl.Owner)
	sharedFile, sharedPath := lifecycleImage(t, f, alice, sharedTable.Table.ID)
	field := *sharedTable.Table.DisplayFieldID
	row := decode[rowsResponse](f, f.api(alice, "POST", "/tables/"+sharedTable.Table.ID+"/rows",
		fmt.Sprintf(`{"rows":[{"fields":{"%s":"kept"}}]}`, field)), 201).Rows[0]
	commentsPath := "/tables/" + sharedTable.Table.ID + "/rows/" + row.ID + "/comments"
	f.expectStatus(f.api(alice, "POST", commentsPath, commentBody("kept comment")), 201, "")
	personal := decode[viewResponse](f, f.api(alice, "POST", "/tables/"+sharedTable.Table.ID+"/views",
		`{"name":"Personal","scope":"personal","type":"grid"}`), 201).View
	groupShared := f.newWorkspace(alice, "Group co-owner")
	group := f.scan("INSERT INTO meta.groups(name) VALUES ('Lifecycle co-owner') RETURNING id::text")
	f.groups = append(f.groups, group)
	f.grantOK(alice, acl.Workspace, groupShared.ID, "group", group, acl.Owner)

	disableLifecycleUser(f, manager, alice)
	f.expectStatus(f.api(bob, "DELETE", "/admin/users/"+alice.id,
		fmt.Sprintf(`{"username":%q}`, alice.login)), 403, "ERR_PERMISSION_DENIED")
	f.expectStatus(f.api(manager, "DELETE", "/admin/users/"+alice.id, `{"username":"wrong"}`),
		400, "ERR_USERNAME_CONFIRMATION")
	if lifecycleCount(f, "SELECT count(*) FROM meta.users WHERE id=$1", alice.id) != 1 {
		t.Fatal("wrong confirmation deleted the user")
	}
	blocker := pgx.Identifier{trashSchema, "lifecycle_blocker"}.Sanitize()
	f.exec("CREATE TABLE " + blocker + " (id integer)")
	f.expectStatus(f.api(manager, "DELETE", "/admin/users/"+alice.id,
		fmt.Sprintf(`{"username":%q}`, alice.login)), 500, "ERR_INTERNAL")
	if lifecycleCount(f, "SELECT count(*) FROM meta.users WHERE id=$1", alice.id) != 1 ||
		lifecycleCount(f, "SELECT count(*) FROM meta.workspaces WHERE id=$1", sole.ID) != 1 ||
		lifecycleCount(f, "SELECT count(*) FROM meta.attachment_purge_queue WHERE id=$1", soleFile.ID) != 0 ||
		lifecycleCount(f, "SELECT count(*) FROM pg_namespace WHERE nspname=$1", soleSchema) != 1 {
		t.Fatal("failed DDL did not roll back the entire user deletion")
	}
	requireLifecycleFile(t, solePath, true)
	f.exec("DROP TABLE " + blocker)
	f.expectStatus(f.api(manager, "DELETE", "/admin/users/"+alice.id,
		fmt.Sprintf(`{"username":%q}`, alice.login)), 200, "")

	for _, schema := range []string{soleSchema, trashSchema} {
		if lifecycleCount(f, "SELECT count(*) FROM pg_namespace WHERE nspname=$1", schema) != 0 {
			t.Fatalf("sole-owner physical schema %s remains", schema)
		}
	}
	for _, id := range []string{sole.ID, trashed.ID} {
		if lifecycleCount(f, "SELECT count(*) FROM meta.workspaces WHERE id=$1", id) != 0 {
			t.Fatalf("sole-owner workspace %s remains", id)
		}
	}
	for _, id := range []string{shared.ID, groupShared.ID} {
		if lifecycleCount(f, "SELECT count(*) FROM meta.workspaces WHERE id=$1", id) != 1 {
			t.Fatalf("shared workspace %s was deleted", id)
		}
	}
	if lifecycleCount(f, "SELECT count(*) FROM meta.users WHERE id=$1", alice.id) != 0 ||
		lifecycleCount(f, "SELECT count(*) FROM meta.deleted_users WHERE id=$1", alice.id) != 1 ||
		lifecycleCount(f, "SELECT count(*) FROM meta.grants WHERE principal_type='user' AND principal_id=$1", alice.id) != 0 ||
		lifecycleCount(f, "SELECT count(*) FROM meta.views WHERE id=$1", personal.ID) != 0 {
		t.Fatal("user, grants, personal view or anonymous attribution is inconsistent")
	}
	requireLifecycleFile(t, solePath, false)
	requireLifecycleFile(t, solePath+".thumb.png", false)
	requireLifecycleFile(t, sharedPath, true)
	requireLifecycleFile(t, sharedPath+".thumb.png", true)
	f.expectStatus(f.api(bob, "GET", "/attachments/"+sharedFile.ID, ""), 200, "")
	surviving := decode[structure.RowRecord](f,
		f.api(bob, "GET", "/tables/"+sharedTable.Table.ID+"/rows/"+row.ID, ""), 200)
	if string(surviving.Fields[field]) != `"kept"` {
		t.Fatalf("shared row changed: %+v", surviving)
	}
	var createdBy, createdByField string
	for _, field := range sharedTable.Fields {
		if field.UIType == "created_by" {
			createdByField = field.ID
			if err := json.Unmarshal(surviving.Fields[field.ID], &createdBy); err != nil {
				t.Fatal(err)
			}
		}
	}
	if createdBy != alice.id {
		t.Fatalf("row authorship ID = %q, want deleted identity", createdBy)
	}
	labels := decode[struct {
		Items []acl.UserLabel `json:"items"`
	}](f, f.api(bob, "POST", "/tables/"+sharedTable.Table.ID+"/directory/user-labels",
		fmt.Sprintf(`{"ids":[%q]}`, alice.id)), 200)
	if len(labels.Items) != 1 || !labels.Items[0].Deleted || labels.Items[0].Name != "" {
		t.Fatalf("deleted attribution label = %+v", labels)
	}
	for _, sample := range []struct{ locale, label string }{
		{"en", "Deleted user"},
		{"zh-CN", "已删除的用户"},
	} {
		f.exec("UPDATE meta.users SET locale=$1 WHERE id=$2", sample.locale, bob.id)
		ticket := prepareExportForTest(f, bob, sharedTable.Views[0].ID, map[string]any{
			"format": "csv", "fieldIds": []string{createdByField},
		})
		exported := f.api(bob, "GET", "/exports/"+ticket, "")
		f.expectStatus(exported, 200, "")
		if !strings.Contains(exported.Body.String(), sample.label) {
			t.Fatalf("%s export lacks deleted-user label: %q", sample.locale, exported.Body.String())
		}
	}
	comments := decode[structure.CommentListing](f, f.api(bob, "GET", commentsPath, ""), 200)
	history := decode[structure.HistoryListing](f,
		f.api(bob, "GET", "/tables/"+sharedTable.Table.ID+"/rows/"+row.ID+"/history", ""), 200)
	if len(comments.Items) != 1 || comments.Items[0].AuthorID != nil || comments.Items[0].AuthorName != "" ||
		len(history.Items) == 0 || history.Items[0].AuthorID != nil || history.Items[0].AuthorName != "" {
		t.Fatalf("remaining authorship not anonymized: comments=%+v history=%+v", comments, history)
	}
	f.expectStatus(f.api(manager, "POST", "/admin/workspaces/"+groupShared.ID+"/rescue",
		fmt.Sprintf(`{"userId":%q}`, bob.id)), 200, "")
	f.expectStatus(f.api(manager, "GET", "/tables/"+sharedTable.Table.ID+"/schema", ""), 404, "ERR_NOT_FOUND")
}

func TestUserLifecycleDeletionFileFailureIsDurableAndRetryable(t *testing.T) {
	f := newACLFixture(t)
	alice, manager := f.user("user", false), f.user("admin", false)
	workspace := f.newWorkspace(alice, "Cleanup failure")
	book := f.newBook(alice, workspace.ID, "Book")
	table := f.newTable(alice, book.ID, "Table")
	file, path := lifecycleImage(t, f, alice, table.Table.ID)
	stored := f.scan("SELECT storage_path FROM meta.attachments WHERE id=$1", file.ID)
	f.exec("UPDATE meta.attachments SET storage_path='invalid' WHERE id=$1", file.ID)
	disableLifecycleUser(f, manager, alice)
	f.expectStatus(f.api(manager, "DELETE", "/admin/users/"+alice.id,
		fmt.Sprintf(`{"username":%q}`, alice.login)), 503, "ERR_USER_CLEANUP_PENDING")
	if lifecycleCount(f, "SELECT count(*) FROM meta.users WHERE id=$1", alice.id) != 0 ||
		lifecycleCount(f, "SELECT count(*) FROM meta.attachment_purge_queue WHERE id=$1 AND cleanup_user_id=$2",
			file.ID, alice.id) != 1 {
		t.Fatal("failed cleanup was not retained after the committed deletion")
	}
	requireLifecycleFile(t, path, true)
	pending := decode[struct {
		Pending int `json:"pending"`
	}](f, f.api(manager, "GET", "/admin/user-deletions/cleanup", ""), 200)
	if pending.Pending != 1 {
		t.Fatalf("pending files = %d", pending.Pending)
	}
	f.expectStatus(f.api(manager, "POST", "/admin/user-deletions/cleanup", ""), 503, "ERR_USER_CLEANUP_PENDING")
	f.exec("UPDATE meta.attachment_purge_queue SET storage_path=$2 WHERE id=$1", file.ID, stored)
	f.expectStatus(f.api(manager, "POST", "/admin/user-deletions/cleanup", ""), 200, "")
	if lifecycleCount(f, "SELECT count(*) FROM meta.attachment_purge_queue WHERE id=$1", file.ID) != 0 {
		t.Fatal("successful retry left the queue entry")
	}
	requireLifecycleFile(t, path, false)
	requireLifecycleFile(t, path+".thumb.png", false)
}

func TestUserLifecycleRescueRechecksOrphanAfterWorkspaceLock(t *testing.T) {
	f := newACLFixture(t)
	alice, bob, carol, manager := f.user("user", false), f.user("user", false), f.user("user", false), f.user("admin", false)
	workspace := f.newWorkspace(alice, "Rescue race")
	disableLifecycleUser(f, manager, alice)
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(f.ctx) }()
	if _, err := tx.Exec(f.ctx, "SELECT id FROM meta.workspaces WHERE id=$1 FOR NO KEY UPDATE", workspace.ID); err != nil {
		t.Fatal(err)
	}
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- f.api(manager, http.MethodPost, "/admin/workspaces/"+workspace.ID+"/rescue",
			fmt.Sprintf(`{"userId":%q}`, bob.id))
	}()
	select {
	case response := <-done:
		t.Fatalf("rescue did not wait for workspace lock: %s", response.Body)
	case <-time.After(300 * time.Millisecond):
	}
	if _, err := tx.Exec(f.ctx, `INSERT INTO meta.grants(resource_type,resource_id,principal_type,principal_id,level)
		VALUES ('workspace',$1,'user',$2,'owner')`, workspace.ID, carol.id); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case response := <-done:
		f.expectStatus(response, 409, "ERR_WORKSPACE_NOT_ORPHANED")
	case <-time.After(10 * time.Second):
		t.Fatal("rescue remained blocked after the workspace lock was released")
	}
	if count := lifecycleCount(f, `SELECT count(*) FROM meta.grants WHERE resource_type='workspace'
		AND resource_id=$1 AND principal_id=$2`, workspace.ID, bob.id); count != 0 {
		t.Fatal("stale orphan check granted an unnecessary owner")
	}
}
