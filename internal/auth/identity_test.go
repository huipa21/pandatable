package auth

import (
	"context"
	"strings"
	"testing"
)

func TestSharedIdentityValidation(t *testing.T) {
	for _, value := range []string{"Alice", "  USER.Name_1-2 "} {
		got, err := NormalizeUsername(value)
		if err != nil || got != strings.ToLower(strings.TrimSpace(value)) {
			t.Fatalf("normalize %q: %q %v", value, got, err)
		}
	}
	for _, value := range []string{"ab", "名字", "a@b", strings.Repeat("x", 33), "bad\x00"} {
		_, err := NormalizeUsername(value)
		assertCode(t, err, "ERR_VALIDATION")
	}
	for _, value := range []string{"", "Alice <a@example.test>", "a@example.test\nb@example.test"} {
		_, err := NormalizeEmail(value)
		assertCode(t, err, "ERR_VALIDATION")
	}
	if ValidDisplayName("") || ValidDisplayName("bad\nname") || ValidDisplayName(strings.Repeat("名", 101)) ||
		!ValidDisplayName(strings.Repeat("名", 100)) {
		t.Fatal("display name boundaries")
	}
}

func TestSharedHashPasswordUsesExistingWorker(t *testing.T) {
	service := &Service{passwords: NewPasswords()}
	service.passwords.worker <- struct{}{}
	defer func() { <-service.passwords.worker }()
	_, err := service.HashPassword(context.Background(), "Admin-password-123")
	assertCode(t, err, "ERR_AUTH_BUSY")
	_, err = service.HashPassword(context.Background(), "short")
	assertCode(t, err, "ERR_VALIDATION")
}
