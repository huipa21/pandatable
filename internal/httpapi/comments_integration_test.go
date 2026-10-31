package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket/wsjson"

	"pandatable/internal/acl"
	"pandatable/internal/events"
	"pandatable/internal/structure"
)

func commentBody(body string) string {
	value, _ := json.Marshal(map[string]string{"body": body})
	return string(value)
}

func TestCommentPermissionsMentionsAndCounts(t *testing.T) {
	f := newACLFixture(t)
	owner, author, bob, outsider, disabled := f.user("user", false), f.user("user", false),
		f.user("user", false), f.user("admin", false), f.user("user", false)
	workspace := f.newWorkspace(owner, "Comments")
	book := f.newBook(owner, workspace.ID, "Book")
	table := f.newTable(owner, book.ID, "Rows")
	f.grantOK(owner, acl.Table, table.Table.ID, "user", author.id, acl.Read)
	group := f.scan("INSERT INTO meta.groups(name) VALUES ($1) RETURNING id::text", "Mention group")
	f.groups = append(f.groups, group)
	f.exec("INSERT INTO meta.group_members(group_id,user_id) VALUES ($1,$2)", group, bob.id)
	grant := f.grantOK(owner, acl.Book, book.ID, "group", group, acl.Read)
	f.grantOK(owner, acl.Table, table.Table.ID, "user", disabled.id, acl.Read)
	f.exec("UPDATE meta.users SET status='disabled' WHERE id=$1", disabled.id)
	row := decode[rowsResponse](f, f.api(owner, "POST", "/tables/"+table.Table.ID+"/rows",
		`{"rows":[{"fields":{}}]}`), 201).Rows[0]
	path := "/tables/" + table.Table.ID + "/rows/" + row.ID + "/comments"
	directory := "/tables/" + table.Table.ID + "/directory/mentions?q="

	listing := decode[acl.DirectoryListing](f, f.api(author, "GET", directory+url.QueryEscape(bob.login), ""), 200)
	if len(listing.Items) != 1 || listing.Items[0].ID != bob.id {
		t.Fatalf("group-inherited recipient missing from mention picker: %+v", listing)
	}
	for _, excluded := range []*aclUser{outsider, disabled} {
		listing = decode[acl.DirectoryListing](f, f.api(author, "GET", directory+url.QueryEscape(excluded.login), ""), 200)
		if len(listing.Items) != 0 {
			t.Fatalf("ineligible recipient exposed: %+v", listing)
		}
	}
	f.expectStatus(f.api(outsider, "GET", directory+"abc", ""), 404, "ERR_NOT_FOUND")
	f.expectStatus(f.api(author, "GET", directory+"a", ""), 400, "ERR_VALIDATION")
	f.expectStatus(f.api(author, "GET", directory+"abc&q=def", ""), 400, "ERR_VALIDATION")

	body := fmt.Sprintf("@%s please check @%s @%s @%s", bob.login, outsider.login, disabled.login, author.login)
	created := decode[struct {
		Comment structure.Comment `json:"comment"`
		EventID string            `json:"eventId"`
	}](f, f.api(author, "POST", path, commentBody(body)), 201)
	if created.EventID == "" || created.Comment.Body != body || len(created.Comment.Mentions) != 2 {
		t.Fatalf("unexpected comment: %+v", created)
	}
	mentioned := map[string]bool{}
	for _, user := range created.Comment.Mentions {
		mentioned[user.ID] = true
	}
	if !mentioned[bob.id] || !mentioned[author.id] || mentioned[outsider.id] || mentioned[disabled.id] {
		t.Fatalf("incorrect mention eligibility: %+v", created.Comment.Mentions)
	}
	comments := decode[structure.CommentListing](f, f.api(bob, "GET", path+"?focus="+created.Comment.ID, ""), 200)
	if len(comments.Items) != 1 || comments.Items[0].ID != created.Comment.ID {
		t.Fatalf("focused comment missing: %+v", comments)
	}
	f.expectStatus(f.api(outsider, "GET", path, ""), 404, "ERR_NOT_FOUND")
	f.expectStatus(f.api(author, "POST", path, commentBody(strings.Repeat("x", 4097))), 400, "ERR_VALIDATION")
	f.expectStatus(f.api(bob, "PATCH", "/comments/"+created.Comment.ID, commentBody("not yours")), 403, "ERR_PERMISSION_DENIED")
	f.expectStatus(f.api(owner, "PATCH", "/comments/"+created.Comment.ID, commentBody("not yours")), 403, "ERR_PERMISSION_DENIED")
	f.expectStatus(f.api(outsider, "DELETE", "/comments/"+created.Comment.ID, ""), 404, "ERR_NOT_FOUND")

	one := decode[structure.RowRecord](f, f.api(bob, "GET", "/tables/"+table.Table.ID+"/rows/"+row.ID, ""), 200)
	if one.CommentCount != 1 {
		t.Fatalf("row comment count = %d", one.CommentCount)
	}
	page := decode[structure.RowQueryResult](f, f.api(bob, "POST", "/views/"+table.Views[0].ID+"/query", `{}`), 200)
	if len(page.Rows) != 1 || page.Rows[0].CommentCount != 1 {
		t.Fatalf("view comment counts = %+v", page.Rows)
	}

	notifications := decode[structure.NotificationListing](f, f.api(bob, "GET", "/notifications", ""), 200)
	if notifications.UnreadCount != 1 || len(notifications.Items) != 1 ||
		notifications.Items[0].CommentID != created.Comment.ID || notifications.Items[0].RowID != row.ID {
		t.Fatalf("mention notification missing: %+v", notifications)
	}
	empty := decode[structure.NotificationListing](f, f.api(author, "GET", "/notifications", ""), 200)
	if empty.UnreadCount != 0 {
		t.Fatalf("self-mention notified its author: %+v", empty)
	}
	notificationID := notifications.Items[0].ID
	f.expectStatus(f.api(author, "POST", "/notifications/read", `{"id":"`+notificationID+`"}`), 200, "")
	notifications = decode[structure.NotificationListing](f, f.api(bob, "GET", "/notifications", ""), 200)
	if notifications.UnreadCount != 1 {
		t.Fatalf("another user marked notification read: %+v", notifications)
	}
	f.expectStatus(f.api(bob, "POST", "/notifications/read", `{"id":"`+notificationID+`"}`), 200, "")
	notifications = decode[structure.NotificationListing](f, f.api(bob, "GET", "/notifications", ""), 200)
	if notifications.UnreadCount != 0 || notifications.Items[0].ReadAt == nil {
		t.Fatalf("read state not persisted: %+v", notifications)
	}

	updated := decode[struct {
		Comment structure.Comment `json:"comment"`
	}](f, f.api(author, "PATCH", "/comments/"+created.Comment.ID,
		commentBody("@"+bob.login+" still mentioned")), 200)
	if updated.Comment.EditedAt == nil {
		t.Fatal("comment edit timestamp missing")
	}
	notifications = decode[structure.NotificationListing](f, f.api(bob, "GET", "/notifications", ""), 200)
	if len(notifications.Items) != 1 || notifications.UnreadCount != 0 {
		t.Fatalf("unchanged mention was notified twice: %+v", notifications)
	}
	f.expectStatus(f.api(author, "PATCH", "/comments/"+created.Comment.ID,
		commentBody("mention removed")), 200, "")
	notifications = decode[structure.NotificationListing](f, f.api(bob, "GET", "/notifications", ""), 200)
	if len(notifications.Items) != 0 || notifications.UnreadCount != 0 {
		t.Fatalf("removed mention retained a notification: %+v", notifications)
	}
	f.expectStatus(f.api(author, "PATCH", "/comments/"+created.Comment.ID,
		commentBody("@"+bob.login+" mentioned again")), 200, "")
	notifications = decode[structure.NotificationListing](f, f.api(bob, "GET", "/notifications", ""), 200)
	if len(notifications.Items) != 1 || notifications.UnreadCount != 1 ||
		notifications.Items[0].ID == notificationID {
		t.Fatalf("re-added mention did not create a new unread notification: %+v", notifications)
	}
	f.expectStatus(f.api(owner, "DELETE", "/grants/"+grant.Grant.ID, ""), 200, "")
	notifications = decode[structure.NotificationListing](f, f.api(bob, "GET", "/notifications", ""), 200)
	if len(notifications.Items) != 0 || notifications.UnreadCount != 0 {
		t.Fatalf("revoked recipient still sees notifications: %+v", notifications)
	}
	f.expectStatus(f.api(owner, "DELETE", "/comments/"+created.Comment.ID, ""), 200, "")
	one = decode[structure.RowRecord](f, f.api(author, "GET", "/tables/"+table.Table.ID+"/rows/"+row.ID, ""), 200)
	if one.CommentCount != 0 {
		t.Fatalf("deleted comment still counted: %+v", one)
	}

	own := decode[struct {
		Comment structure.Comment `json:"comment"`
	}](f, f.api(author, "POST", path, commentBody("Author can delete")), 201)
	f.expectStatus(f.api(author, "DELETE", "/comments/"+own.Comment.ID, ""), 200, "")
	one = decode[structure.RowRecord](f, f.api(author, "GET", "/tables/"+table.Table.ID+"/rows/"+row.ID, ""), 200)
	if one.CommentCount != 0 {
		t.Fatalf("author-deleted comment still counted: %+v", one)
	}

	retained := decode[struct {
		Comment structure.Comment `json:"comment"`
	}](f, f.api(author, "POST", path, commentBody("Retained on row deletion")), 201)
	f.expectStatus(f.api(owner, "DELETE", "/tables/"+table.Table.ID+"/rows", `{"ids":["`+row.ID+`"]}`), 200, "")
	if count := f.scan("SELECT count(*)::text FROM meta.comments WHERE id=$1 AND deleted_at IS NULL", retained.Comment.ID); count != "1" {
		t.Fatalf("row deletion removed its comments: %s", count)
	}
	f.expectStatus(f.api(author, "GET", path, ""), 404, "ERR_NOT_FOUND")
}

func TestCommentPaginationKeepsOldFocusedComment(t *testing.T) {
	f := newACLFixture(t)
	owner := f.user("user", false)
	workspace := f.newWorkspace(owner, "Comment pagination")
	book := f.newBook(owner, workspace.ID, "Book")
	table := f.newTable(owner, book.ID, "Rows")
	row := decode[rowsResponse](f, f.api(owner, "POST", "/tables/"+table.Table.ID+"/rows",
		`{"rows":[{"fields":{}}]}`), 201).Rows[0]
	f.exec(`INSERT INTO meta.comments (table_id, row_id, user_id, body, created_at)
		SELECT $1::uuid, $2::bigint, $3::uuid, 'Comment ' || n,
			now() - n * interval '1 minute'
		FROM generate_series(1, 52) AS n`, table.Table.ID, row.ID, owner.id)
	oldest := f.scan(`SELECT id::text FROM meta.comments WHERE table_id=$1
		ORDER BY created_at ASC LIMIT 1`, table.Table.ID)
	path := "/tables/" + table.Table.ID + "/rows/" + row.ID + "/comments"
	first := decode[structure.CommentListing](f, f.api(owner, "GET", path+"?focus="+oldest, ""), 200)
	if len(first.Items) != 50 || !first.HasMore || first.Items[0].ID != oldest {
		t.Fatalf("old focused comment was not pinned on the first page: %+v", first)
	}
	next := decode[structure.CommentListing](f, f.api(owner, "GET", path+"?focus="+oldest+"&offset=50", ""), 200)
	if len(next.Items) != 2 || next.HasMore {
		t.Fatalf("unexpected second comment page: %+v", next)
	}
	seen := map[string]bool{}
	for _, comment := range append(first.Items, next.Items...) {
		if seen[comment.ID] {
			t.Fatalf("comment repeated across focused pages: %s", comment.ID)
		}
		seen[comment.ID] = true
	}
	if len(seen) != 52 {
		t.Fatalf("focused pagination omitted comments: got %d", len(seen))
	}
}

func TestMentionDirectoryFiltersBeforeLimit(t *testing.T) {
	f := newACLFixture(t)
	owner, eligible := f.user("user", false), f.user("user", false)
	workspace := f.newWorkspace(owner, "Mention directory")
	book := f.newBook(owner, workspace.ID, "Book")
	table := f.newTable(owner, book.ID, "Rows")
	f.grantOK(owner, acl.Table, table.Table.ID, "user", eligible.id, acl.Read)
	prefix := "pick" + owner.login
	f.exec("UPDATE meta.users SET username=$1 WHERE id=$2", prefix+"zz", eligible.id)
	f.exec(`INSERT INTO meta.users (username, email, display_name, password_hash, role, must_change_password, avatar_color)
		SELECT $1 || lpad(n::text, 2, '0'), $1 || n::text || '@acl.test',
			$1 || n::text, $2, 'user', false, 'blue'
		FROM generate_series(0, 20) AS n`, prefix, f.phc)
	t.Cleanup(func() {
		f.exec(`DELETE FROM meta.users WHERE username IN
			(SELECT $1 || lpad(n::text, 2, '0') FROM generate_series(0, 20) AS n)`, prefix)
	})
	listing := decode[acl.DirectoryListing](f, f.api(owner, "GET",
		"/tables/"+table.Table.ID+"/directory/mentions?q="+url.QueryEscape(prefix), ""), 200)
	if len(listing.Items) != 1 || listing.Items[0].ID != eligible.id || listing.HasMore {
		t.Fatalf("inaccessible candidates consumed the mention result limit: %+v", listing)
	}
}

func TestMentionPushRechecksRevokedAccess(t *testing.T) {
	f := newACLFixture(t)
	owner, bob := f.user("user", false), f.user("user", false)
	workspace := f.newWorkspace(owner, "Revoked mention")
	book := f.newBook(owner, workspace.ID, "Book")
	table := f.newTable(owner, book.ID, "Rows")
	grant := f.grantOK(owner, acl.Table, table.Table.ID, "user", bob.id, acl.Read)
	server, _ := realtimeServer(t, f)
	socket := dialRealtime(t, f, server, bob, server.URL)
	f.expectStatus(f.api(owner, "DELETE", "/grants/"+grant.Grant.ID, ""), 200, "")

	for _, event := range []events.Event{
		{Type: "notification", TableID: table.Table.ID, Recipient: bob.id},
		{Type: "notification", TableID: table.Table.ID},
		{Type: "notification.changed", Recipient: bob.id, EventID: "revoked-marker"},
	} {
		payload, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		f.exec("SELECT pg_notify($1, $2)", events.Channel, string(payload))
	}
	ctx, cancel := context.WithTimeout(f.ctx, 5*time.Second)
	defer cancel()
	for {
		var event events.Event
		if err := wsjson.Read(ctx, socket, &event); err != nil {
			t.Fatalf("read marker after revoked notification: %v", err)
		}
		if event.Type == "notification" {
			t.Fatalf("revoked recipient received a notification: %+v", event)
		}
		if event.Type == "notification.changed" && event.EventID == "revoked-marker" {
			break
		}
	}
}

func TestMentionPushWithoutSubscriptionAndRollback(t *testing.T) {
	f := newACLFixture(t)
	owner, bob := f.user("user", false), f.user("user", false)
	workspace := f.newWorkspace(owner, "Mention push")
	book := f.newBook(owner, workspace.ID, "Book")
	table := f.newTable(owner, book.ID, "Rows")
	f.grantOK(owner, acl.Table, table.Table.ID, "user", bob.id, acl.Read)
	row := decode[rowsResponse](f, f.api(owner, "POST", "/tables/"+table.Table.ID+"/rows",
		`{"rows":[{"fields":{}}]}`), 201).Rows[0]
	server, _ := realtimeServer(t, f)
	socket := dialRealtime(t, f, server, bob, server.URL)
	path := "/tables/" + table.Table.ID + "/rows/" + row.ID + "/comments"
	created := decode[struct {
		Comment structure.Comment `json:"comment"`
		EventID string            `json:"eventId"`
	}](f, f.api(owner, "POST", path, commentBody("@"+bob.login+" please check")), 201)
	event := readRealtime(t, f, socket, "notification")
	if event.TableID != table.Table.ID || event.EventID != created.EventID || event.Recipient != "" {
		t.Fatalf("invalid recipient-only push: %+v", event)
	}

	f.exec(`CREATE FUNCTION meta.test_reject_mention() RETURNS trigger LANGUAGE plpgsql AS
		$$ BEGIN RAISE EXCEPTION 'fixture rollback'; END $$`)
	f.exec(`CREATE CONSTRAINT TRIGGER test_reject_mention AFTER INSERT ON meta.notifications
		DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION meta.test_reject_mention()`)
	t.Cleanup(func() { f.exec("DROP FUNCTION IF EXISTS meta.test_reject_mention() CASCADE") })
	f.expectStatus(f.api(owner, "POST", path, commentBody("@"+bob.login+" rolled back")), 500, "ERR_INTERNAL")
	if count := f.scan("SELECT count(*)::text FROM meta.comments WHERE table_id=$1 AND row_id=$2", table.Table.ID, row.ID); count != "1" {
		t.Fatalf("failed notification transaction left a comment: %s", count)
	}
	ctx, cancel := context.WithTimeout(f.ctx, 250*time.Millisecond)
	defer cancel()
	var leaked events.Event
	if err := wsjson.Read(ctx, socket, &leaked); err == nil {
		t.Fatalf("rolled-back transaction published an event: %+v", leaked)
	}
}
