package structure

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"pandatable/internal/codec"
)

func TestSharedViewIndexNeedsSelectsSupportedPersistentUses(t *testing.T) {
	fields := map[string]autoIndexField{
		"text":     {id: "text", uiType: codec.Text, column: "c_text", pgType: "text"},
		"hidden":   {id: "hidden", uiType: codec.Text, column: "c_hidden", pgType: "text"},
		"number":   {id: "number", uiType: codec.Integer, column: "c_number", pgType: "int8"},
		"decimal":  {id: "decimal", uiType: codec.Decimal, column: "c_decimal", pgType: "numeric"},
		"array":    {id: "array", uiType: codec.MultiSelect, column: "c_array", pgType: "text[]"},
		"computed": {id: "computed", uiType: codec.Kind("formula"), pgType: "text"},
		"system":   {id: "system", uiType: codec.CreatedAt, column: "created_at", pgType: "timestamptz", system: true},
	}
	config := []byte(`{
		"fields":[{"id":"text","visible":true},{"id":"hidden","visible":false}],
		"sort":[{"field":"number","dir":"asc"}],
		"groupBy":["array"],
		"filter":{"op":"and","items":[
			{"field":"decimal","cmp":"gt","value":10},
			{"field":"text","cmp":"contains","value":"Acme"}
		]}
	}`)
	got, err := sharedViewIndexNeeds(config, fields)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]autoIndexNeed{
		"text":    {trigram: true},
		"number":  {btree: true},
		"decimal": {btree: true},
		"array":   {btree: true},
	}
	if len(got) != len(want) {
		t.Fatalf("index needs = %#v, want %#v", got, want)
	}
	for id, expected := range want {
		if got[id] != expected {
			t.Errorf("index need for %s = %+v, want %+v", id, got[id], expected)
		}
	}
}

func TestAutomaticIndexDDLUsesConcurrentQuotedDefinitions(t *testing.T) {
	field := autoIndexField{
		id: "11111111-1111-4111-8111-111111111111", schema: "b_aaaaaaaaaa",
		table: "t_bbbbbbbbbb", column: "c_cccccccccc",
	}
	for _, kind := range []automaticIndexKind{automaticBtree, automaticTrigram} {
		name, err := automaticIndexName(field.id, kind)
		if err != nil {
			t.Fatal(err)
		}
		statement, err := automaticIndexDDL(field, name, kind)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(statement, "CREATE INDEX CONCURRENTLY IF NOT EXISTS") {
			t.Errorf("%s DDL is not concurrent: %s", kind, statement)
		}
		if strings.Contains(statement, "BEGIN") || !strings.Contains(statement, `"b_aaaaaaaaaa"."`) {
			t.Errorf("%s DDL has unsafe transaction/schema shape: %s", kind, statement)
		}
		if kind == automaticBtree && !strings.Contains(statement, `("c_cccccccccc")`) {
			t.Errorf("btree DDL does not index the quoted field: %s", statement)
		}
		if kind == automaticTrigram && !strings.Contains(statement, `USING gin ("c_cccccccccc" gin_trgm_ops)`) {
			t.Errorf("trigram DDL does not use GIN trigram ops: %s", statement)
		}
	}
	if _, err := automaticIndexName("not-a-uuid", automaticBtree); err == nil {
		t.Fatal("invalid field id accepted")
	}
}

func TestAutomaticIndexesRequireMoreThanTenThousandRowsAndRetryMetadata(t *testing.T) {
	f := newFixture(t)
	owner := f.user("user", "active", "en")
	table := f.fieldTable(owner)
	label := table.create(FieldInput{Name: "Label", UIType: "text"})
	amount := table.create(FieldInput{Name: "Amount", UIType: "integer", Options: json.RawMessage(`{"size":"int"}`)})
	hidden := table.create(FieldInput{Name: "Hidden", UIType: "text"})

	view := table.schema.Views[0]
	updated, err := f.service.UpdateViewColumns(f.ctx, owner, view.ID, view.Version, ViewColumns{
		Fields: []ViewColumn{
			{ID: label.ID, Visible: true},
			{ID: amount.ID, Visible: true},
			{ID: hidden.ID, Visible: false},
		},
		Sort:      []ViewSort{{Field: amount.ID, Dir: "asc"}},
		GroupBy:   []string{amount.ID},
		Filter:    json.RawMessage(fmt.Sprintf(`{"field":%q,"cmp":"gt","value":0}`, amount.ID)),
		FilterSet: true,
	})
	if err != nil {
		t.Fatalf("save shared view config: %v", err)
	}
	if updated.Scope != "shared" {
		t.Fatalf("fixture view scope = %q, want shared", updated.Scope)
	}

	personal, err := f.service.CreateView(f.ctx, owner, table.id(), ViewInput{
		Name: "Personal search", Type: "grid", Scope: "personal",
	})
	if err != nil || personal.Scope != "personal" {
		t.Fatalf("create personal view = %+v, %v", personal, err)
	}
	physical, err := physicalTable(table.pgSchema, table.pgName)
	if err != nil {
		t.Fatal(err)
	}
	labelColumn := pgx.Identifier{table.column(label)}.Sanitize()
	if _, err := f.pool.Exec(f.ctx, fmt.Sprintf(
		"INSERT INTO %s (%s) SELECT 'row-' || value::text FROM generate_series(1, 10000) AS value",
		physical, labelColumn)); err != nil {
		t.Fatal(err)
	}
	if err := f.service.scanAutoIndexes(f.ctx); err != nil {
		t.Fatalf("scan at threshold: %v", err)
	}
	if exists := f.scanIndexExists(table, label.ID, automaticTrigram); exists {
		t.Fatal("created trigram index at exactly 10,000 rows")
	}

	if _, err := f.pool.Exec(f.ctx, fmt.Sprintf("INSERT INTO %s (%s) VALUES ('row-10001')", physical, labelColumn)); err != nil {
		t.Fatal(err)
	}
	if err := f.service.scanAutoIndexes(f.ctx); err != nil {
		t.Fatalf("scan above threshold: %v", err)
	}
	f.requireIndex(t, table, label, automaticTrigram)
	f.requireIndex(t, table, amount, automaticBtree)
	if f.scanIndexExists(table, label.ID, automaticBtree) {
		t.Fatal("created btree index for text quick-search only")
	}
	if f.scanIndexExists(table, hidden.ID, automaticTrigram) {
		t.Fatal("created trigram index for hidden field or personal-view-only use")
	}
	var options map[string]json.RawMessage
	if err := f.pool.QueryRow(f.ctx, "SELECT options FROM meta.fields WHERE id = $1", label.ID).Scan(&options); err != nil {
		t.Fatal(err)
	}
	var indexed map[string]bool
	if err := json.Unmarshal(options["indexed"], &indexed); err != nil {
		t.Fatalf("decode indexed metadata %s: %v", options["indexed"], err)
	}
	if !indexed["trigram"] {
		t.Fatalf("indexed metadata did not record successful trigram index: %+v", options)
	}
	if err := f.pool.QueryRow(f.ctx, "SELECT options FROM meta.fields WHERE id = $1", amount.ID).Scan(&options); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(options["indexed"], &indexed); err != nil || !indexed["btree"] ||
		string(options["size"]) != `"int"` {
		t.Fatalf("indexed metadata did not preserve the unrelated size option: %+v (%v)", options, err)
	}
	updatedLabel, err := table.update(label, FieldPatch{Options: json.RawMessage(`{}`)})
	if err != nil || string(updatedLabel.Options) != "{}" {
		t.Fatalf("update options with internal index metadata = %s, %v", updatedLabel.Options, err)
	}
	search := "row-10001"
	rows, err := f.service.QueryRows(f.ctx, owner, view.ID, RowQueryRequest{Search: &search})
	if err != nil || len(rows.Rows) != 1 {
		t.Fatalf("query indexed field after metadata update returned %d rows: %v", len(rows.Rows), err)
	}

	serviceAfterRestart, err := New(f.pool, f.resolver)
	if err != nil {
		t.Fatal(err)
	}
	if err := serviceAfterRestart.scanAutoIndexes(f.ctx); err != nil {
		t.Fatalf("repeated scan: %v", err)
	}
	f.requireIndex(t, table, label, automaticTrigram)

	f.exec(`UPDATE meta.fields SET options = '{"indexed":false}'::jsonb WHERE id = $1`, label.ID)
	if err := serviceAfterRestart.scanAutoIndexes(f.ctx); err == nil {
		t.Fatal("malformed indexed metadata did not surface a retryable failure")
	}
	f.exec(`UPDATE meta.fields SET options = '{}'::jsonb WHERE id = $1`, label.ID)
	if err := serviceAfterRestart.scanAutoIndexes(f.ctx); err != nil {
		t.Fatalf("retry after metadata failure: %v", err)
	}
	f.requireIndex(t, table, label, automaticTrigram)
	if err := f.pool.QueryRow(f.ctx, "SELECT options FROM meta.fields WHERE id = $1", label.ID).Scan(&options); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(options["indexed"], &indexed); err != nil || !indexed["trigram"] {
		t.Fatalf("retry did not restore indexed metadata: %s (%v)", options["indexed"], err)
	}
}

func (f *fixture) scanIndexExists(table *fieldTable, fieldID string, kind automaticIndexKind) bool {
	f.t.Helper()
	name, err := automaticIndexName(fieldID, kind)
	if err != nil {
		f.t.Fatal(err)
	}
	return scanOne[bool](f,
		`SELECT EXISTS (SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE n.nspname = $1 AND c.relname = $2)`, table.pgSchema, name)
}

func (f *fixture) requireIndex(t *testing.T, table *fieldTable, field Field, kind automaticIndexKind) {
	t.Helper()
	name, err := automaticIndexName(field.ID, kind)
	if err != nil {
		t.Fatal(err)
	}
	var method, opclass, column string
	var valid bool
	if err := f.pool.QueryRow(f.ctx, `
		SELECT am.amname, opc.opcname, a.attname, i.indisvalid
		FROM pg_class idx
		JOIN pg_namespace n ON n.oid = idx.relnamespace
		JOIN pg_index i ON i.indexrelid = idx.oid
		JOIN pg_am am ON am.oid = idx.relam
		JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = i.indkey[0]
		JOIN pg_opclass opc ON opc.oid = i.indclass[0]
		WHERE n.nspname = $1 AND idx.relname = $2`, table.pgSchema, name).
		Scan(&method, &opclass, &column, &valid); err != nil {
		t.Fatalf("read index %s: %v", name, err)
	}
	expectedMethod := "btree"
	if kind == automaticTrigram {
		expectedMethod = "gin"
	}
	if method != expectedMethod || column != table.column(field) || !valid ||
		kind == automaticTrigram && opclass != "gin_trgm_ops" {
		t.Fatalf("index %s = method %s, opclass %s, column %s, valid %t", name, method, opclass, column, valid)
	}
}
