package auth

import (
	"context"
	"net/mail"
	"strings"
	"unicode"
	"unicode/utf8"
)

func NormalizeUsername(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) < 3 || len(value) > 32 {
		return "", validation()
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z' || character >= '0' && character <= '9' ||
			character == '.' || character == '_' || character == '-') {
			return "", validation()
		}
	}
	return value, nil
}

func NormalizeEmail(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if !utf8.ValidString(value) || len(value) > 254 {
		return "", validation()
	}
	address, err := mail.ParseAddress(value)
	if err != nil || address.Address != value || strings.ContainsAny(value, "\r\n") {
		return "", validation()
	}
	return value, nil
}

func ValidDisplayName(value string) bool {
	return utf8.ValidString(value) && strings.TrimSpace(value) != "" &&
		utf8.RuneCountInString(value) <= 100 && !strings.ContainsFunc(value, unicode.IsControl)
}

func (service *Service) HashPassword(ctx context.Context, password string) (string, error) {
	if !validPassword(password) {
		return "", validation()
	}
	hash, err := service.passwords.Hash(ctx, password)
	if err != nil {
		return "", passwordError(err)
	}
	return hash, nil
}

// SessionHash validates canonical tokens before sharing their digest with
// transactional authorization checks. Raw tokens are never sent to PostgreSQL.
func SessionHash(token string) ([]byte, error) {
	hash, err := tokenHash(token)
	if err != nil {
		return nil, unauthenticated()
	}
	return hash, nil
}
