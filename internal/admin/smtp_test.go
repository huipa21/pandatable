package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestSMTPSettingsAuthorizationAndSecretStorage(t *testing.T) {
	f := newFixture(t)
	f.service.smtpKey = bytes.Repeat([]byte{7}, 32)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		f.exec(ctx, `UPDATE meta.smtp_settings SET enabled=false,host='',port=587,security='starttls',
			username='',password_cipher=NULL,from_address='' WHERE singleton`)
	})
	enabled := true
	port := 2525
	secret := "smtp-test-secret"
	username := "smtp-private-user"
	settings, err := f.service.UpdateSMTPSettings(f.ctx, f.token, SMTPPatch{
		Enabled: &enabled, Host: ptr("127.0.0.1"), Port: &port, Security: ptr("none"),
		Username: &username, Password: &secret, FromAddress: ptr("SENDER@example.test"),
	})
	if err != nil || !settings.HasUsername || !settings.HasPassword || settings.FromAddress != "sender@example.test" {
		t.Fatalf("SMTP update failed: %+v %v", settings, err)
	}
	raw, err := json.Marshal(settings)
	if err != nil || strings.Contains(string(raw), secret) || strings.Contains(string(raw), username) ||
		strings.Contains(string(raw), "password_cipher") {
		t.Fatal("SMTP update response exposed credentials")
	}
	read, err := f.service.SMTPSettings(f.ctx, f.token)
	raw, marshalErr := json.Marshal(read)
	if err != nil || marshalErr != nil || !read.HasUsername || !read.HasPassword ||
		strings.Contains(string(raw), secret) || strings.Contains(string(raw), username) {
		t.Fatal("SMTP read exposed or lost credentials")
	}
	var cipher []byte
	if err := f.pool.QueryRow(f.ctx, "SELECT password_cipher FROM meta.smtp_settings WHERE singleton").Scan(&cipher); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(cipher, []byte(secret)) {
		t.Fatal("SMTP password was stored in plaintext")
	}
	config, err := f.service.DeliverySMTP(f.ctx)
	if err != nil || config.Username != username || config.Password != secret {
		t.Fatal("SMTP credentials could not be recovered", err)
	}
	updated, err := f.service.UpdateSMTPSettings(f.ctx, f.token, SMTPPatch{Host: ptr("localhost")})
	config, deliveryErr := f.service.DeliverySMTP(f.ctx)
	if err != nil || deliveryErr != nil || !updated.HasUsername || !updated.HasPassword ||
		config.Username != username || config.Password != secret {
		t.Fatal("omitted credentials must be preserved", err, deliveryErr)
	}
	cleared, err := f.service.UpdateSMTPSettings(f.ctx, f.token, SMTPPatch{Username: ptr("")})
	if err != nil || cleared.HasUsername || !cleared.HasPassword {
		t.Fatal("SMTP username was not cleared", err)
	}
	if _, err := f.service.UpdateSMTPSettings(f.ctx, f.token, SMTPPatch{Username: &username}); err != nil {
		t.Fatal("restore SMTP username", err)
	}
	_, userToken := f.account("user", "active", false)
	_, err = f.service.SMTPSettings(f.ctx, userToken)
	code(t, err, "ERR_PERMISSION_DENIED")
	_, err = f.service.UpdateSMTPSettings(f.ctx, userToken, SMTPPatch{Enabled: &enabled})
	code(t, err, "ERR_PERMISSION_DENIED")
	_, err = f.service.UpdateSMTPSettings(f.ctx, f.token, SMTPPatch{Host: ptr("smtp.example.test")})
	code(t, err, "ERR_VALIDATION")
	f.service.smtpKey = nil
	_, err = f.service.UpdateSMTPSettings(f.ctx, f.token, SMTPPatch{Password: ptr("replacement")})
	code(t, err, "ERR_SMTP_KEY")
	if _, err := f.service.DeliverySMTP(f.ctx); err == nil {
		t.Fatal("missing key must not silently discard the stored password")
	}
}
