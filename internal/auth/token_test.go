package auth

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func TestSessionTokens(t *testing.T) {
	seen := make(map[string]bool)
	for range 1000 {
		token, hash, err := newToken()
		if err != nil {
			t.Fatal(err)
		}
		random, err := base64.RawURLEncoding.DecodeString(token)
		if err != nil || len(random) != 32 || len(token) != 43 || len(hash) != 32 || seen[token] {
			t.Fatal("unsafe or duplicate token")
		}
		seen[token] = true
		expected := sha256.Sum256([]byte(token))
		actual, err := tokenHash(token)
		if err != nil || !bytes.Equal(expected[:], hash) || !bytes.Equal(hash, actual) || bytes.Equal(hash, random) {
			t.Fatal("only the SHA-256 token digest may identify a session")
		}
	}
	for _, token := range []string{"", "short", strings.Repeat("a", 10000), strings.Repeat("A", 42) + "B",
		strings.Repeat("/", 43), strings.Repeat("A", 42) + "=", strings.Repeat("A", 42) + "\n"} {
		if _, err := tokenHash(token); !errors.Is(err, ErrToken) {
			t.Fatal("invalid token accepted")
		}
	}
}
