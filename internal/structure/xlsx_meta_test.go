package structure

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/acl"
	"pandatable/internal/codec"
)

func TestExportMetadataCanonicalJSONAndCellLimits(t *testing.T) {
	bound, err := codec.Bind(codec.Checkbox, nil)
	if err != nil {
		t.Fatal(err)
	}

	field := Field{
		ID: "checkbox", Name: "Done", UIType: "checkbox",
		Options: json.RawMessage(`{"icon":"star","nested":{"z":2,"a":1}}`),
		Default: json.RawMessage(`false`),
	}
	schema := TableSchema{Fields: []Field{field}}
	table := rowTable{byID: map[string]rowField{
		field.ID: {id: field.ID, kind: codec.Checkbox, codec: bound},
	}}
	build := func() (XLSXMetadata, error) {
		return buildXLSXMetadata(context.Background(), nil, pgtype.UUID{}, schema, table, "instance", time.Unix(1, 0))
	}
	metadata, err := build()
	if err != nil {
		t.Fatal(err)
	}
	if got := metadata.Fields[0].OptionsJSON; got != `{"icon":"star","nested":{"a":1,"z":2}}` {
		t.Fatalf("canonical options = %q", got)
	}
	firstHash, err := XLSXMetaSHA256(metadata.Fields)
	if err != nil || len(firstHash) != 64 {
		t.Fatalf("metadata hash = %q, %v", firstHash, err)
	}
	metadata.Fields[0].Required = true
	changedHash, err := XLSXMetaSHA256(metadata.Fields)
	if err != nil || changedHash == firstHash {
		t.Fatalf("edited metadata hash = %q, %v", changedHash, err)
	}

	const jsonOverhead = len(`{"icon":""}`)
	schema.Fields[0].Options = json.RawMessage(`{"icon":"` + strings.Repeat("a", maxSpreadsheetCellChars-jsonOverhead) + `"}`)
	if _, err := build(); err != nil {
		t.Fatalf("metadata cell at Excel's UTF-16 limit rejected: %v", err)
	}
	schema.Fields[0].Options = json.RawMessage(`{"icon":"` + strings.Repeat("a", maxSpreadsheetCellChars-jsonOverhead) + `😀"}`)
	if _, err := build(); err == nil {
		t.Fatal("oversized UTF-16 metadata cell was accepted")
	}
	if _, err := canonicalXLSXJSON(json.RawMessage(`{"x":1} {"x":2}`)); err == nil {
		t.Fatal("multiple JSON values were accepted")
	}
}

func TestExportInstanceIDPersistenceAndConcurrency(t *testing.T) {
	f := newFixture(t)
	var previous []byte
	err := f.pool.QueryRow(f.ctx, `SELECT value FROM meta.settings WHERE key='instance_id'`).Scan(&previous)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal(err)
	}
	existed := err == nil
	f.exec(`DELETE FROM meta.settings WHERE key='instance_id'`)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if existed {
			_, err = f.pool.Exec(ctx, `INSERT INTO meta.settings(key,value) VALUES ('instance_id',$1::jsonb)
				ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value`, previous)
		} else {
			_, err = f.pool.Exec(ctx, `DELETE FROM meta.settings WHERE key='instance_id'`)
		}
		if err != nil {
			t.Errorf("restore instance ID setting: %v", err)
		}
	})
	type result struct {
		id  string
		err error
	}
	const workers = 16
	results := make(chan result, workers)
	for range workers {
		go func() {
			id, err := f.service.xlsxSourceInstanceID(f.ctx)
			results <- result{id: id, err: err}
		}()
	}
	instanceID := ""
	for range workers {
		got := <-results
		if got.err != nil {
			t.Error(got.err)
			continue
		}
		if instanceID == "" {
			instanceID = got.id
		}
		if got.id != instanceID {
			t.Errorf("concurrent exports got different source IDs: %q and %q", instanceID, got.id)
		}
	}
	if t.Failed() {
		t.FailNow()
	}
	if parsed, ok := acl.ParseUUID(instanceID); !ok || parsed.String() != instanceID {
		t.Fatalf("generated instance ID is invalid: %q", instanceID)
	}
	if stored := scanOne[string](f, `SELECT value #>> '{}' FROM meta.settings WHERE key='instance_id'`); stored != instanceID {
		t.Fatalf("stored instance ID = %q, want %q", stored, instanceID)
	}
	for _, invalid := range []string{`null`, `42`, `"not-a-uuid"`} {
		f.exec(`UPDATE meta.settings SET value=$1::jsonb WHERE key='instance_id'`, invalid)
		if id, err := f.service.xlsxSourceInstanceID(f.ctx); err == nil {
			t.Errorf("invalid stored instance ID %s accepted as %q", invalid, id)
		}
	}
}
