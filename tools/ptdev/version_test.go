package main

import (
	"errors"
	"testing"
)

func TestFormatVersion(t *testing.T) {
	for _, test := range []struct {
		description, want string
		err               error
	}{
		{"v1.2.3", "1.2.3", nil},
		{"v1.2.3-dirty", "1.2.3-dirty", nil},
		{"v1.2.3-4-gabc1234", "1.2.3-4-gabc1234", nil},
		{"v1.2.3-4-gabc1234-dirty", "1.2.3-4-gabc1234-dirty", nil},
		{"abc1234", "0.0.0-dev+abc1234", nil},
		{"abc1234-dirty\n", "0.0.0-dev+abc1234.dirty", nil},
		{"1.2.3", "1.2.3", nil},
		{"", "0.0.0-dev", nil},
		{"", "0.0.0-dev", errors.New("git not available")},
		{"fatal: no commits", "0.0.0-dev", errors.New("git describe failed")},
	} {
		if got := formatVersion(test.description, test.err); got != test.want {
			t.Errorf("formatVersion(%q, %v) = %q, want %q", test.description, test.err, got, test.want)
		}
	}
}
