package auth

import (
	"bytes"
	"context"
	"net/netip"
	"strings"
	"testing"
	"time"

	"pandatable/internal/db"
)

type setupFixture struct {
	pool    *db.Pool
	service *Service
	ctx     context.Context
	now     time.Time
	token   string
}

func newSetupFixture(t *testing.T) *setupFixture {
	t.Helper()
	pool := database.Open(t)
	service, err := New(pool)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	f := &setupFixture{pool: pool, service: service, ctx: ctx,
		now: time.Now().UTC().Truncate(time.Microsecond)}
	service.now = func() time.Time { return f.now }
	var needed bool
	f.token, needed, err = service.InitializeSetup(ctx)
	if err != nil || !needed || f.token == "" {
		t.Fatalf("initialize empty setup: needed=%t token=%t err=%v", needed, f.token != "", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM meta.grants WHERE resource_type='workspace'
			AND resource_id IN (SELECT w.id FROM meta.workspaces w JOIN meta.users u ON u.id=w.created_by
				WHERE u.email LIKE '%@setup.test');
			DELETE FROM meta.workspaces WHERE created_by IN (SELECT id FROM meta.users WHERE email LIKE '%@setup.test')`); err != nil {
			t.Error("cleanup setup workspaces:", err)
		}
		if _, err := pool.Exec(cleanupCtx, "DELETE FROM meta.users WHERE email LIKE '%@setup.test'"); err != nil {
			t.Error("cleanup setup users:", err)
		}
		if _, err := pool.Exec(cleanupCtx, "DELETE FROM meta.settings WHERE key='setup_completed'"); err != nil {
			t.Error("cleanup setup completion marker:", err)
		}
	})
	return f
}

func (fixture *setupFixture) input(token, username, email string) SetupInput {
	return SetupInput{Token: token, Username: username, Email: email,
		DisplayName: "First Administrator", Password: "12345678", Locale: "zh-CN"}
}

func TestSetupTokenExpiryAndRestartRotation(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	token, digest, err := newToken()
	if err != nil {
		t.Fatal(err)
	}
	clear(digest)
	service := &Service{now: func() time.Time { return now }, setupToken: token, setupExpiry: now.Add(SetupTokenLifetime)}
	if !service.hasSetupToken(token, now) || service.hasSetupToken("wrong", now) {
		t.Fatal("setup token validation failed")
	}
	if service.hasSetupToken(token, now.Add(SetupTokenLifetime)) {
		t.Fatal("setup token remained valid at its expiry boundary")
	}
	rotated, rotatedDigest, err := newToken()
	if err != nil {
		t.Fatal(err)
	}
	clear(rotatedDigest)
	service.setupToken = rotated
	service.setupExpiry = now.Add(SetupTokenLifetime)
	if rotated == token || service.hasSetupToken(token, now) {
		t.Fatal("restart token rotation must invalidate the previous process token")
	}
	if !service.hasSetupToken(rotated, now) {
		t.Fatal("fresh process token is invalid")
	}
}

func TestNormalizeSetupInputBoundaries(t *testing.T) {
	valid := func(username, password string) (SetupInput, error) {
		return normalizeSetupInput(SetupInput{
			Token: "not-validated-here", Username: username, Email: "  OWNER@Example.TEST ",
			DisplayName: "First Admin", Password: password, Locale: "en",
		})
	}
	for _, test := range []struct {
		name     string
		username string
		password string
		wantFail bool
	}{
		{"minimum password", " Admin_1 ", "12345678", false},
		{"maximum password", "Admin_1", strings.Repeat("x", 32), false},
		{"password is not trimmed", "Admin_1", " 123456 ", false},
		{"short password", "Admin_1", "1234567", true},
		{"long password", "Admin_1", strings.Repeat("x", 33), true},
		{"minimum username", " ABC ", "12345678", false},
		{"maximum username", strings.Repeat("A", 32), "12345678", false},
		{"short username", "ab", "12345678", true},
		{"long username", strings.Repeat("a", 33), "12345678", true},
		{"invalid username character", "admin name", "12345678", true},
		{"unicode username", "用户abc", "12345678", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			input, err := valid(test.username, test.password)
			if (err != nil) != test.wantFail {
				t.Fatalf("normalize error=%v, wantFail=%t", err, test.wantFail)
			}
			if err == nil && (input.Username != strings.ToLower(strings.TrimSpace(test.username)) || input.Email != "owner@example.test") {
				t.Fatalf("identities were not normalized: %+v", input)
			}
		})
	}
	for _, email := range []string{"", "not-an-email", "Admin <owner@example.test>"} {
		input := SetupInput{Username: "admin", Email: email, DisplayName: "Admin", Password: "12345678", Locale: "en"}
		if _, err := normalizeSetupInput(input); err == nil {
			t.Fatalf("accepted invalid required email %q", email)
		}
	}
}

func TestSetupFlowPostgreSQL(t *testing.T) {
	f := newSetupFixture(t)
	needed, err := f.service.SetupNeeded(f.ctx)
	if err != nil || !needed {
		t.Fatalf("empty database setup status: needed=%t err=%v", needed, err)
	}
	input := f.input(f.token, " Setup-Admin ", " OWNER@Setup.Test ")
	wrong := input
	wrong.Token = strings.Repeat("x", len(f.token))
	if _, _, err := f.service.Setup(f.ctx, wrong, netip.MustParseAddr("192.0.2.10"), "owned setup test"); err == nil {
		t.Fatal("wrong token accepted")
	} else {
		assertCode(t, err, "ERR_SETUP_TOKEN")
	}
	var users, sessions, markers int
	if err := f.pool.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM meta.users),
		(SELECT count(*) FROM meta.sessions),
		(SELECT count(*) FROM meta.settings WHERE key='setup_completed')`).Scan(&users, &sessions, &markers); err != nil || users != 0 || sessions != 0 || markers != 0 {
		t.Fatalf("wrong token created partial setup: users=%d sessions=%d markers=%d err=%v", users, sessions, markers, err)
	}
	session, rawToken, err := f.service.Setup(f.ctx, input, netip.MustParseAddr("192.0.2.10"), "owned setup test")
	if err != nil {
		t.Fatal("create first administrator:", err)
	}
	if session.User.Username != "setup-admin" || session.User.Email == nil || *session.User.Email != "owner@setup.test" ||
		session.User.DisplayName != input.DisplayName || session.User.Role != "admin" || session.User.Locale != "zh-CN" ||
		session.User.MustChangePassword || session.User.EffectiveTimezone != "Asia/Shanghai" {
		t.Fatalf("incorrect first-admin profile: %+v", session.User)
	}
	var starterOK bool
	if err := f.pool.QueryRow(f.ctx, `SELECT EXISTS (
		SELECT 1 FROM meta.user_workspace_preferences p
		JOIN meta.workspaces w ON w.id=p.workspace_id
		JOIN meta.grants g ON g.resource_type='workspace' AND g.resource_id=w.id
		WHERE p.user_id=$1 AND w.created_by=$1 AND w.name='Default' AND w.deleted_at IS NULL
		  AND g.principal_type='user' AND g.principal_id=$1 AND g.level='owner')`, session.User.ID).Scan(&starterOK); err != nil || !starterOK {
		t.Fatalf("first administrator starter workspace: ok=%t err=%v", starterOK, err)
	}
	var role, passwordHash, avatarColor string
	var email string
	var lastLogin time.Time
	var storedSessionHash []byte
	if err := f.pool.QueryRow(f.ctx, `SELECT u.role,u.password_hash,u.avatar_color,u.email,u.last_login_at,s.id_hash
		FROM meta.users u JOIN meta.sessions s ON s.user_id=u.id WHERE u.id=$1`, session.User.ID).
		Scan(&role, &passwordHash, &avatarColor, &email, &lastLogin, &storedSessionHash); err != nil {
		t.Fatal("read first-admin storage:", err)
	}
	wantSessionHash, err := tokenHash(rawToken)
	if err != nil || role != "admin" || email != "owner@setup.test" || avatarColor != "teal" ||
		!lastLogin.Equal(f.now) || !strings.HasPrefix(passwordHash, "$argon2id$v=19$m=65536,t=1,p=2$") ||
		len(storedSessionHash) != 32 || !bytes.Equal(storedSessionHash, wantSessionHash) ||
		bytes.Contains(storedSessionHash, []byte(rawToken)) {
		t.Fatal("incorrect or unsafe persisted first-admin/session data")
	}
	if match, err := f.service.passwords.Verify(f.ctx, input.Password, passwordHash); err != nil || !match {
		t.Fatal("stored first-admin password does not verify")
	}
	if _, err := f.service.Authenticate(f.ctx, rawToken); err != nil {
		t.Fatal("setup session did not authenticate:", err)
	}
	needed, err = f.service.SetupNeeded(f.ctx)
	if err != nil || needed {
		t.Fatalf("completed setup status: needed=%t err=%v", needed, err)
	}
	if _, _, err := f.service.Setup(f.ctx, input, netip.MustParseAddr("192.0.2.10"), "owned setup test"); err == nil {
		t.Fatal("setup accepted after completion")
	} else {
		assertCode(t, err, "ERR_SETUP_COMPLETE")
	}
	if rotated, needed, err := f.service.InitializeSetup(f.ctx); err != nil || needed || rotated != "" {
		t.Fatalf("completed setup issued a bootstrap token: needed=%t token=%t err=%v", needed, rotated != "", err)
	}
	t.Log("AT-101 service: wrong token is atomic, first admin fields/hash/session are exact, completion is one-time")
}

func TestSetupTransactionalRollbackPostgreSQL(t *testing.T) {
	f := newSetupFixture(t)
	if _, err := f.pool.Exec(f.ctx, `CREATE FUNCTION meta.test_reject_setup_session() RETURNS trigger
		LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'forced setup rollback'; END $$`); err != nil {
		t.Fatal("create rollback trigger function:", err)
	}
	if _, err := f.pool.Exec(f.ctx, `CREATE TRIGGER test_reject_setup_session BEFORE INSERT ON meta.sessions
		FOR EACH ROW EXECUTE FUNCTION meta.test_reject_setup_session()`); err != nil {
		t.Fatal("create rollback trigger:", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = f.pool.Exec(ctx, "DROP TRIGGER IF EXISTS test_reject_setup_session ON meta.sessions")
		_, _ = f.pool.Exec(ctx, "DROP FUNCTION IF EXISTS meta.test_reject_setup_session()")
	})
	input := f.input(f.token, "setup-rollback", "rollback@setup.test")
	if _, _, err := f.service.Setup(f.ctx, input, netip.MustParseAddr("192.0.2.11"), "rollback test"); err == nil {
		t.Fatal("forced session insert failure unexpectedly succeeded")
	}
	var users, sessions, markers, workspaces, grants, preferences int
	if err := f.pool.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM meta.users),
		(SELECT count(*) FROM meta.sessions),
		(SELECT count(*) FROM meta.settings WHERE key='setup_completed'),
		(SELECT count(*) FROM meta.workspaces), (SELECT count(*) FROM meta.grants),
		(SELECT count(*) FROM meta.user_workspace_preferences)`).
		Scan(&users, &sessions, &markers, &workspaces, &grants, &preferences); err != nil {
		t.Fatal(err)
	}
	if users != 0 || sessions != 0 || markers != 0 || workspaces != 0 || grants != 0 || preferences != 0 {
		t.Fatalf("failed setup left partial rows: users=%d sessions=%d markers=%d workspaces=%d grants=%d preferences=%d",
			users, sessions, markers, workspaces, grants, preferences)
	}
	if _, err := f.pool.Exec(f.ctx, "DROP TRIGGER test_reject_setup_session ON meta.sessions"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, "DROP FUNCTION meta.test_reject_setup_session()"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.service.Setup(f.ctx, input, netip.MustParseAddr("192.0.2.11"), "rollback test"); err != nil {
		t.Fatal("rollback consumed setup token:", err)
	}
}

func TestConcurrentSetupProcessesPostgreSQL(t *testing.T) {
	f := newSetupFixture(t)
	otherService, err := New(f.pool)
	if err != nil {
		t.Fatal(err)
	}
	otherService.now = func() time.Time { return f.now }
	otherToken, needed, err := otherService.InitializeSetup(f.ctx)
	if err != nil || !needed || otherToken == f.token {
		t.Fatalf("initialize second process token: needed=%t distinct=%t err=%v", needed, otherToken != f.token, err)
	}
	type result struct {
		session Session
		err     error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	for index, service := range []*Service{f.service, otherService} {
		index, service := index, service
		go func() {
			<-start
			token, username, email := f.token, "setup-process-a", "process-a@setup.test"
			if index == 1 {
				token, username, email = otherToken, "setup-process-b", "process-b@setup.test"
			}
			session, _, err := service.Setup(f.ctx, f.input(token, username, email), netip.MustParseAddr("192.0.2.12"), "concurrent setup")
			results <- result{session: session, err: err}
		}()
	}
	close(start)
	first, second := <-results, <-results
	successes, completed := 0, 0
	for _, attempt := range []result{first, second} {
		if attempt.err == nil {
			successes++
			if attempt.session.User.Role != "admin" {
				t.Fatal("concurrent setup returned non-admin")
			}
		} else {
			assertCode(t, attempt.err, "ERR_SETUP_COMPLETE")
			completed++
		}
	}
	if successes != 1 || completed != 1 {
		t.Fatalf("concurrent setup outcomes: success=%d completed=%d", successes, completed)
	}
	var users, sessions int
	if err := f.pool.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM meta.users),
		(SELECT count(*) FROM meta.sessions)`).Scan(&users, &sessions); err != nil || users != 1 || sessions != 1 {
		t.Fatalf("cross-process setup created duplicates: users=%d sessions=%d err=%v", users, sessions, err)
	}
	t.Log("two independently initialized setup services serialize through PostgreSQL; exactly one first admin and session commit")
}

func TestSetupRejectsExistingActiveOrDisabledUsersPostgreSQL(t *testing.T) {
	for _, status := range []string{"active", "disabled"} {
		t.Run(status, func(t *testing.T) {
			f := newSetupFixture(t)
			passwordHash, err := f.service.passwords.Hash(f.ctx, "Existing-password-123")
			if err != nil {
				t.Fatal(err)
			}
			_, err = f.pool.Exec(f.ctx, `INSERT INTO meta.users
				(username,email,display_name,password_hash,role,status,avatar_color)
				VALUES ('setup-existing',$1,'Existing',$2,'user',$3,'blue')`, status+"@setup.test", passwordHash, status)
			if err != nil {
				t.Fatal("insert existing user:", err)
			}
			needed, err := f.service.SetupNeeded(f.ctx)
			if err != nil || needed {
				t.Fatalf("existing %s user setup status: needed=%t err=%v", status, needed, err)
			}
			if _, _, err := f.service.Setup(f.ctx, f.input(f.token, "setup-new", "new@setup.test"), netip.MustParseAddr("192.0.2.13"), "existing user test"); err == nil {
				t.Fatal("setup accepted when an existing user is present")
			} else {
				assertCode(t, err, "ERR_SETUP_COMPLETE")
			}
		})
	}
}

func TestSetupTokenExpiryPostgreSQL(t *testing.T) {
	f := newSetupFixture(t)
	f.now = f.now.Add(SetupTokenLifetime)
	_, _, err := f.service.Setup(f.ctx, f.input(f.token, "setup-expired", "expired@setup.test"), netip.MustParseAddr("192.0.2.14"), "expired token test")
	assertCode(t, err, "ERR_SETUP_TOKEN")
	needed, err := f.service.SetupNeeded(f.ctx)
	if err != nil || !needed {
		t.Fatalf("expired token changed setup state: needed=%t err=%v", needed, err)
	}
}
