package structure

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"pandatable/internal/apperr"
)

const (
	maxNameRunes = 100
	maxIconRunes = 16
)

func validation(message string) *apperr.Error {
	return apperr.New(400, "ERR_VALIDATION", message)
}

// cleanName trims and bounds a workspace/book/table display name. Display
// names are metadata only (D-002): any Unicode text without controls is valid,
// and no sibling uniqueness is imposed (D-097).
func cleanName(value string) (string, error) {
	name := strings.TrimSpace(value)
	if !utf8.ValidString(name) || name == "" || utf8.RuneCountInString(name) > maxNameRunes ||
		strings.ContainsFunc(name, unicode.IsControl) {
		return "", validation("Name must be 1 to 100 characters without control characters")
	}
	return name, nil
}

func cleanIcon(value string) (string, error) {
	icon := strings.TrimSpace(value)
	if !utf8.ValidString(icon) || icon == "" || utf8.RuneCountInString(icon) > maxIconRunes ||
		strings.ContainsFunc(icon, unicode.IsControl) {
		return "", validation("Icon must be 1 to 16 characters without control characters")
	}
	return icon, nil
}

// cleanColor accepts only #RRGGBB and stores it lowercase.
func cleanColor(value string) (string, error) {
	if len(value) != 7 || value[0] != '#' {
		return "", validation("Color must be #RRGGBB")
	}
	for _, character := range value[1:] {
		if !(character >= '0' && character <= '9' || character >= 'a' && character <= 'f' || character >= 'A' && character <= 'F') {
			return "", validation("Color must be #RRGGBB")
		}
	}
	return strings.ToLower(value), nil
}
