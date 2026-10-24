package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"net/netip"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/apperr"
	"pandatable/internal/auth/authsql"
	"pandatable/internal/db"
	"pandatable/internal/testdb"
)

var database testdb.Suite

func TestMain(m *testing.M) { os.Exit(database.Run(m)) }

const fixturePassword = "Fixture-password-123"

type authFixture struct {
	pool            *db.Pool
	service         *Service
	ctx             context.Context
	id              pgtype.UUID
	username, email string
	now             time.Time
}

func newFixture(t *testing.T) *authFixture {
	t.Helper()
	pool := database.Open(t)
	service, err := New(pool)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	fixture := &authFixture{pool: pool, service: service, ctx: ctx,
		username: strings.ToLower(rand.Text()), now: time.Now().UTC().Truncate(time.Microsecond)}
	fixture.email = fixture.username + "@example.test"
	service.now = func() time.Time { return fixture.now }
	hash, err := service.passwords.Hash(ctx, fixturePassword)
	if err != nil {
		t.Fatal(err)
	}
	err = pool.QueryRow(ctx, `INSERT INTO meta.users
		(username,email,display_name,password_hash,role,avatar_color)
		VALUES ($1,$2,'Fixture',$3,'user','blue') RETURNING id`, fixture.username, fixture.email, hash).Scan(&fixture.id)
	if err != nil {
		t.Fatal("insert owned auth fixture:", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := pool.Exec(ctx, "DELETE FROM meta.users WHERE id=$1", fixture.id); err != nil {
			t.Error("cleanup owned auth fixture:", err)
		}
	})
	return fixture
}

func (fixture *authFixture) login(t *testing.T, login string) (Session, string) {
	t.Helper()
	session, token, err := fixture.service.Login(fixture.ctx, login, fixturePassword,
		netip.MustParseAddr("192.0.2.1"), "owned fixture")
	if err != nil {
		t.Fatal("fixture login:", err)
	}
	return session, token
}

func assertCode(t *testing.T, err error, code string) {
	t.Helper()
	var app *apperr.Error
	if !errors.As(err, &app) || app.Code != code {
		t.Fatalf("error=%v, want %s", err, code)
	}
}

func (fixture *authFixture) sessionCount(t *testing.T, want int) {
	t.Helper()
	var count int
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT count(*) FROM meta.sessions WHERE user_id=$1", fixture.id).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("session count=%d, want %d", count, want)
	}
}

func TestSessionLifecyclePostgreSQL(t *testing.T) {
	f := newFixture(t)
	session, token := f.login(t, strings.ToUpper(f.username))
	if session.User.ID != f.id.String() || session.User.Timezone != nil || session.User.EffectiveTimezone != "Asia/Shanghai" ||
		!session.ExpiresAt.Equal(f.now.Add(IdleLifetime)) {
		t.Fatal("incorrect initial session/profile")
	}
	var hash []byte
	var created, seen, expires, lastLogin time.Time
	var ip, userAgent, zone string
	if err := f.pool.QueryRow(f.ctx, `SELECT s.id_hash,s.created_at,s.last_seen_at,s.expires_at,
		host(s.ip),s.user_agent,u.last_login_at,current_setting('timezone')
		FROM meta.sessions s JOIN meta.users u ON u.id=s.user_id WHERE s.user_id=$1`, f.id).
		Scan(&hash, &created, &seen, &expires, &ip, &userAgent, &lastLogin, &zone); err != nil {
		t.Fatal(err)
	}
	expected, _ := tokenHash(token)
	if !bytes.Equal(hash, expected) || len(hash) != 32 || bytes.Contains(hash, []byte(token)) ||
		!created.Equal(f.now) || !seen.Equal(f.now) || !lastLogin.Equal(f.now) ||
		!expires.Equal(session.ExpiresAt) || ip != "192.0.2.1" || userAgent != "owned fixture" || zone != "UTC" {
		t.Fatal("unsafe stored session metadata or non-UTC database session")
	}
	start := f.now
	for _, day := range []int{6, 12, 18, 24, 29} {
		f.now = start.Add(time.Duration(day) * 24 * time.Hour)
		current, err := f.service.Authenticate(f.ctx, token)
		if err != nil {
			t.Fatal(err)
		}
		want := f.now.Add(IdleLifetime)
		if absolute := start.Add(AbsoluteLifetime); absolute.Before(want) {
			want = absolute
		}
		if !current.ExpiresAt.Equal(want) {
			t.Fatalf("day %d: sliding expiry=%s want=%s", day, current.ExpiresAt, want)
		}
	}
	// Concurrent requests may reach SQL in a different order than their clocks.
	f.now = start.Add(29*24*time.Hour - time.Minute)
	if _, err := f.service.Authenticate(f.ctx, token); err != nil {
		t.Fatal("out-of-order request should not invalidate an active session:", err)
	}
	f.now = start.Add(AbsoluteLifetime)
	if _, err := f.pool.Exec(f.ctx, "UPDATE meta.sessions SET expires_at=$2 WHERE user_id=$1", f.id, f.now.Add(365*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	_, err := f.service.Authenticate(f.ctx, token)
	assertCode(t, err, "ERR_UNAUTHENTICATED")
	f.now = start.Add(40 * 24 * time.Hour)
	_, idleToken := f.login(t, f.email)
	f.now = f.now.Add(IdleLifetime)
	if _, err := f.pool.Exec(f.ctx, "UPDATE meta.sessions SET expires_at=$2 WHERE user_id=$1", f.id, f.now.Add(365*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	_, err = f.service.Authenticate(f.ctx, idleToken)
	assertCode(t, err, "ERR_UNAUTHENTICATED")
	_, revoked := f.login(t, f.username)
	if err := f.service.Logout(f.ctx, revoked); err != nil {
		t.Fatal(err)
	}
	if err := f.service.Logout(f.ctx, revoked); err != nil {
		t.Fatal("logout should be idempotent for a canonical already-revoked token:", err)
	}
	_, err = f.service.Authenticate(f.ctx, revoked)
	assertCode(t, err, "ERR_UNAUTHENTICATED")
	_, err = f.service.Authenticate(f.ctx, "not-a-token")
	assertCode(t, err, "ERR_UNAUTHENTICATED")
	t.Log("session hash-only storage, UTC metadata, seven-day idle, 30-day absolute, monotonic sliding and revocation verified")
}

func TestSessionExpiryAndDisablePostgreSQL(t *testing.T) {
	f := newFixture(t)
	_, token := f.login(t, f.username)
	hash, _ := tokenHash(token)
	if _, err := f.pool.Exec(f.ctx, "UPDATE meta.sessions SET expires_at=$2 WHERE id_hash=$1", hash, f.now); err != nil {
		t.Fatal(err)
	}
	_, err := f.service.Authenticate(f.ctx, token)
	assertCode(t, err, "ERR_UNAUTHENTICATED")
	_, token = f.login(t, f.username)
	if _, err := f.pool.Exec(f.ctx, "UPDATE meta.users SET status='disabled' WHERE id=$1", f.id); err != nil {
		t.Fatal(err)
	}
	_, err = f.service.Authenticate(f.ctx, token)
	assertCode(t, err, "ERR_UNAUTHENTICATED")
	_, _, err = f.service.Login(f.ctx, f.email, fixturePassword, netip.MustParseAddr("192.0.2.2"), "")
	assertCode(t, err, "ERR_INVALID_CREDENTIALS")
	if err := f.pool.WithTx(f.ctx, func(tx pgx.Tx) error {
		queries := authsql.New(tx)
		if _, err := queries.LockUser(f.ctx, f.id); err != nil {
			return err
		}
		return queries.RevokeUserSessions(f.ctx, f.id)
	}); err != nil {
		t.Fatal(err)
	}
	f.sessionCount(t, 0)
	var status string
	if err := f.pool.QueryRow(f.ctx, "SELECT status FROM meta.users WHERE id=$1", f.id).Scan(&status); err != nil || status != "disabled" {
		t.Fatal("disable must retain the user")
	}
}

func TestConcurrentSessionRevocationPostgreSQL(t *testing.T) {
	f := newFixture(t)
	_, token := f.login(t, f.username)
	start := make(chan struct{})
	results := make(chan error, 9)
	var wait sync.WaitGroup
	for index := range 9 {
		wait.Go(func() {
			<-start
			if index == 0 {
				results <- f.service.Logout(f.ctx, token)
				return
			}
			_, err := f.service.Authenticate(f.ctx, token)
			results <- err
		})
	}
	close(start)
	wait.Wait()
	close(results)
	for err := range results {
		if err != nil {
			assertCode(t, err, "ERR_UNAUTHENTICATED")
		}
	}
	f.sessionCount(t, 0)
	_, err := f.service.Authenticate(f.ctx, token)
	assertCode(t, err, "ERR_UNAUTHENTICATED")
	t.Log("eight concurrent validators cannot recreate a concurrently revoked session")
}

func TestLoginPasswordChangeFencePostgreSQL(t *testing.T) {
	f := newFixture(t)
	_, oldSession := f.login(t, f.username)
	newHash, err := f.service.passwords.Hash(f.ctx, "Next-password-123")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Error(err)
		}
	})
	queries := authsql.New(tx)
	if _, err := queries.LockUser(f.ctx, f.id); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, _, err := f.service.Login(f.ctx, f.email, fixturePassword, netip.MustParseAddr("192.0.2.2"), "")
		result <- err
	}()
	// Wait for PostgreSQL evidence of the old-password login blocked on our user lock.
	waitCtx, cancel := context.WithTimeout(f.ctx, 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		var blocked bool
		err := f.pool.QueryRow(waitCtx, `SELECT EXISTS (
			SELECT 1 FROM pg_stat_activity
			WHERE datname=current_database() AND wait_event_type='Lock'
			AND query LIKE '-- name: LockUser%')`).Scan(&blocked)
		if err != nil {
			t.Fatal(err)
		}
		if blocked {
			break
		}
		select {
		case <-waitCtx.Done():
			t.Fatal("old-password login never reached the serialized user lock")
		case <-ticker.C:
		}
	}
	if err := queries.ChangePassword(f.ctx, authsql.ChangePasswordParams{ID: f.id, PasswordHash: newHash, UpdatedAt: timestamp(f.now)}); err != nil {
		t.Fatal(err)
	}
	if err := queries.RevokeUserSessions(f.ctx, f.id); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		assertCode(t, err, "ERR_INVALID_CREDENTIALS")
	case <-waitCtx.Done():
		t.Fatal("serialized login did not complete")
	}
	f.sessionCount(t, 0)
	_, err = f.service.Authenticate(f.ctx, oldSession)
	assertCode(t, err, "ERR_UNAUTHENTICATED")
	t.Log("old-password verification racing a committed password change cannot create a surviving session")
}

func TestLoginLimitsAndAliasesPostgreSQL(t *testing.T) {
	f := newFixture(t)
	for attempt := range 5 {
		login := f.username
		if attempt%2 == 1 {
			login = strings.ToUpper(f.email)
		}
		password := "wrong-password"
		if attempt == 0 {
			password = "short"
		}
		_, _, err := f.service.Login(f.ctx, login, password, netip.AddrFrom4([4]byte{192, 0, 2, byte(attempt + 1)}), "")
		assertCode(t, err, "ERR_INVALID_CREDENTIALS")
	}
	_, _, err := f.service.Login(f.ctx, f.email, fixturePassword, netip.MustParseAddr("198.51.100.1"), "")
	assertCode(t, err, "ERR_RATE_LIMITED")
	f.now = f.now.Add(time.Minute)
	f.login(t, f.username)
	for range 4 {
		_, _, err := f.service.Login(f.ctx, f.email, "wrong-password", netip.MustParseAddr("198.51.100.2"), "")
		assertCode(t, err, "ERR_INVALID_CREDENTIALS")
	}
	f.login(t, f.email)
	_, _, err = f.service.Login(f.ctx, f.username, "wrong-password", netip.MustParseAddr("198.51.100.2"), "")
	assertCode(t, err, "ERR_INVALID_CREDENTIALS")
	f.login(t, f.email)
	f.now = f.now.Add(time.Minute)
	for range 10 {
		f.login(t, f.email)
	}
	_, _, err = f.service.Login(f.ctx, f.username, fixturePassword, netip.MustParseAddr("192.0.2.1"), "")
	assertCode(t, err, "ERR_RATE_LIMITED")
	t.Log("AT-103: username/email/case aliases share five-failure lock; correct password rejected during lock; exact clock-based expiry and 10/min/IP verified")
}

func TestPasswordChangePostgreSQL(t *testing.T) {
	f := newFixture(t)
	if _, err := f.pool.Exec(f.ctx, "UPDATE meta.users SET must_change_password=true WHERE id=$1", f.id); err != nil {
		t.Fatal(err)
	}
	session, first := f.login(t, f.username)
	_, second := f.login(t, f.email)
	if !session.User.MustChangePassword {
		t.Fatal("forced-change state must be exposed")
	}
	theme := "dark"
	_, err := f.service.UpdateProfile(f.ctx, first, ProfilePatch{Theme: &theme})
	assertCode(t, err, "ERR_PASSWORD_CHANGE_REQUIRED")
	err = f.service.ChangePassword(f.ctx, first, "wrong-password", "Next-password-123")
	assertCode(t, err, "ERR_INVALID_CREDENTIALS")
	f.sessionCount(t, 2)
	err = f.service.ChangePassword(f.ctx, first, fixturePassword, "short")
	assertCode(t, err, "ERR_VALIDATION")
	if err := f.service.ChangePassword(f.ctx, first, fixturePassword, "Next-password-123"); err != nil {
		t.Fatal(err)
	}
	f.sessionCount(t, 0)
	for _, token := range []string{first, second} {
		_, err := f.service.Authenticate(f.ctx, token)
		assertCode(t, err, "ERR_UNAUTHENTICATED")
	}
	_, _, err = f.service.Login(f.ctx, f.username, fixturePassword, netip.MustParseAddr("192.0.2.1"), "")
	assertCode(t, err, "ERR_INVALID_CREDENTIALS")
	current, _, err := f.service.Login(f.ctx, f.email, "Next-password-123", netip.MustParseAddr("192.0.2.1"), "")
	if err != nil || current.User.MustChangePassword {
		t.Fatal("new password must work and clear forced-change state:", err)
	}
	t.Log("API password change verifies current password and atomically revokes all sessions; old password fails and forced-change state clears")
}

func TestProfilePreferencesPostgreSQL(t *testing.T) {
	f := newFixture(t)
	_, token := f.login(t, f.username)
	name, locale, theme, timezone := "爱丽丝", "zh-CN", "dark", "Europe/London"
	user, err := f.service.UpdateProfile(f.ctx, token, ProfilePatch{
		DisplayName: &name, Locale: &locale, Theme: &theme, Timezone: &timezone, TimezoneSet: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if user.DisplayName != name || user.Locale != locale || user.Theme != theme || user.Timezone == nil ||
		*user.Timezone != timezone || user.EffectiveTimezone != timezone {
		t.Fatal("incorrect updated preferences")
	}
	restarted, err := New(f.pool)
	if err != nil {
		t.Fatal(err)
	}
	restarted.now = f.service.now
	session, err := restarted.Authenticate(f.ctx, token)
	if err != nil || session.User.DisplayName != name || session.User.EffectiveTimezone != timezone {
		t.Fatal("preferences must persist across service recreation:", err)
	}
	for _, invalid := range []string{"", "Invalid/Timezone", "+08:00", "Europe/London'; DROP TABLE meta.users;--", "posix/UTC"} {
		_, err := f.service.UpdateProfile(f.ctx, token, ProfilePatch{TimezoneSet: true, Timezone: &invalid})
		assertCode(t, err, "ERR_VALIDATION")
	}
	invalid := "invalid"
	for _, patch := range []ProfilePatch{{}, {Locale: &invalid}, {Theme: &invalid}, {DisplayName: new("")}, {DisplayName: new("bad\nname")}} {
		_, err := f.service.UpdateProfile(f.ctx, token, patch)
		assertCode(t, err, "ERR_VALIDATION")
	}
	if _, err := f.pool.Exec(f.ctx, `INSERT INTO meta.settings(key,value) VALUES ('default_timezone','"Europe/Paris"')`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := f.pool.Exec(ctx, "DELETE FROM meta.settings WHERE key='default_timezone'"); err != nil {
			t.Error(err)
		}
	})
	user, err = f.service.UpdateProfile(f.ctx, token, ProfilePatch{TimezoneSet: true})
	if err != nil || user.Timezone != nil || user.EffectiveTimezone != "Europe/Paris" || user.Locale != locale {
		t.Fatal("NULL timezone must follow configured team default and preserve omitted fields:", err)
	}
	var null bool
	if err := f.pool.QueryRow(f.ctx, "SELECT timezone IS NULL FROM meta.users WHERE id=$1", f.id).Scan(&null); err != nil || !null {
		t.Fatal("timezone must actually be SQL NULL:", err)
	}
	for _, invalid := range []string{`"Invalid/Timezone"`, "null", "123", "{}"} {
		if _, err := f.pool.Exec(f.ctx, `UPDATE meta.settings SET value=$1::jsonb WHERE key='default_timezone'`, invalid); err != nil {
			t.Fatal(err)
		}
		if _, err := f.service.Authenticate(f.ctx, token); err == nil {
			t.Fatal("invalid stored timezone must fail explicitly")
		}
	}
	t.Log("profile persistence, PG catalog validation, NULL/default timezone and UTC storage verified; datetime fields/filters/admin UI not claimed")
}
