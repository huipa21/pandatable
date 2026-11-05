package structure

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

var bookIconNames = map[string]struct{}{
	"panelClose": {}, "panelOpen": {}, "search": {}, "sun": {}, "moon": {},
	"undo": {}, "redo": {}, "table": {}, "book": {}, "view": {},
	"chevronDown": {}, "chevronRight": {}, "plus": {}, "more": {}, "fields": {},
	"filter": {}, "group": {}, "sort": {}, "colour": {}, "height": {},
	"trash": {}, "eye": {}, "expand": {}, "fullscreen": {}, "close": {},
	"duplicate": {}, "chevronUp": {}, "comments": {}, "history": {},
	"compact": {}, "edit": {}, "calendar": {}, "clock": {}, "check": {},
	"link": {}, "text": {}, "number": {}, "formula": {}, "lookup": {},
	"rollup": {}, "attachment": {}, "mail": {}, "star": {}, "phone": {},
	"code": {}, "user": {}, "percent": {}, "currency": {},
}

func cleanBookIcon(value string) (string, error) {
	if !utf8.ValidString(value) || strings.ContainsFunc(value, unicode.IsControl) {
		return "", validation("Unknown book icon")
	}
	icon := strings.TrimSpace(value)
	if _, ok := bookIconNames[icon]; !ok {
		return "", validation("Unknown book icon")
	}
	return icon, nil
}
