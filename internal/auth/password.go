package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

var (
	ErrPassword = errors.New("password must contain 8-32 UTF-8 bytes")
	ErrPHC      = errors.New("invalid or unsupported password hash")
	ErrBusy     = errors.New("password worker busy")
)

type Passwords struct {
	worker chan struct{}
}

func NewPasswords() *Passwords { return &Passwords{worker: make(chan struct{}, 1)} }

func validPassword(password string) bool {
	return len(password) >= 8 && len(password) <= 32 && utf8.ValidString(password)
}

func (passwords *Passwords) derive(ctx context.Context, password string, salt []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case passwords.worker <- struct{}{}:
		defer func() { <-passwords.worker }()
	default:
		return nil, ErrBusy
	}
	key := argon2.IDKey([]byte(password), salt, 1, 64*1024, 2, 32)
	if err := ctx.Err(); err != nil {
		clear(key)
		return nil, err
	}
	return key, nil
}

func (passwords *Passwords) Hash(ctx context.Context, password string) (string, error) {
	if !validPassword(password) {
		return "", ErrPassword
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	key, err := passwords.derive(ctx, password, salt)
	if err != nil {
		return "", err
	}
	defer clear(key)
	return "$argon2id$v=19$m=65536,t=1,p=2$" +
		base64.RawStdEncoding.EncodeToString(salt) + "$" +
		base64.RawStdEncoding.EncodeToString(key), nil
}

func parsePHC(phc string) ([]byte, []byte, error) {
	if len(phc) > 256 {
		return nil, nil, ErrPHC
	}
	parts := strings.Split(phc, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" ||
		parts[2] != "v=19" || parts[3] != "m=65536,t=1,p=2" {
		return nil, nil, ErrPHC
	}
	salt, saltErr := base64.RawStdEncoding.Strict().DecodeString(parts[4])
	key, keyErr := base64.RawStdEncoding.Strict().DecodeString(parts[5])
	if saltErr != nil || keyErr != nil || len(salt) != 16 || len(key) != 32 ||
		base64.RawStdEncoding.EncodeToString(salt) != parts[4] ||
		base64.RawStdEncoding.EncodeToString(key) != parts[5] {
		return nil, nil, ErrPHC
	}
	return salt, key, nil
}

func (passwords *Passwords) Verify(ctx context.Context, password, phc string) (bool, error) {
	salt, expected, err := parsePHC(phc)
	if err != nil {
		return false, err
	}
	defer clear(expected)
	if !validPassword(password) {
		return false, ErrPassword
	}
	actual, err := passwords.derive(ctx, password, salt)
	if err != nil {
		return false, err
	}
	defer clear(actual)
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}
