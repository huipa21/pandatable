package structure

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/apperr"
	"pandatable/internal/ddl"
)

func uuidOf(value byte) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{15: value}, Valid: true}
}

func siblings(positions ...float64) []sibling {
	items := make([]sibling, len(positions))
	for index, position := range positions {
		items[index] = sibling{ID: uuidOf(byte(index + 1)), Position: position}
	}
	return items
}

func TestPlaceChoosesStableBoundsAndRenumbersExhaustedGaps(t *testing.T) {
	three := siblings(1024, 2048, 3072)
	for _, test := range []struct {
		name      string
		moving    byte
		index     int
		unchanged bool
		position  float64
		renumber  bool
	}{
		{"to front", 3, 0, false, 1024 - 1024, false},
		{"to end", 1, 2, false, 3072 + 1024, false},
		{"between", 1, 1, false, (2048 + 3072) / 2.0, false},
		{"same index", 2, 1, true, 0, false},
		{"front stays", 1, 0, true, 0, false},
	} {
		moved, err := place(three, uuidOf(test.moving), test.index)
		if err != nil || moved.Unchanged != test.unchanged || (!test.unchanged && moved.Position != test.position) || (moved.Renumber != nil) != test.renumber {
			t.Fatalf("%s: %+v %v", test.name, moved, err)
		}
	}
	// A single sibling can only stay put.
	if moved, err := place(siblings(5), uuidOf(1), 0); err != nil || !moved.Unchanged {
		t.Fatalf("single sibling: %+v %v", moved, err)
	}
	for _, index := range []int{-1, 3, 100} {
		if _, err := place(three, uuidOf(1), index); err == nil {
			t.Fatalf("index %d must be rejected", index)
		} else {
			var app *apperr.Error
			if !errors.As(err, &app) || app.Code != "ERR_VALIDATION" {
				t.Fatalf("index %d: %v", index, err)
			}
		}
	}
	if _, err := place(three, uuidOf(9), 0); err == nil {
		t.Fatal("a moving item outside its live siblings is an internal inconsistency")
	}

	// Gaps below the threshold or identical positions renumber every live sibling in order.
	for _, positions := range [][]float64{{1, 1 + 1e-9, 9}, {7, 7, 7}} {
		moved, err := place(siblings(positions...), uuidOf(3), 1)
		if err != nil || moved.Renumber == nil {
			t.Fatalf("%v: %+v %v", positions, moved, err)
		}
		want := []pgtype.UUID{uuidOf(1), uuidOf(3), uuidOf(2)}
		for index := range want {
			if moved.Renumber[index] != want[index] {
				t.Fatalf("%v: renumber order %v", positions, moved.Renumber)
			}
		}
	}
	// Repeated midpoints eventually hit the threshold instead of losing precision.
	items := siblings(0, 1)
	for range 80 {
		moved, err := place([]sibling{items[0], items[1], {ID: uuidOf(3), Position: 100}}, uuidOf(3), 1)
		if err != nil {
			t.Fatal(err)
		}
		if moved.Renumber != nil {
			return
		}
		if moved.Position <= items[0].Position || moved.Position >= items[1].Position {
			t.Fatalf("midpoint %v escaped (%v,%v)", moved.Position, items[0].Position, items[1].Position)
		}
		items[1].Position = moved.Position
	}
	t.Fatal("bisecting 80 times must trigger a renumber")
}

func TestNameIconAndColorValidation(t *testing.T) {
	for _, valid := range []string{"a", "客户 订单", " trimmed ", strings.Repeat("😀", 100), "quote ' \" ; --"} {
		if _, err := cleanName(valid); err != nil {
			t.Fatalf("%q rejected: %v", valid, err)
		}
	}
	if got, _ := cleanName("  x  "); got != "x" {
		t.Fatal("names are trimmed")
	}
	for _, invalid := range []string{"", "   ", strings.Repeat("a", 101), "a\x00b", "tab\tname", "line\nbreak", "\x7f", "bad\xffutf8"} {
		if _, err := cleanName(invalid); err == nil {
			t.Fatalf("%q accepted", invalid)
		}
	}
	for _, valid := range []string{"#000000", "#ABCDEF", "#a1b2c3"} {
		if got, err := cleanColor(valid); err != nil || got != strings.ToLower(valid) {
			t.Fatalf("%s: %s %v", valid, got, err)
		}
	}
	for _, invalid := range []string{"", "#fff", "000000", "#00000g", "#0000000", "red", "#٠٠٠٠٠٠"} {
		if _, err := cleanColor(invalid); err == nil {
			t.Fatalf("%q accepted", invalid)
		}
	}
	if _, err := cleanIcon("🐼"); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{"", " ", strings.Repeat("a", 17), "a\nb"} {
		if _, err := cleanIcon(invalid); err == nil {
			t.Fatalf("icon %q accepted", invalid)
		}
	}
}

func TestGeneratedCollisionClassificationIsExact(t *testing.T) {
	pg := func(code, constraint string) error {
		return fmt.Errorf("insert: %w", &pgconn.PgError{Code: code, ConstraintName: constraint})
	}
	for _, collision := range []error{
		errors.Join(ddl.ErrIdentifierCollision, apperr.New(409, "ERR_DDL_CONFLICT", "x")),
		pg("23505", "books_pg_schema_key"),
		pg("23505", "tables_book_id_pg_name_key"),
	} {
		if !generatedCollision(collision) {
			t.Fatalf("%v must retry", collision)
		}
	}
	for _, other := range []error{
		nil,
		errors.New("plain"),
		pg("23505", "fields_table_id_lower_name_idx"),
		pg("23505", "workspaces_pkey"),
		pg("23503", "books_pg_schema_key"),
		pg("55P03", ""),
		apperr.New(409, "ERR_DDL_CONFLICT", "existing data"),
		apperr.New(409, "ERR_DDL_LOCK_TIMEOUT", "busy"),
	} {
		if generatedCollision(other) {
			t.Fatalf("%v must not be retried", other)
		}
	}
}

func TestRetryCollisionsBound(t *testing.T) {
	calls := 0
	err := retryCollisions(func() error { calls++; return ddl.ErrIdentifierCollision })
	var app *apperr.Error
	if !errors.As(err, &app) || app.Status != 409 || app.Code != "ERR_DDL_CONFLICT" || calls != 3 {
		t.Fatalf("exhausted: %v after %d calls", err, calls)
	}
	calls = 0
	if err := retryCollisions(func() error {
		calls++
		if calls < 3 {
			return ddl.ErrIdentifierCollision
		}
		return nil
	}); err != nil || calls != 3 {
		t.Fatalf("third attempt succeeds: %v %d", err, calls)
	}
	boom := errors.New("boom")
	calls = 0
	if err := retryCollisions(func() error { calls++; return boom }); !errors.Is(err, boom) || calls != 1 {
		t.Fatalf("other errors are returned immediately: %v %d", err, calls)
	}
}

func TestSanitizeLockMapsOnlyBareLockTimeouts(t *testing.T) {
	err := sanitizeLock(fmt.Errorf("lock: %w", &pgconn.PgError{Code: "55P03", Message: "secret detail"}))
	var app *apperr.Error
	if !errors.As(err, &app) || app.Code != "ERR_DDL_LOCK_TIMEOUT" || strings.Contains(app.Message, "secret") {
		t.Fatalf("%v", err)
	}
	original := apperr.New(404, "ERR_NOT_FOUND", "x")
	if sanitizeLock(original) != original || sanitizeLock(nil) != nil {
		t.Fatal("other errors pass through")
	}
}

func TestDefaultLabelsAndGridConfig(t *testing.T) {
	for locale, labels := range labelSets {
		for _, text := range []string{labels.name, labels.rowID, labels.createdAt, labels.createdBy, labels.updatedAt, labels.updatedBy, labels.grid} {
			if strings.TrimSpace(text) == "" {
				t.Fatalf("%s has an empty label", locale)
			}
		}
		seen := map[string]bool{strings.ToLower(labels.name): true}
		for _, field := range systemFields {
			key := strings.ToLower(field.label(labels))
			if seen[key] {
				t.Fatalf("%s default field names must be case-insensitively unique: %s", locale, key)
			}
			seen[key] = true
		}
	}
	if labelsFor("fr") != labelSets["en"] || labelsFor("zh-CN") != labelSets["zh-CN"] {
		t.Fatal("unknown locales fall back to English")
	}
	columns := map[string]string{}
	for _, field := range systemFields {
		columns[field.column] = field.pgType
		if err := ddl.ValidateIdentifier(ddl.Column, field.column); err == nil {
			t.Fatalf("system column %s must not look like a generated c_ name", field.column)
		}
	}
	if len(columns) != 5 || columns["id"] != "bigint" || columns["created_at"] != "timestamptz" || columns["updated_by"] != "uuid" {
		t.Fatalf("system columns = %v", columns)
	}
	raw, err := defaultGridConfig("n", []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil || decoded["frozen"].(float64) != 1 || len(decoded["fields"].([]any)) != 3 {
		t.Fatalf("%s %v", raw, err)
	}
}

func TestServiceRequiresDependencies(t *testing.T) {
	if _, err := New(nil, nil); err == nil {
		t.Fatal("nil dependencies must be rejected")
	}
}

func TestResourceOfTreatsMalformedIDsAsMissing(t *testing.T) {
	for _, id := range []string{"", "x", "00000000-0000-0000-0000-00000000000g", "{00000000-0000-4000-8000-000000000000}"} {
		_, err := resourceOf("table", id)
		var app *apperr.Error
		if !errors.As(err, &app) || app.Status != 404 {
			t.Fatalf("%q: %v", id, err)
		}
	}
}
