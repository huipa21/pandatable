package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestPasswords(t *testing.T) {
	passwords := NewPasswords()
	ctx := context.Background()
	first, err := passwords.Hash(ctx, "  password  ")
	if err != nil {
		t.Fatal(err)
	}
	second, err := passwords.Hash(ctx, "  password  ")
	if err != nil {
		t.Fatal(err)
	}
	if first == second || !strings.HasPrefix(first, "$argon2id$v=19$m=65536,t=1,p=2$") {
		t.Fatal("hashes must use independent salts and exact argon2id parameters")
	}
	for _, test := range []struct {
		password string
		match    bool
	}{{"  password  ", true}, {"password", false}, {"wrong-password", false}} {
		match, err := passwords.Verify(ctx, test.password, first)
		if err != nil || match != test.match {
			t.Errorf("verify: match=%t err=%v", match, err)
		}
	}
	for _, value := range []string{"12345678", strings.Repeat("a", 32), "密码测试"} {
		if !validPassword(value) {
			t.Errorf("valid UTF-8 password length %d rejected", len(value))
		}
		phc, err := passwords.Hash(ctx, value)
		if err != nil {
			t.Fatal("valid boundary password could not be hashed:", err)
		}
		if match, err := passwords.Verify(ctx, value, phc); err != nil || !match {
			t.Fatal("valid boundary password did not round-trip:", err)
		}
	}
	for _, value := range []string{"", "1234567", strings.Repeat("a", 33), strings.Repeat("中", 11), "12345678\xff"} {
		if _, err := passwords.Hash(ctx, value); !errors.Is(err, ErrPassword) {
			t.Errorf("length %d: want password error, got %v", len(value), err)
		}
	}
	if _, err := passwords.Verify(ctx, "short", first); !errors.Is(err, ErrPassword) {
		t.Fatalf("invalid verification password: %v", err)
	}
}

func TestPHCBoundsAndWorker(t *testing.T) {
	passwords := NewPasswords()
	hash, err := passwords.Hash(context.Background(), "password")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(hash, "$")
	for _, invalid := range []string{
		"", strings.Repeat("x", 257), hash + "$", hash[:len(hash)-1],
		strings.Replace(hash, "argon2id", "argon2i", 1),
		strings.Replace(hash, "v=19", "v=16", 1),
		strings.Replace(hash, "m=65536", "m=0", 1),
		strings.Replace(hash, "m=65536", "m=4294967295", 1),
		strings.Replace(hash, "t=1", "t=1000000", 1),
		strings.Replace(hash, "p=2", "p=255", 1),
		strings.Replace(hash, "p=2", "p=0", 1),
		strings.Replace(hash, "m=65536,t=1,p=2", "t=1,m=65536,p=2", 1),
		strings.Replace(hash, parts[4], parts[4]+"=", 1),
		strings.Replace(hash, parts[4], parts[4]+"\n", 1),
		strings.Replace(hash, parts[5], strings.Repeat("A", 42)+"B", 1),
	} {
		if _, _, err := parsePHC(invalid); !errors.Is(err, ErrPHC) {
			t.Errorf("invalid PHC accepted (length %d)", len(invalid))
		}
	}
	passwords.worker <- struct{}{}
	if _, err := passwords.Hash(context.Background(), "password"); !errors.Is(err, ErrBusy) {
		t.Errorf("hash should fail fast when worker occupied: %v", err)
	}
	if _, err := passwords.Verify(context.Background(), "password", hash); !errors.Is(err, ErrBusy) {
		t.Errorf("verify should share worker bound: %v", err)
	}
	if _, err := passwords.Verify(context.Background(), "password", "invalid"); !errors.Is(err, ErrPHC) {
		t.Errorf("PHC must be bounded before expensive work: %v", err)
	}
	<-passwords.worker
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := passwords.Hash(ctx, "password"); !errors.Is(err, context.Canceled) {
		t.Errorf("cancellation: %v", err)
	}
}

func FuzzParsePHC(f *testing.F) {
	f.Add("")
	f.Add("$argon2id$v=19$m=65536,t=1,p=2$invalid$invalid")
	f.Fuzz(func(t *testing.T, phc string) {
		salt, key, err := parsePHC(phc)
		if err == nil && (len(salt) != 16 || len(key) != 32) {
			t.Fatal("accepted unbounded PHC")
		}
	})
}

func BenchmarkPasswordHash(b *testing.B) {
	passwords := NewPasswords()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := passwords.Hash(context.Background(), "benchmark-password"); err != nil {
			b.Fatal(err)
		}
	}
}
