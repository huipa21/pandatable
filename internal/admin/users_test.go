package admin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"

	"pandatable/internal/acl"
	"pandatable/internal/apperr"
	"pandatable/internal/auth"
	"pandatable/internal/db"
	"pandatable/internal/testdb"
)

var database testdb.Suite

func TestMain(m *testing.M) { os.Exit(database.Run(m)) }

const password = "Fixture-password-123"

type fixture struct {
	t          *testing.T
	ctx        context.Context
	pool       *db.Pool
	auth       *auth.Service
	service    *Service
	resolver   *acl.Resolver
	users      []string
	workspaces []string
	admin      string
	token      string
	phc        string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{t: t, pool: database.Open(t)}
	var cancel context.CancelFunc
	f.ctx, cancel = context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)
	var err error
	f.auth, err = auth.New(f.pool)
	if err != nil {
		t.Fatal(err)
	}
	f.phc, err = f.auth.HashPassword(f.ctx, password)
	if err != nil {
		t.Fatal(err)
	}
	f.resolver, err = acl.NewResolver(f.pool)
	if err != nil {
		t.Fatal(err)
	}
	f.service, err = NewService(f.pool, f.auth, f.resolver)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		f.exec(ctx, "DELETE FROM meta.grants WHERE principal_id=ANY($1::uuid[])", f.users)
		f.exec(ctx, "DELETE FROM meta.workspaces WHERE id=ANY($1::uuid[])", f.workspaces)
		f.exec(ctx, "DELETE FROM meta.users WHERE id=ANY($1::uuid[])", f.users)
	})
	f.admin, f.token = f.account("admin", "active", false)
	return f
}

func (f *fixture) exec(ctx context.Context, query string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(ctx, query, args...); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) account(role, status string, forced bool) (string, string) {
	f.t.Helper()
	var id string
	name := strings.ToLower(rand.Text())[:16]
	if err := f.pool.QueryRow(f.ctx, `INSERT INTO meta.users
		(username,email,display_name,password_hash,role,status,must_change_password,avatar_color)
		VALUES ($1,$2,$1,$3,$4,$5,$6,'teal') RETURNING id::text`,
		name, name+"@test.example", f.phc, role, status, forced).Scan(&id); err != nil {
		f.t.Fatal(err)
	}
	f.users = append(f.users, id)
	return id, f.session(id)
}

func (f *fixture) session(id string) string {
	f.t.Helper()
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		f.t.Fatal(err)
	}
	token := base64.RawURLEncoding.EncodeToString(value)
	hash := sha256.Sum256([]byte(token))
	f.exec(f.ctx, `INSERT INTO meta.sessions(id_hash,user_id,expires_at)
		VALUES ($1,$2,now()+interval '7 days')`, hash[:], id)
	return token
}

func ptr(value string) *string { return &value }

func code(t *testing.T, err error, want string) {
	t.Helper()
	var app *apperr.Error
	if !errors.As(err, &app) || app.Code != want {
		t.Fatalf("error=%v, want %s", err, want)
	}
}

func (f *fixture) countSessions(id string, want int) {
	f.t.Helper()
	var count int
	if err := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM meta.sessions WHERE user_id=$1", id).Scan(&count); err != nil {
		f.t.Fatal(err)
	}
	if count != want {
		f.t.Fatalf("sessions=%d want=%d", count, want)
	}
}

func TestAdminCreateNormalizeSearchAndCollisions(t *testing.T) {
	f := newFixture(t)
	input := CreateInput{Username: "  Alice  ", Email: ptr(" Alice@Example.Test "),
		DisplayName: "Literal %_ Alice", Role: "user", Password: password}
	user, err := f.service.Create(f.ctx, f.token, input)
	if err != nil {
		t.Fatal(err)
	}
	f.users = append(f.users, user.ID)
	if user.Username != "alice" || user.Email == nil || *user.Email != "alice@example.test" ||
		user.Status != "active" || !user.MustChangePassword || user.LastLoginAt != nil {
		t.Fatalf("invalid created user: %+v", user)
	}
	payload, _ := json.Marshal(user)
	for _, secret := range []string{password, "passwordHash", f.phc} {
		if strings.Contains(string(payload), secret) {
			t.Fatal("DTO exposes password")
		}
	}
	for _, q := range []string{" ALICE ", "example.test", "%_"} {
		result, err := f.service.List(f.ctx, f.token, ListInput{Q: q, Limit: 1})
		if err != nil || result.Total != 1 || len(result.Items) != 1 || result.Items[0].ID != user.ID {
			t.Fatalf("search %q result=%+v err=%v", q, result, err)
		}
	}
	result, err := f.service.List(f.ctx, f.token, ListInput{Q: "alice", Offset: 1, Limit: 500})
	if err != nil || result.Total != 1 || result.Items == nil || len(result.Items) != 0 {
		t.Fatalf("offset page=%+v err=%v", result, err)
	}
	_, err = f.service.Create(f.ctx, f.token, input)
	code(t, err, "ERR_IDENTITY_CONFLICT")
	input.Username = "another"
	_, err = f.service.Create(f.ctx, f.token, input)
	code(t, err, "ERR_IDENTITY_CONFLICT")
	// Legacy email/username ambiguity must also be rejected, even though new
	// canonical usernames cannot contain '@'.
	legacy, _ := f.account("user", "disabled", false)
	f.exec(f.ctx, "UPDATE meta.users SET email='legacyname' WHERE id=$1", legacy)
	input.Username, input.Email = "LegacyName", nil
	_, err = f.service.Create(f.ctx, f.token, input)
	code(t, err, "ERR_IDENTITY_CONFLICT")
	f.exec(f.ctx, "UPDATE meta.users SET username='legacy@example.test',email=NULL WHERE id=$1", legacy)
	_, err = f.service.Update(f.ctx, f.token, user.ID, UserPatch{EmailSet: true, Email: ptr("LEGACY@example.test")})
	code(t, err, "ERR_IDENTITY_CONFLICT")
	updated, err := f.service.Update(f.ctx, f.token, user.ID, UserPatch{EmailSet: true, Email: ptr("   "),
		DisplayName: ptr("Updated")})
	if err != nil || updated.User.Email != nil || updated.User.DisplayName != "Updated" || updated.SessionsRevoked {
		t.Fatalf("clear email=%+v err=%v", updated, err)
	}
}

func TestAdminValidationAndAuthorization(t *testing.T) {
	f := newFixture(t)
	for _, state := range []struct {
		role, status, code string
		forced             bool
	}{{"user", "active", "ERR_PERMISSION_DENIED", false}, {"admin", "active", "ERR_PASSWORD_CHANGE_REQUIRED", true},
		{"admin", "disabled", "ERR_UNAUTHENTICATED", false}} {
		_, token := f.account(state.role, state.status, state.forced)
		_, err := f.service.List(f.ctx, token, ListInput{Limit: 50})
		code(t, err, state.code)
		_, err = f.service.Create(f.ctx, token, CreateInput{})
		code(t, err, state.code)
	}
	for _, input := range []ListInput{{Limit: 0}, {Limit: 501}, {Limit: 1, Offset: -1}, {Limit: 1, Status: "other"},
		{Limit: 1, Q: strings.Repeat("x", 255)}, {Limit: 1, Q: "x\x00y"}} {
		_, err := f.service.List(f.ctx, f.token, input)
		code(t, err, "ERR_VALIDATION")
	}
	for _, input := range []CreateInput{
		{Username: "ab", DisplayName: "A", Role: "user", Password: password},
		{Username: "good", DisplayName: "", Role: "user", Password: password},
		{Username: "good", DisplayName: "A", Role: "owner", Password: password},
		{Username: "good", DisplayName: "A", Role: "user", Password: "short"},
		{Username: "good", DisplayName: "A", Role: "user", Password: password, Email: ptr("Name <a@example.test>")},
	} {
		_, err := f.service.Create(f.ctx, f.token, input)
		code(t, err, "ERR_VALIDATION")
	}
	for _, patch := range []UserPatch{{}, {Role: ptr("owner")}, {Status: ptr("trash")}, {DisplayName: ptr("bad\n")}} {
		_, err := f.service.Update(f.ctx, f.token, f.admin, patch)
		code(t, err, "ERR_VALIDATION")
	}
	_, err := f.service.Update(f.ctx, f.token, "bad-id", UserPatch{Status: ptr("disabled")})
	code(t, err, "ERR_NOT_FOUND")
	_, err = f.service.Update(f.ctx, f.token, "00000000-0000-0000-0000-000000000001", UserPatch{Status: ptr("disabled")})
	code(t, err, "ERR_NOT_FOUND")
}

func TestAdminDisableEnableRevocationAndACL(t *testing.T) {
	f := newFixture(t)
	id, token := f.account("user", "active", false)
	f.session(id)
	var workspace string
	if err := f.pool.QueryRow(f.ctx, "INSERT INTO meta.workspaces(name,created_by) VALUES ('Retained',$1) RETURNING id::text",
		id).Scan(&workspace); err != nil {
		t.Fatal(err)
	}
	f.workspaces = append(f.workspaces, workspace)
	f.exec(f.ctx, `INSERT INTO meta.grants(resource_type,resource_id,principal_type,principal_id,level)
		VALUES ('workspace',$1,'user',$2,'owner')`, workspace, id)
	resource, _ := acl.NewResource("workspace", workspace)
	before, err := f.resolver.Access(f.ctx, id, resource)
	if err != nil || before.Level != acl.Owner {
		t.Fatalf("prime cache=%+v err=%v", before, err)
	}
	result, err := f.service.Update(f.ctx, f.token, id, UserPatch{Status: ptr("disabled")})
	if err != nil || !result.SessionsRevoked || result.User.Status != "disabled" {
		t.Fatalf("disable=%+v err=%v", result, err)
	}
	f.countSessions(id, 0)
	after, err := f.resolver.Access(f.ctx, id, resource)
	if err != nil || after.Visible || after.Level != acl.None {
		t.Fatalf("disabled cached access=%+v err=%v", after, err)
	}
	_, err = f.auth.Authenticate(f.ctx, token)
	code(t, err, "ERR_UNAUTHENTICATED")
	var name, creator string
	if err := f.pool.QueryRow(f.ctx, "SELECT name,created_by::text FROM meta.workspaces WHERE id=$1", workspace).
		Scan(&name, &creator); err != nil || name != "Retained" || creator != id {
		t.Fatal("disable deleted data/authorship", err)
	}
	var username string
	if err := f.pool.QueryRow(f.ctx, "SELECT username FROM meta.users WHERE id=$1", id).Scan(&username); err != nil {
		t.Fatal(err)
	}
	_, _, err = f.auth.Login(f.ctx, username, password, netip.MustParseAddr("192.0.2.1"), "")
	code(t, err, "ERR_INVALID_CREDENTIALS")
	disabled, err := f.service.List(f.ctx, f.token, ListInput{Status: "disabled", Limit: 50})
	if err != nil || disabled.Total != 1 || disabled.Items[0].ID != id {
		t.Fatal("disabled list", err)
	}
	// Even an out-of-band surviving session is revoked when re-enabling.
	f.session(id)
	_, err = f.service.Update(f.ctx, f.token, id, UserPatch{Status: ptr("active")})
	if err != nil {
		t.Fatal(err)
	}
	f.countSessions(id, 0)
	access, err := f.resolver.Access(f.ctx, id, resource)
	if err != nil || access.Level != acl.Owner {
		t.Fatal("enable failed to invalidate disabled cache", err)
	}
	// Admin role alone never gains resource access.
	access, err = f.resolver.Access(f.ctx, f.admin, resource)
	if err != nil || access.Visible {
		t.Fatal("admin gained implicit data access", err)
	}
}

func TestAdminLastActiveAdminAndSelfReset(t *testing.T) {
	f := newFixture(t)
	for _, patch := range []UserPatch{{Status: ptr("disabled")}, {Role: ptr("user")},
		{Role: ptr("user"), Status: ptr("disabled")}} {
		_, err := f.service.Update(f.ctx, f.token, f.admin, patch)
		code(t, err, "ERR_LAST_ADMIN")
		f.countSessions(f.admin, 1)
	}
	f.account("admin", "disabled", false)
	_, err := f.service.Update(f.ctx, f.token, f.admin, UserPatch{Role: ptr("user")})
	code(t, err, "ERR_LAST_ADMIN")
	second, token := f.account("admin", "active", false)
	_, err = f.service.Update(f.ctx, token, second, UserPatch{Role: ptr("user")})
	if err != nil {
		t.Fatal(err)
	}
	f.countSessions(second, 0)
	_, err = f.service.List(f.ctx, token, ListInput{Limit: 1})
	code(t, err, "ERR_UNAUTHENTICATED")
	secondToken := f.session(second)
	_, err = f.service.Update(f.ctx, f.token, second, UserPatch{Role: ptr("admin")})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.service.Update(f.ctx, secondToken, second, UserPatch{Status: ptr("disabled")})
	code(t, err, "ERR_UNAUTHENTICATED")
	secondToken = f.session(second)
	_, err = f.service.Update(f.ctx, secondToken, second, UserPatch{Status: ptr("disabled")})
	if err != nil {
		t.Fatal("self-disable while another admin remains:", err)
	}
	f.countSessions(second, 0)
	f.session(f.admin)
	if err := f.service.ResetPassword(f.ctx, f.token, f.admin, "Reset-password-123"); err != nil {
		t.Fatal(err)
	}
	f.countSessions(f.admin, 0)
	var forced bool
	var username string
	if err := f.pool.QueryRow(f.ctx, "SELECT must_change_password,username FROM meta.users WHERE id=$1", f.admin).
		Scan(&forced, &username); err != nil || !forced {
		t.Fatal("self-reset must force change", err)
	}
	session, newToken, err := f.auth.Login(f.ctx, username, "Reset-password-123", netip.MustParseAddr("192.0.2.1"), "")
	if err != nil || !session.User.MustChangePassword {
		t.Fatal("temporary password login", err)
	}
	_, err = f.service.List(f.ctx, newToken, ListInput{Limit: 1})
	code(t, err, "ERR_PASSWORD_CHANGE_REQUIRED")
	if err := f.auth.ChangePassword(f.ctx, newToken, "Reset-password-123", "Changed-password-123"); err != nil {
		t.Fatal(err)
	}
	f.countSessions(f.admin, 0)
}

func TestAdminRollbackRevocationAtomicity(t *testing.T) {
	f := newFixture(t)
	id, _ := f.account("user", "active", false)
	f.exec(f.ctx, `CREATE FUNCTION meta.fail_admin_session_delete() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'fixture revocation failure'; END $$`)
	f.exec(f.ctx, `CREATE TRIGGER fail_admin_session_delete BEFORE DELETE ON meta.sessions
		FOR EACH ROW EXECUTE FUNCTION meta.fail_admin_session_delete()`)
	t.Cleanup(func() {
		f.exec(context.Background(), "DROP TRIGGER fail_admin_session_delete ON meta.sessions")
		f.exec(context.Background(), "DROP FUNCTION meta.fail_admin_session_delete()")
	})
	for _, operation := range []func() error{
		func() error {
			_, err := f.service.Update(f.ctx, f.token, id, UserPatch{Status: ptr("disabled")})
			return err
		},
		func() error { return f.service.ResetPassword(f.ctx, f.token, id, "Reset-password-123") },
	} {
		if err := operation(); err == nil {
			t.Fatal("revocation failure must fail the entire mutation")
		}
		var status, phc string
		var forced bool
		if err := f.pool.QueryRow(f.ctx, "SELECT status,password_hash,must_change_password FROM meta.users WHERE id=$1", id).
			Scan(&status, &phc, &forced); err != nil || status != "active" || phc != f.phc || forced {
			t.Fatal("partial transaction after rollback", err)
		}
		f.countSessions(id, 1)
	}
}

func TestAdminConcurrentLastAdminChanges(t *testing.T) {
	f := newFixture(t)
	second, secondToken := f.account("admin", "active", false)
	start := make(chan struct{})
	outcomes := make(chan error, 2)
	for _, actor := range []struct{ id, token string }{{f.admin, f.token}, {second, secondToken}} {
		go func(id, token string) {
			<-start
			_, err := f.service.Update(f.ctx, token, id, UserPatch{Role: ptr("user")})
			outcomes <- err
		}(actor.id, actor.token)
	}
	close(start)
	first, next := <-outcomes, <-outcomes
	if first == nil && next == nil || first != nil && next != nil {
		t.Fatalf("exactly one demotion should commit: %v / %v", first, next)
	}
	if first != nil {
		code(t, first, "ERR_LAST_ADMIN")
	} else {
		code(t, next, "ERR_LAST_ADMIN")
	}
	var count int
	if err := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM meta.users WHERE role='admin' AND status='active'").
		Scan(&count); err != nil || count != 1 {
		t.Fatal("concurrent lockout", err)
	}
}

func TestAdminWaitsForUserRowAndRechecksCaller(t *testing.T) {
	f := newFixture(t)
	id, _ := f.account("user", "active", false)
	second, token := f.account("admin", "active", false)
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(f.ctx, "SELECT id FROM meta.users WHERE id=$1 FOR UPDATE", second); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := f.service.Update(f.ctx, token, id, UserPatch{DisplayName: ptr("Must not commit")})
		done <- err
	}()
	awaitBlocked(t, f)
	if _, err := tx.Exec(f.ctx, "UPDATE meta.users SET role='user' WHERE id=$1", second); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(f.ctx, "DELETE FROM meta.sessions WHERE user_id=$1", second); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	code(t, <-done, "ERR_UNAUTHENTICATED")

	// Reset/status operations must serialize with auth's user-row protocol.
	tx, err = f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(f.ctx, "SELECT id FROM meta.users WHERE id=$1 FOR UPDATE", id); err != nil {
		t.Fatal(err)
	}
	go func() {
		_, err := f.service.Update(f.ctx, f.token, id, UserPatch{Status: ptr("disabled")})
		done <- err
	}()
	awaitBlocked(t, f)
	if err := tx.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	f.countSessions(id, 0)
}

func awaitBlocked(t *testing.T, f *fixture) {
	awaitBlockedCount(t, f, 1)
}

func awaitBlockedCount(t *testing.T, f *fixture, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var count int
		err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM pg_stat_activity
			WHERE datname=current_database() AND wait_event_type='Lock' AND pid<>pg_backend_pid()`).
			Scan(&count)
		if err != nil {
			t.Fatal(err)
		}
		if count >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no PostgreSQL lock evidence")
}

func TestAdminConcurrentIdentityUpdates(t *testing.T) {
	f := newFixture(t)
	first, _ := f.account("user", "active", false)
	second, _ := f.account("user", "active", false)
	start := make(chan struct{})
	done := make(chan error, 2)
	for _, id := range []string{first, second} {
		go func(id string) {
			<-start
			_, err := f.service.Update(f.ctx, f.token, id, UserPatch{EmailSet: true, Email: ptr("SHARED@example.test")})
			done <- err
		}(id)
	}
	close(start)
	a, b := <-done, <-done
	if a == nil && b == nil || a != nil && b != nil {
		t.Fatalf("exactly one identity update should commit: %v / %v", a, b)
	}
	if a != nil {
		code(t, a, "ERR_IDENTITY_CONFLICT")
	} else {
		code(t, b, "ERR_IDENTITY_CONFLICT")
	}
}

func TestAdminResetSerializesWithOldPasswordLogin(t *testing.T) {
	f := newFixture(t)
	id, _ := f.account("user", "active", false)
	var username string
	if err := f.pool.QueryRow(f.ctx, "SELECT username FROM meta.users WHERE id=$1", id).Scan(&username); err != nil {
		t.Fatal(err)
	}
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(f.ctx, "SELECT id FROM meta.users WHERE id=$1 FOR UPDATE", id); err != nil {
		t.Fatal(err)
	}
	reset := make(chan error, 1)
	go func() { reset <- f.service.ResetPassword(f.ctx, f.token, id, "Reset-password-123") }()
	awaitBlocked(t, f)
	login := make(chan error, 1)
	go func() {
		_, _, err := f.auth.Login(f.ctx, username, password, netip.MustParseAddr("192.0.2.1"), "")
		login <- err
	}()
	awaitBlockedCount(t, f, 2)
	if err := tx.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-reset; err != nil {
		t.Fatal(err)
	}
	code(t, <-login, "ERR_INVALID_CREDENTIALS")
	f.countSessions(id, 0)
}
