package codec

import (
	"bytes"
	"encoding/json"
	"io"
	"regexp"
	"unicode"
	"unicode/utf8"
)

// optionSet is a strict JSON object: duplicate keys are rejected and every key
// must be consumed by the codec (unknown keys are rejected by done()).
type optionSet struct {
	values map[string]json.RawMessage
	used   map[string]bool
}

func parseOptions(raw json.RawMessage) (*optionSet, error) {
	set := &optionSet{values: map[string]json.RawMessage{}, used: map[string]bool{}}
	trimmed := trimJSONSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return set, nil
	}
	if trimmed[0] != '{' {
		return nil, failOption("")
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	if _, err := decoder.Token(); err != nil {
		return nil, failOption("")
	}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, failOption("")
		}
		key, ok := token.(string)
		if !ok {
			return nil, failOption("")
		}
		if _, duplicate := set.values[key]; duplicate {
			return nil, failOption(key)
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, failOption(key)
		}
		set.values[key] = value
	}
	if _, err := decoder.Token(); err != nil {
		return nil, failOption("")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, failOption("")
	}
	return set, nil
}

func (set *optionSet) take(key string) (json.RawMessage, bool) {
	value, ok := set.values[key]
	if ok {
		set.used[key] = true
	}
	return value, ok
}

func (set *optionSet) done() error {
	for key := range set.values {
		if !set.used[key] {
			return failOption(key)
		}
	}
	return nil
}

func (set *optionSet) enum(key, fallback string, allowed ...string) (string, error) {
	raw, ok := set.take(key)
	if !ok {
		return fallback, nil
	}
	var value string
	if !jsonIs(raw, '"') || json.Unmarshal(raw, &value) != nil {
		return "", failOption(key)
	}
	for _, candidate := range allowed {
		if value == candidate {
			return value, nil
		}
	}
	return "", failOption(key)
}

func (set *optionSet) boolean(key string, fallback bool) (bool, error) {
	raw, ok := set.take(key)
	if !ok {
		return fallback, nil
	}
	switch string(trimJSONSpace(raw)) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	return false, failOption(key)
}

var smallIntPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]{0,8})$`)

// integer reads an optional bounded integer; present=false when absent.
func (set *optionSet) integer(key string, low, high int) (value int, present bool, err error) {
	raw, ok := set.take(key)
	if !ok {
		return 0, false, nil
	}
	text := string(trimJSONSpace(raw))
	if !smallIntPattern.MatchString(text) {
		return 0, false, failOption(key)
	}
	var parsed int
	if json.Unmarshal([]byte(text), &parsed) != nil || parsed < low || parsed > high {
		return 0, false, failOption(key)
	}
	return parsed, true, nil
}

func jsonIs(raw json.RawMessage, first byte) bool {
	trimmed := trimJSONSpace(raw)
	return len(trimmed) > 0 && trimmed[0] == first
}

// isTrimSpace is the shared whitespace set trimmed around email/url/number/
// choice input: Unicode White_Space (Go unicode.IsSpace) plus U+FEFF.
func isTrimSpace(r rune) bool { return unicode.IsSpace(r) || r == '\uFEFF' }

// isControl reports C0/C1 control characters and DEL.
func isControl(r rune) bool { return r < 0x20 || (r >= 0x7F && r <= 0x9F) }

// validString enforces valid UTF-8, no NUL and no control characters except
// the explicitly allowed ones.
func validString(text string, allowTab, allowLF bool) bool {
	if !utf8.ValidString(text) {
		return false
	}
	for _, r := range text {
		if !isControl(r) || (r == '\t' && allowTab) || (r == '\n' && allowLF) {
			continue
		}
		return false
	}
	return true
}
