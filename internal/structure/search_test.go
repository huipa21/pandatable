package structure

import (
	"context"
	"errors"
	"strings"
	"testing"

	"pandatable/internal/apperr"
	"pandatable/internal/codec"
)

func TestQuickSearchFieldSelectionAndBinding(t *testing.T) {
	text, err := codec.Bind(codec.Text, nil)
	if err != nil {
		t.Fatal(err)
	}
	fields := []rowField{
		{id: "first", kind: codec.Text, column: "c_aaaaaaaaaa", codec: text},
		{id: "second", kind: codec.Text, column: "c_bbbbbbbbbb", codec: text},
		{id: "attachment", kind: codec.Attachment, column: "c_cccccccccc", codec: text},
		{id: "unavailable", kind: codec.Text, column: "c_dddddddddd"},
	}
	search := `50%_done\ok`
	selected := "second"
	sql, args, err := compileQuickSearch(context.Background(), nil, Actor{}, rowTable{}, fields, &search, &selected)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sql, "c_aaaaaaaaaa") || !strings.Contains(sql, `("source"."c_bbbbbbbbbb")::text ILIKE $1 ESCAPE '\'`) ||
		len(args) != 1 || args[0] != `%50\%\_done\\ok%` {
		t.Fatalf("field-scoped search = %q, %#v", sql, args)
	}
	sql, args, err = compileQuickSearch(context.Background(), nil, Actor{}, rowTable{}, fields, &search, nil)
	if err != nil || !strings.Contains(sql, " OR ") || len(args) != 2 {
		t.Fatalf("all-field search = %q, %#v, %v", sql, args, err)
	}
	for _, invalid := range []string{"unknown", "", "attachment", "unavailable", `second" OR TRUE --`} {
		_, _, err := compileQuickSearch(context.Background(), nil, Actor{}, rowTable{}, fields, &search, &invalid)
		var apiError *apperr.Error
		if !errors.As(err, &apiError) || apiError.Code != "ERR_VALIDATION" || apiError.Details["field"] != "searchField" {
			t.Errorf("searchField %q returned %v", invalid, err)
		}
		for _, term := range []*string{nil, new(string)} {
			sql, args, err := compileQuickSearch(context.Background(), nil, Actor{}, rowTable{}, fields, term, &invalid)
			if err != nil || sql != "" || len(args) != 0 {
				t.Errorf("inactive search with field %q = %q, %#v, %v", invalid, sql, args, err)
			}
		}
	}
}
