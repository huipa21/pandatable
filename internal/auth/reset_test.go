package auth

import (
	"bytes"
	"net/netip"
	"testing"
	"time"
)

func TestPasswordResetLifecyclePostgreSQL(t *testing.T) {
	f := newFixture(t)
	_, session := f.login(t, f.username)
	message, found, err := f.service.IssuePasswordReset(f.ctx, "missing@example.test")
	if err != nil || found || message.Token != "" {
		t.Fatal("unknown email must not create a reset", err)
	}
	message, found, err = f.service.IssuePasswordReset(f.ctx, f.email)
	if err != nil || !found || message.Email != f.email || message.Token == "" {
		t.Fatal("reset was not issued", err)
	}
	var stored []byte
	var expires time.Time
	if err := f.pool.QueryRow(f.ctx, "SELECT token_hash,expires_at FROM meta.password_resets WHERE user_id=$1", f.id).
		Scan(&stored, &expires); err != nil {
		t.Fatal(err)
	}
	hash, _ := tokenHash(message.Token)
	if !bytes.Equal(stored, hash) || bytes.Contains(stored, []byte(message.Token)) ||
		!expires.Equal(f.now.Add(ResetLifetime)) {
		t.Fatal("reset token must be stored as a digest with a bounded lifetime")
	}
	assertCode(t, f.service.CompletePasswordReset(f.ctx, message.Token, "short"), "ERR_VALIDATION")
	if err := f.service.CompletePasswordReset(f.ctx, message.Token, "Replacement-password-123"); err != nil {
		t.Fatal(err)
	}
	f.sessionCount(t, 0)
	_, err = f.service.Authenticate(f.ctx, session)
	assertCode(t, err, "ERR_UNAUTHENTICATED")
	assertCode(t, f.service.CompletePasswordReset(f.ctx, message.Token, "Replacement-password-123"), "ERR_RESET_TOKEN")
	_, _, err = f.service.Login(f.ctx, f.username, "Replacement-password-123", netip.MustParseAddr("192.0.2.2"), "")
	if err != nil {
		t.Fatal("new password must authenticate", err)
	}

	expired, found, err := f.service.IssuePasswordReset(f.ctx, f.email)
	if err != nil || !found {
		t.Fatal(err)
	}
	f.now = f.now.Add(ResetLifetime)
	assertCode(t, f.service.CompletePasswordReset(f.ctx, expired.Token, "Another-password-123"), "ERR_RESET_TOKEN")
}

func TestPasswordResetInvalidatedByAccountChanges(t *testing.T) {
	f := newFixture(t)
	for _, update := range []struct {
		name  string
		query string
	}{
		{"email", "UPDATE meta.users SET email='changed@example.test' WHERE id=$1"},
		{"status", "UPDATE meta.users SET status='disabled' WHERE id=$1"},
		{"password", "UPDATE meta.users SET password_hash=password_hash || 'x' WHERE id=$1"},
	} {
		t.Run(update.name, func(t *testing.T) {
			message, found, err := f.service.IssuePasswordReset(f.ctx, f.email)
			if err != nil || !found {
				t.Fatal(err)
			}
			if _, err := f.pool.Exec(f.ctx, update.query, f.id); err != nil {
				t.Fatal(err)
			}
			if _, err := f.pool.Exec(f.ctx, "UPDATE meta.users SET email=$2,status='active' WHERE id=$1", f.id, f.email); err != nil {
				t.Fatal(err)
			}
			assertCode(t, f.service.CompletePasswordReset(f.ctx, message.Token, "Replacement-password-123"), "ERR_RESET_TOKEN")
		})
	}
}

func TestResetRateLimits(t *testing.T) {
	service := &Service{resetLimits: newLimits(), now: time.Now}
	ip := netip.MustParseAddr("192.0.2.4")
	for range 3 {
		if err := service.CheckResetEmail("person@example.test"); err != nil {
			t.Fatal(err)
		}
	}
	assertCode(t, service.CheckResetEmail("person@example.test"), "ERR_RATE_LIMITED")
	if err := service.CheckResetEmail("other@example.test"); err != nil {
		t.Fatal(err)
	}
	for range 10 {
		if err := service.CheckResetIP(ip); err != nil {
			t.Fatal(err)
		}
	}
	assertCode(t, service.CheckResetIP(ip), "ERR_RATE_LIMITED")
}

func TestMentionEmailPreferencePostgreSQL(t *testing.T) {
	f := newFixture(t)
	session, token := f.login(t, f.email)
	if session.User.MentionEmail {
		t.Fatal("mention email must default to off")
	}
	enabled := true
	user, err := f.service.UpdateProfile(f.ctx, token, ProfilePatch{MentionEmail: &enabled})
	if err != nil || !user.MentionEmail {
		t.Fatal("opt-in was not persisted", err)
	}
	session, err = f.service.Authenticate(f.ctx, token)
	if err != nil || !session.User.MentionEmail {
		t.Fatal("opt-in was not returned in the profile", err)
	}
}
