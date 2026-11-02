package httpapi

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	"pandatable/internal/acl"
	"pandatable/internal/admin"
	"pandatable/internal/mailer"
	"pandatable/internal/structure"
)

func TestMailAuthorizationResetAndMentionOutbox(t *testing.T) {
	f := newACLFixture(t)
	owner, recipient, outsider := f.user("admin", false), f.user("user", false), f.user("user", false)
	management, err := admin.NewService(f.pool, f.authService, f.resolver, bytes.Repeat([]byte{9}, 32))
	if err != nil {
		t.Fatal(err)
	}
	structures, err := structure.New(f.pool, f.resolver)
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	mail, err := mailer.New(f.pool, f.authService, management, "https://table.example", logger)
	if err != nil {
		t.Fatal(err)
	}
	f.handler, err = New(logger, testAssets(), AuthOptions{
		Service: f.authService, ACL: f.aclService, Structure: structures, Admin: management,
		Mailer: mail, BaseURL: "https://table.example",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		f.exec(`UPDATE meta.smtp_settings SET enabled=false,host='',port=587,security='starttls',
			username='',password_cipher=NULL,from_address='' WHERE singleton`)
	})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	secret := "smtp-only-test-secret"
	username := "smtp-private-user"
	body := fmt.Sprintf(`{"enabled":true,"host":"127.0.0.1","port":%d,"security":"none",`+
		`"username":"%s","password":"%s","fromAddress":"sender@example.test"}`, port, username, secret)
	f.expectStatus(f.api(recipient, "PATCH", "/admin/smtp", body), 403, "ERR_PERMISSION_DENIED")
	updated := f.api(owner, "PATCH", "/admin/smtp", body)
	f.expectStatus(updated, 200, "")
	read := f.api(owner, "GET", "/admin/smtp", "")
	f.expectStatus(read, 200, "")
	for _, credential := range []string{username, secret} {
		if strings.Contains(updated.Body.String(), credential) || strings.Contains(read.Body.String(), credential) {
			t.Fatal("SMTP response exposed credentials")
		}
	}
	f.expectStatus(f.api(owner, "POST", "/admin/smtp/test", `{"to":"test@example.test"}`), 502, "ERR_SMTP_SEND")

	existing := f.api(nil, "POST", "/auth/reset/request", `{"email":"`+recipient.login+`@acl.test"}`)
	unknown := f.api(nil, "POST", "/auth/reset/request", `{"email":"unknown@example.test"}`)
	f.expectStatus(existing, 200, "")
	f.expectStatus(unknown, 200, "")
	if existing.Body.String() != unknown.Body.String() {
		t.Fatal("reset requests must not disclose whether the account exists")
	}
	f.expectStatus(f.api(nil, "POST", "/auth/reset/complete", `{"token":"invalid","password":"Valid-password-123"}`),
		400, "ERR_RESET_TOKEN")
	for _, user := range []*aclUser{owner, recipient, outsider} {
		f.expectStatus(f.api(user, "PATCH", "/auth/me", `{"mentionEmail":true}`), 200, "")
	}
	workspace := f.newWorkspace(owner, "Mail")
	book := f.newBook(owner, workspace.ID, "Mail")
	table := f.newTable(owner, book.ID, "Mail")
	grant := f.grantOK(owner, acl.Table, table.Table.ID, "user", recipient.id, acl.Read)
	row := decode[rowsResponse](f, f.api(owner, "POST", "/tables/"+table.Table.ID+"/rows",
		`{"rows":[{"fields":{}}]}`), 201).Rows[0]
	path := "/tables/" + table.Table.ID + "/rows/" + row.ID + "/comments"
	comment := fmt.Sprintf("@%s @%s @%s", recipient.login, owner.login, outsider.login)
	f.expectStatus(f.api(owner, "POST", path, commentBody(comment)), 201, "")
	if count := f.scan("SELECT count(*)::text FROM meta.mention_mail_jobs"); count != "1" {
		t.Fatalf("expected one eligible mail job, got %s", count)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		mail.Run(ctx)
	}()
	defer func() { cancel(); <-done }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		attempts := f.scan("SELECT attempts::text FROM meta.mention_mail_jobs LIMIT 1")
		if attempts == "1" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("failed SMTP delivery was not recorded for retry")
		}
		time.Sleep(50 * time.Millisecond)
	}
	notifications := decode[structure.NotificationListing](f, f.api(recipient, "GET", "/notifications", ""), 200)
	if notifications.UnreadCount != 1 || len(notifications.Items) != 1 {
		t.Fatal("SMTP failure must not roll back the in-app notification")
	}
	f.expectStatus(f.api(recipient, "PATCH", "/auth/me", `{"mentionEmail":false}`), 200, "")
	f.exec("UPDATE meta.mention_mail_jobs SET next_attempt_at=now()")
	for {
		if f.scan("SELECT count(*)::text FROM meta.mention_mail_jobs") == "0" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("opted-out recipient retained a pending mail job")
		}
		time.Sleep(50 * time.Millisecond)
	}
	cancel()
	<-done
	failures := strings.Count(logs.String(), "mention email delivery failed")
	f.expectStatus(f.api(recipient, "PATCH", "/auth/me", `{"mentionEmail":true}`), 200, "")
	f.expectStatus(f.api(owner, "POST", path, commentBody("@"+recipient.login+" revoked access")), 201, "")
	if count := f.scan("SELECT count(*)::text FROM meta.mention_mail_jobs"); count != "1" {
		t.Fatalf("expected a pending job before revoking access, got %s", count)
	}
	f.expectStatus(f.api(owner, "DELETE", "/grants/"+grant.Grant.ID, ""), 200, "")
	retryCtx, retryCancel := context.WithCancel(context.Background())
	retryDone := make(chan struct{})
	go func() {
		defer close(retryDone)
		mail.Run(retryCtx)
	}()
	defer func() { retryCancel(); <-retryDone }()
	deadline = time.Now().Add(5 * time.Second)
	for {
		if f.scan("SELECT count(*)::text FROM meta.mention_mail_jobs") == "0" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("recipient with revoked access retained a pending mail job")
		}
		time.Sleep(50 * time.Millisecond)
	}
	retryCancel()
	<-retryDone
	if strings.Count(logs.String(), "mention email delivery failed") != failures {
		t.Fatal("mailer attempted delivery after table access was revoked")
	}
	if strings.Contains(logs.String(), secret) || strings.Contains(logs.String(), username) {
		t.Fatal("SMTP credentials leaked into logs")
	}
}
