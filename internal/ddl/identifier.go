// Package ddl performs physical schema changes in a caller-owned transaction.
// Services own ACL checks, metadata, schema versions, events and transaction completion.
package ddl

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"io"
	"strings"
)

type ObjectKind string

const (
	Book     ObjectKind = "b"
	Table    ObjectKind = "t"
	Column   ObjectKind = "c"
	Junction ObjectKind = "j"
	Sequence ObjectKind = "s"
)

const alphabet = "abcdefghijklmnopqrstuvwxyz234567"

func NewIdentifier(kind ObjectKind) (string, error) {
	return newIdentifier(kind, rand.Reader)
}

func newIdentifier(kind ObjectKind, random io.Reader) (string, error) {
	if !validKind(kind) {
		return "", validation("Invalid physical object kind")
	}
	var entropy [7]byte
	if _, err := io.ReadFull(random, entropy[:]); err != nil {
		return "", fmt.Errorf("generate physical identifier: %w", err)
	}
	// Ten complete base32 symbols use the first 50 random bits, without bias.
	suffix := strings.ToLower(base32.StdEncoding.EncodeToString(entropy[:])[:10])
	return string(kind) + "_" + suffix, nil
}

func ValidateIdentifier(kind ObjectKind, name string) error {
	if !validKind(kind) || len(name) != 12 || name[:2] != string(kind)+"_" {
		return validation("Invalid physical identifier")
	}
	for _, value := range name[2:] {
		if !strings.ContainsRune(alphabet, value) {
			return validation("Invalid physical identifier")
		}
	}
	return nil
}

func validKind(kind ObjectKind) bool {
	switch kind {
	case Book, Table, Column, Junction, Sequence:
		return true
	default:
		return false
	}
}
