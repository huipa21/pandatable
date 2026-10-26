package structure

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"pandatable/internal/acl"
	"pandatable/internal/apperr"
	"pandatable/internal/ddl"
)

// fieldTable is a fresh owner-created table with its physical names.
type fieldTable struct {
	f        *fixture
	owner    Actor
	schema   TableSchema
	pgSchema string
	pgName   string
}

func (f *fixture) fieldTable(owner Actor) *fieldTable {
	f.t.Helper()
	table := f.table(owner, f.book(owner, f.workspace(owner).ID).ID)
	schema, name := f.physicalNames(table.Table.ID)
	return &fieldTable{f: f, owner: owner, schema: table, pgSchema: schema, pgName: name}
}

func (ft *fieldTable) id() string { return ft.schema.Table.ID }

func (ft *fieldTable) quoted() string { return ft.pgSchema + "." + ft.pgName }

func (ft *fieldTable) create(input FieldInput) Field {
	ft.f.t.Helper()
	result, err := ft.f.service.CreateField(ft.f.ctx, ft.owner, ft.id(), input)
	if err != nil {
		ft.f.t.Fatalf("create %+v: %v", input, err)
	}
	if result.Field == nil || result.Schema.Table.ID != ft.id() {
		ft.f.t.Fatalf("create result = %+v", result)
	}
	return *result.Field
}

func (ft *fieldTable) update(field Field, patch FieldPatch) (Field, error) {
	patch.Version = field.Version
	result, err := ft.f.service.UpdateField(ft.f.ctx, ft.owner, field.ID, patch)
	if err != nil {
		return Field{}, err
	}
	return *result.Field, nil
}

func (ft *fieldTable) column(field Field) string {
	ft.f.t.Helper()
	name := scanOne[string](ft.f, "SELECT pg_column FROM meta.fields WHERE id=$1", field.ID)
	if err := ddl.ValidateIdentifier(ddl.Column, name); err != nil {
		ft.f.t.Fatal(err)
	}
	return name
}

func (ft *fieldTable) physical(field Field) column {
	ft.f.t.Helper()
	name := ft.column(field)
	for _, c := range ft.f.columns(ft.pgSchema, ft.pgName) {
		if c.Name == name {
			return c
		}
	}
	ft.f.t.Fatalf("column %s missing", name)
	return column{}
}

func (ft *fieldTable) schemaVersion() int {
	return scanOne[int](ft.f, "SELECT schema_version FROM meta.tables WHERE id=$1", ft.id())
}

func (ft *fieldTable) hasUnique(field Field) bool {
	return scanOne[bool](ft.f, `SELECT EXISTS (SELECT 1 FROM pg_constraint c JOIN pg_class r ON r.oid=c.conrelid
		JOIN pg_namespace n ON n.oid=r.relnamespace JOIN pg_attribute a ON a.attrelid=r.oid AND a.attnum = ANY(c.conkey)
		WHERE n.nspname=$1 AND r.relname=$2 AND c.contype='u' AND a.attname=$3)`, ft.pgSchema, ft.pgName, ft.column(field))
}

func expectReason(t *testing.T, err error, status int, code, reason string) *apperr.Error {
	t.Helper()
	var app *apperr.Error
	if !errors.As(err, &app) || app.Status != status || app.Code != code {
		t.Fatalf("error = %v, want HTTP %d %s", err, status, code)
	}
	if reason != "" && (app.Details == nil || app.Details["reason"] != reason) {
		t.Fatalf("details = %v, want reason %s", app.Details, reason)
	}
	return app
}

func TestCreateFieldWritesTypedColumnsMetadataAndSchemaVersion(t *testing.T) {
	f := newFixture(t)
	ft := f.fieldTable(f.user("user", "active", "en"))
	cases := []struct {
		kind, options, dataType, pgType, canonical string
	}{
		{"text", `{"maxLength":20}`, "character varying", "varchar(20)", `{"maxLength":20}`},
		{"long_text", ``, "text", "text", `{"richText":false}`},
		{"integer", `{"size":"big","thousandSep":false}`, "bigint", "bigint", `{"size":"big","thousandSep":false}`},
		{"decimal", `{"precision":10,"scale":3}`, "numeric", "numeric(10,3)", `{"precision":10,"scale":3,"thousandSep":true}`},
		{"float", `{"size":"real"}`, "real", "real", `{"size":"real","thousandSep":true}`},
		{"checkbox", `{}`, "boolean", "boolean", `{}`},
		{"date", `{"dateFormat":"us"}`, "date", "date", `{"dateFormat":"us"}`},
		{"datetime", `{"timezone":"fixed","fixedTz":"Asia/Shanghai"}`, "timestamp with time zone", "timestamptz",
			`{"dateFormat":"iso","timeFormat":"24h","timezone":"fixed","fixedTz":"Asia/Shanghai"}`},
		{"single_select", `{"choices":[{"id":"a","label":" Todo ","color":"green"}]}`, "text", "text",
			`{"choices":[{"id":"a","label":"Todo","color":"green"}]}`},
		{"multi_select", `{"choices":[]}`, "ARRAY", "text[]", `{"choices":[]}`},
		{"email", ``, "text", "text", `{}`},
		{"url", ``, "text", "text", `{}`},
	}
	version := ft.schemaVersion()
	var previous float64
	for _, tc := range cases {
		field := ft.create(FieldInput{Name: "F " + tc.kind, UIType: tc.kind, Options: json.RawMessage(tc.options),
			Description: "  about " + tc.kind + "  "})
		version++
		if ft.schemaVersion() != version {
			t.Fatalf("%s: schema version %d, want %d", tc.kind, ft.schemaVersion(), version)
		}
		if field.UIType != tc.kind || field.IsSystem || field.Version != 1 || field.Position <= previous ||
			field.Description != "about "+tc.kind || string(field.Default) != "null" {
			t.Fatalf("%s: field = %+v", tc.kind, field)
		}
		previous = field.Position
		var options any
		_ = json.Unmarshal(field.Options, &options)
		canonical, _ := json.Marshal(options)
		var want any
		_ = json.Unmarshal([]byte(tc.canonical), &want)
		wantJSON, _ := json.Marshal(want)
		if string(canonical) != string(wantJSON) {
			t.Fatalf("%s: options %s, want %s", tc.kind, field.Options, tc.canonical)
		}
		physical := ft.physical(field)
		if physical.Type != tc.dataType {
			t.Fatalf("%s: physical type %s, want %s", tc.kind, physical.Type, tc.dataType)
		}
		if got := scanOne[string](f, "SELECT pg_type FROM meta.fields WHERE id=$1", field.ID); got != tc.pgType {
			t.Fatalf("%s: pg_type %s, want %s", tc.kind, got, tc.pgType)
		}
		checkbox := tc.kind == "checkbox"
		if (physical.Nullable == "NO") != checkbox || (physical.Default != nil) != checkbox {
			t.Fatalf("%s: nullable=%s default=%v", tc.kind, physical.Nullable, physical.Default)
		}
	}
	schema, err := f.service.Schema(f.ctx, ft.owner, ft.id())
	if err != nil || len(schema.Fields) != 6+len(cases) || int(schema.SchemaVersion) != version {
		t.Fatalf("schema = %+v %v", schema, err)
	}
	raw, _ := json.Marshal(schema)
	if strings.Contains(string(raw), ft.pgName) || strings.Contains(string(raw), ft.column(schema.Fields[6])) {
		t.Fatal("schema DTO leaks physical names")
	}
}

func TestCreateFieldRejectsInvalidInputWithoutSideEffects(t *testing.T) {
	f := newFixture(t)
	ft := f.fieldTable(f.user("user", "active", "en"))
	columns := len(f.columns(ft.pgSchema, ft.pgName))
	version := ft.schemaVersion()
	for _, tc := range []struct {
		input        FieldInput
		status       int
		code, reason string
		detailOption string
	}{
		{FieldInput{Name: "name", UIType: "text"}, 409, "ERR_FIELD_NAME_CONFLICT", "", ""},
		{FieldInput{Name: " NAME ", UIType: "integer"}, 409, "ERR_FIELD_NAME_CONFLICT", "", ""},
		{FieldInput{Name: "id", UIType: "text"}, 409, "ERR_FIELD_NAME_CONFLICT", "", ""},
		{FieldInput{Name: "  ", UIType: "text"}, 400, "ERR_VALIDATION", "", ""},
		{FieldInput{Name: "x", UIType: "text", Description: strings.Repeat("字", 1001)}, 400, "ERR_VALIDATION", "", ""},
		{FieldInput{Name: "x", UIType: "text", Description: "bell\a"}, 400, "ERR_VALIDATION", "", ""},
		{FieldInput{Name: "x", UIType: "rating"}, 400, "ERR_VALIDATION", "", ""},
		{FieldInput{Name: "x", UIType: "created_at"}, 400, "ERR_VALIDATION", "system_field", ""},
		{FieldInput{Name: "x", UIType: "text", Options: json.RawMessage(`{"color":"red"}`)}, 400, "ERR_VALIDATION", "invalid_options", "color"},
		{FieldInput{Name: "x", UIType: "datetime", Options: json.RawMessage(`{"timezone":"fixed","fixedTz":"Mars/Olympus"}`)},
			400, "ERR_VALIDATION", "invalid_options", "fixedTz"},
		{FieldInput{Name: "x", UIType: "checkbox", IsRequired: true}, 400, "ERR_VALIDATION", "constraint_unsupported", ""},
		{FieldInput{Name: "x", UIType: "long_text", IsUnique: true}, 400, "ERR_VALIDATION", "constraint_unsupported", ""},
		{FieldInput{Name: "x", UIType: "integer", Default: json.RawMessage(`1.5`)}, 400, "ERR_VALIDATION", "", "default"},
		{FieldInput{Name: "x", UIType: "integer", Default: json.RawMessage(`"now()"`)}, 400, "ERR_VALIDATION", "", "default"},
		{FieldInput{Name: "x", UIType: "single_select", Options: json.RawMessage(`{"choices":[{"id":"a","label":"A"}]}`),
			Default: json.RawMessage(`"B"`)}, 400, "ERR_VALIDATION", "", "default"},
	} {
		_, err := f.service.CreateField(f.ctx, ft.owner, ft.id(), tc.input)
		app := expectReason(t, err, tc.status, tc.code, tc.reason)
		if tc.detailOption != "" && app.Details["option"] != tc.detailOption {
			t.Fatalf("%+v: details %v", tc.input, app.Details)
		}
	}
	if got := len(f.columns(ft.pgSchema, ft.pgName)); got != columns || ft.schemaVersion() != version {
		t.Fatalf("rejections changed state: columns %d->%d version %d->%d", columns, got, version, ft.schemaVersion())
	}
	if scanOne[int](f, "SELECT count(*) FROM meta.fields WHERE table_id=$1", ft.id()) != 6 {
		t.Fatal("rejections left metadata")
	}
	// A datetime zone Go can load must still be in pg_timezone_names; a valid
	// one passes the catalog check.
	ft.create(FieldInput{Name: "UTC time", UIType: "datetime", Options: json.RawMessage(`{"timezone":"fixed","fixedTz":"Europe/Paris"}`)})
}

func TestFieldDefaultsRequiredAndUniqueUseExactPreflights(t *testing.T) {
	f := newFixture(t)
	ft := f.fieldTable(f.user("user", "active", "en"))
	for range 3 {
		f.exec("INSERT INTO " + ft.quoted() + " DEFAULT VALUES")
	}
	// Creating with a default does not backfill existing rows; new rows get it.
	score := ft.create(FieldInput{Name: "Score", UIType: "integer", Default: json.RawMessage(`7`)})
	if string(score.Default) != "7" {
		t.Fatalf("canonical default = %s", score.Default)
	}
	col := ft.column(score)
	if got := scanOne[int](f, "SELECT count(*) FROM "+ft.quoted()+" WHERE "+col+" IS NULL"); got != 3 {
		t.Fatalf("existing rows backfilled: %d nulls", got)
	}
	f.exec("INSERT INTO " + ft.quoted() + " DEFAULT VALUES")
	if got := scanOne[int](f, "SELECT count(*) FROM "+ft.quoted()+" WHERE "+col+" = 7"); got != 1 {
		t.Fatalf("new row default count = %d", got)
	}

	// Required with empties fails exactly, and with fillEmpty fills them.
	_, err := ft.update(score, FieldPatch{IsRequired: ptr(true)})
	app := expectReason(t, err, 409, "ERR_REQUIRED_HAS_EMPTY", "")
	if app.Details["count"] != int64(3) {
		t.Fatalf("empty count details = %v", app.Details)
	}
	if ft.physical(score).Nullable != "YES" {
		t.Fatal("failed preflight changed the column")
	}
	score, err = ft.update(score, FieldPatch{IsRequired: ptr(true), FillEmpty: true})
	if err != nil || !score.IsRequired || score.Version != 2 || ft.physical(score).Nullable != "NO" {
		t.Fatalf("fill required = %+v %v", score, err)
	}
	if got := scanOne[int](f, "SELECT count(*) FROM "+ft.quoted()+" WHERE "+col+" = 7"); got != 4 {
		t.Fatalf("filled count = %d", got)
	}
	// Removing the default from a required field is allowed; the NOT NULL stays.
	score, err = ft.update(score, FieldPatch{Default: json.RawMessage(`null`)})
	if err != nil || string(score.Default) != "null" || ft.physical(score).Default != nil {
		t.Fatalf("clear default = %+v %v", score, err)
	}

	// Unique preflight counts duplicate values; distinct values pass.
	_, err = ft.update(score, FieldPatch{IsUnique: ptr(true)})
	app = expectReason(t, err, 409, "ERR_UNIQUE_HAS_DUPLICATES", "")
	if app.Details["count"] != int64(1) || ft.hasUnique(score) {
		t.Fatalf("duplicate details = %v", app.Details)
	}
	f.exec("UPDATE " + ft.quoted() + " SET " + col + " = id")
	score, err = ft.update(score, FieldPatch{IsUnique: ptr(true)})
	if err != nil || !score.IsUnique || !ft.hasUnique(score) {
		t.Fatalf("unique = %+v %v", score, err)
	}
	score, err = ft.update(score, FieldPatch{IsUnique: ptr(false), IsRequired: ptr(false)})
	if err != nil || score.IsUnique || ft.hasUnique(score) || ft.physical(score).Nullable != "YES" {
		t.Fatalf("relax = %+v %v", score, err)
	}

	// A required field without a default rejects fillEmpty; a typed default
	// of a different kind fills the exact typed value.
	when := ft.create(FieldInput{Name: "When", UIType: "date"})
	_, err = ft.update(when, FieldPatch{IsRequired: ptr(true), FillEmpty: true})
	expectError(t, err, 409, "ERR_REQUIRED_HAS_EMPTY")
	when, err = ft.update(when, FieldPatch{IsRequired: ptr(true), FillEmpty: true, Default: json.RawMessage(`"2024-02-29"`)})
	if err != nil {
		t.Fatal(err)
	}
	if got := scanOne[int](f, "SELECT count(*) FROM "+ft.quoted()+" WHERE "+ft.column(when)+" = DATE '2024-02-29'"); got != 4 {
		t.Fatalf("date fill = %d", got)
	}
	if def := ft.physical(when).Default; def == nil || !strings.Contains(*def, "2024-02-29") {
		t.Fatalf("physical date default = %v", def)
	}

	// Checkbox: default true is typed; clearing restores the forced false.
	done := ft.create(FieldInput{Name: "Done", UIType: "checkbox", Default: json.RawMessage(`true`)})
	if def := ft.physical(done).Default; def == nil || *def != "true" {
		t.Fatalf("checkbox default = %v", def)
	}
	if got := scanOne[int](f, "SELECT count(*) FROM "+ft.quoted()+" WHERE NOT "+ft.column(done)); got != 4 {
		t.Fatalf("checkbox add must keep existing rows false: %d", got)
	}
	done, err = ft.update(done, FieldPatch{Default: json.RawMessage(`null`)})
	if def := ft.physical(done).Default; err != nil || def == nil || *def != "false" {
		t.Fatalf("checkbox cleared default = %v %v", def, err)
	}
}

func TestFieldOptionEditsRewriteSelectLabelsAndRejectStorageChanges(t *testing.T) {
	f := newFixture(t)
	ft := f.fieldTable(f.user("user", "active", "en"))
	status := ft.create(FieldInput{Name: "Status", UIType: "single_select",
		Options: json.RawMessage(`{"choices":[{"id":"a","label":"Todo"},{"id":"b","label":"Done"},{"id":"c","label":"Gone"}]}`),
		Default: json.RawMessage(`"Todo"`)})
	tags := ft.create(FieldInput{Name: "Tags", UIType: "multi_select",
		Options: json.RawMessage(`{"choices":[{"id":"x","label":"Red"},{"id":"y","label":"Blue"},{"id":"z","label":"Old"}]}`),
		Default: json.RawMessage(`["Blue","Red"]`)})
	if string(tags.Default) != `["Red","Blue"]` {
		t.Fatalf("multi default must follow choice order: %s", tags.Default)
	}
	statusCol, tagsCol := ft.column(status), ft.column(tags)
	f.exec("INSERT INTO "+ft.quoted()+" ("+statusCol+","+tagsCol+") VALUES ($1,$2),($3,$4),($5,$6),($7,$8)",
		"Todo", []string{"Red", "Old"}, "Done", []string{"Blue", "Red"}, "Gone", []string{"Old", "Blue"}, nil, nil)

	// Swap Todo/Done labels by id, drop Gone; rename Red→Crimson and merge Old
	// into Blue's new label via a rename that removes the Old choice.
	status, err := ft.update(status, FieldPatch{Options: json.RawMessage(
		`{"choices":[{"id":"a","label":"Done"},{"id":"b","label":"Todo"}]}`)})
	if err != nil {
		t.Fatal(err)
	}
	if string(status.Default) != `"Done"` {
		t.Fatalf("renamed single default = %s", status.Default)
	}
	tags, err = ft.update(tags, FieldPatch{Options: json.RawMessage(
		`{"choices":[{"id":"x","label":"Crimson"},{"id":"y","label":"Old"}]}`)})
	if err != nil {
		t.Fatal(err)
	}
	if string(tags.Default) != `["Crimson","Old"]` {
		t.Fatalf("renamed multi default = %s", tags.Default)
	}
	rows, err := f.pool.Query(f.ctx, "SELECT "+statusCol+", "+tagsCol+" FROM "+ft.quoted()+" ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for rows.Next() {
		var single *string
		var multi []string
		if err := rows.Scan(&single, &multi); err != nil {
			t.Fatal(err)
		}
		value := "<nil>"
		if single != nil {
			value = *single
		}
		got = append(got, value+"|"+strings.Join(multi, ","))
	}
	rows.Close()
	// Row 3: "Old" (removed z, stored label kept) and "Blue"→"Old" merge
	// into one element in first-occurrence order.
	want := []string{"Done|Crimson,Old", "Todo|Old,Crimson", "Gone|Old", "<nil>|"}
	if strings.Join(got, ";") != strings.Join(want, ";") {
		t.Fatalf("rewritten values = %v, want %v", got, want)
	}

	for _, tc := range []struct {
		field   Field
		options string
	}{
		{status, `{"choices":[{"id":"a","label":"DONE"},{"id":"b","label":"done"}]}`},
		{ft.create(FieldInput{Name: "N", UIType: "integer"}), `{"size":"big"}`},
		{ft.create(FieldInput{Name: "D", UIType: "decimal"}), `{"precision":20,"scale":2}`},
		{ft.create(FieldInput{Name: "T", UIType: "text"}), `{"maxLength":5}`},
	} {
		_, err := ft.update(tc.field, FieldPatch{Options: json.RawMessage(tc.options)})
		if tc.field.ID == status.ID {
			expectReason(t, err, 400, "ERR_VALIDATION", "invalid_options")
		} else {
			expectReason(t, err, 400, "ERR_VALIDATION", "storage_change")
		}
	}
	// Display-only option edits are metadata-only.
	n := ft.create(FieldInput{Name: "Plain", UIType: "integer"})
	if n, err = ft.update(n, FieldPatch{Options: json.RawMessage(`{"thousandSep":false}`)}); err != nil || n.Version != 2 {
		t.Fatalf("display option = %+v %v", n, err)
	}
}

func TestSystemFieldsAllowOnlyNameAndDescription(t *testing.T) {
	f := newFixture(t)
	ft := f.fieldTable(f.user("user", "active", "zh-CN"))
	created := ft.schema.Fields[2]
	if created.UIType != "created_at" {
		t.Fatalf("fixture field = %+v", created)
	}
	renamed, err := ft.update(created, FieldPatch{Name: ptr("录入时间"), Description: ptr("系统")})
	if err != nil || renamed.Name != "录入时间" || renamed.Version != 2 {
		t.Fatalf("system rename = %+v %v", renamed, err)
	}
	for _, patch := range []FieldPatch{{Options: json.RawMessage(`{}`)}, {IsRequired: ptr(false)}, {IsUnique: ptr(true)},
		{Default: json.RawMessage(`null`)}, {Index: ptr(0)}} {
		_, err := ft.update(renamed, patch)
		expectReason(t, err, 400, "ERR_VALIDATION", "system_field")
	}
	_, err = f.service.DeleteField(f.ctx, ft.owner, renamed.ID, renamed.Version)
	expectReason(t, err, 400, "ERR_VALIDATION", "system_field")
	_, err = f.service.UpdateTable(f.ctx, ft.owner, ft.id(), TablePatch{DisplayFieldID: &renamed.ID})
	expectReason(t, err, 400, "ERR_VALIDATION", "display_ineligible")
	if got := scanOne[string](f, "SELECT pg_column FROM meta.fields WHERE id=$1", renamed.ID); got != "created_at" {
		t.Fatalf("system column changed: %s", got)
	}
}

func TestFieldRenameIsMetadataOnlyAndVersioned(t *testing.T) {
	f := newFixture(t)
	ft := f.fieldTable(f.user("user", "active", "en"))
	field := ft.create(FieldInput{Name: "Amount", UIType: "decimal"})
	col := ft.column(field)
	f.exec("INSERT INTO "+ft.quoted()+" ("+col+") VALUES ($1)", "12.50")
	payload := "x'); DROP TABLE meta.users; --"
	for _, name := range []string{"金额 合计", payload, "amount"} {
		updated, err := ft.update(field, FieldPatch{Name: ptr(name)})
		if err != nil || updated.Name != name || updated.Version != field.Version+1 {
			t.Fatalf("rename %q = %+v %v", name, updated, err)
		}
		field = updated
	}
	if ft.column(field) != col || scanOne[string](f, "SELECT "+col+"::text FROM "+ft.quoted()) != "12.50" {
		t.Fatal("rename changed physical name or data")
	}
	// Stale versions conflict; trimmed-case sibling names conflict.
	_, err := f.service.UpdateField(f.ctx, ft.owner, field.ID, FieldPatch{Version: 1, Name: ptr("x")})
	expectError(t, err, 409, "ERR_VERSION_CONFLICT")
	_, err = ft.update(field, FieldPatch{Name: ptr(" NAME ")})
	expectError(t, err, 409, "ERR_FIELD_NAME_CONFLICT")
	_, err = ft.update(field, FieldPatch{})
	expectError(t, err, 400, "ERR_VALIDATION")
	_, err = f.service.UpdateField(f.ctx, ft.owner, "not-a-uuid", FieldPatch{Version: 1, Name: ptr("x")})
	expectError(t, err, 404, "ERR_NOT_FOUND")
}

func TestFieldReorderAndSoftDeleteKeepDataAndReassignDisplay(t *testing.T) {
	f := newFixture(t)
	ft := f.fieldTable(f.user("user", "active", "en"))
	a := ft.create(FieldInput{Name: "A", UIType: "text", IsUnique: true, Default: json.RawMessage(`"same"`)})
	b := ft.create(FieldInput{Name: "B", UIType: "checkbox"})
	c := ft.create(FieldInput{Name: "C", UIType: "email"})
	order := func() string {
		schema, err := f.service.Schema(f.ctx, ft.owner, ft.id())
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, field := range schema.Fields {
			if !field.IsSystem {
				names = append(names, field.Name)
			}
		}
		return strings.Join(names, ",")
	}
	if got := order(); got != "Name,A,B,C" {
		t.Fatalf("initial order = %s", got)
	}
	c, err := ft.update(c, FieldPatch{Index: ptr(0)})
	if err != nil || order() != "C,Name,A,B" {
		t.Fatalf("reorder = %s %v", order(), err)
	}
	_, err = ft.update(c, FieldPatch{Index: ptr(9)})
	expectError(t, err, 400, "ERR_VALIDATION")

	// Display reassigns to the first eligible remaining field (C), skipping
	// the checkbox; the deleted column keeps data and accepts new rows.
	f.exec("INSERT INTO " + ft.quoted() + " DEFAULT VALUES")
	a, err = ft.update(a, FieldPatch{IsRequired: ptr(true)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.UpdateTable(f.ctx, ft.owner, ft.id(), TablePatch{DisplayFieldID: &a.ID}); err != nil {
		t.Fatal(err)
	}
	_, err = f.service.UpdateTable(f.ctx, ft.owner, ft.id(), TablePatch{DisplayFieldID: &b.ID})
	expectReason(t, err, 400, "ERR_VALIDATION", "display_ineligible")
	_, err = f.service.UpdateTable(f.ctx, ft.owner, ft.id(), TablePatch{DisplayFieldID: ptr("00000000-0000-0000-0000-000000000000")})
	expectError(t, err, 400, "ERR_VALIDATION")
	_, err = f.service.DeleteField(f.ctx, ft.owner, a.ID, a.Version-1)
	expectError(t, err, 409, "ERR_VERSION_CONFLICT")
	result, err := f.service.DeleteField(f.ctx, ft.owner, a.ID, a.Version)
	if err != nil || result.Field != nil || *result.Schema.Table.DisplayFieldID != c.ID {
		t.Fatalf("delete display = %+v %v", result.Schema.Table, err)
	}
	if order() != "C,Name,B" {
		t.Fatalf("after delete = %s", order())
	}
	physical := ft.physical(a)
	if physical.Nullable != "YES" || physical.Default != nil || !ft.hasUnique(a) {
		t.Fatalf("deleted column = %+v unique=%v", physical, ft.hasUnique(a))
	}
	f.exec("INSERT INTO " + ft.quoted() + " DEFAULT VALUES")
	f.exec("INSERT INTO " + ft.quoted() + " DEFAULT VALUES")
	if got := scanOne[int](f, "SELECT count(*) FROM "+ft.quoted()+" WHERE "+ft.column(a)+" = 'same'"); got != 1 {
		t.Fatalf("deleted column data = %d", got)
	}
	// A deleted name is free again; deleted fields are 404.
	ft.create(FieldInput{Name: "a", UIType: "text"})
	_, err = f.service.DeleteField(f.ctx, ft.owner, a.ID, a.Version+1)
	expectError(t, err, 404, "ERR_NOT_FOUND")

	// The last eligible field cannot be deleted while it is the display field.
	solo := f.fieldTable(ft.owner)
	soloName := solo.schema.Fields[0]
	_, err = f.service.DeleteField(f.ctx, solo.owner, soloName.ID, soloName.Version)
	expectError(t, err, 409, "ERR_DISPLAY_FIELD_REQUIRED")
	solo.create(FieldInput{Name: "Flag", UIType: "checkbox"})
	_, err = f.service.DeleteField(f.ctx, solo.owner, soloName.ID, soloName.Version)
	expectError(t, err, 409, "ERR_DISPLAY_FIELD_REQUIRED")
}

func TestFieldMutationsRequireTableOwner(t *testing.T) {
	f := newFixture(t)
	alice, bob, carol, admin := f.user("user", "active", "en"), f.user("user", "active", "en"),
		f.user("user", "active", "en"), f.user("admin", "active", "en")
	ft := f.fieldTable(alice)
	field := ft.create(FieldInput{Name: "Owned", UIType: "text"})
	f.grant(acl.Table, ft.id(), "user", bob.ID, acl.Write)
	f.grant(acl.Table, ft.id(), "user", carol.ID, acl.Read)
	version := ft.schemaVersion()
	for _, actor := range []struct {
		user   Actor
		status int
		code   string
	}{{bob, 403, "ERR_PERMISSION_DENIED"}, {carol, 403, "ERR_PERMISSION_DENIED"}, {admin, 404, "ERR_NOT_FOUND"}} {
		_, err := f.service.CreateField(f.ctx, actor.user, ft.id(), FieldInput{Name: "X", UIType: "text"})
		expectError(t, err, actor.status, actor.code)
		_, err = f.service.UpdateField(f.ctx, actor.user, field.ID, FieldPatch{Version: field.Version, Name: ptr("X")})
		expectError(t, err, actor.status, actor.code)
		_, err = f.service.DeleteField(f.ctx, actor.user, field.ID, field.Version)
		expectError(t, err, actor.status, actor.code)
		_, err = f.service.UpdateTable(f.ctx, actor.user, ft.id(), TablePatch{DisplayFieldID: &field.ID})
		expectError(t, err, actor.status, actor.code)
	}
	if ft.schemaVersion() != version {
		t.Fatal("denied mutations changed schema version")
	}
	// A table-level owner grant (not book/workspace owner) suffices for field
	// schema and display, but not for table rename (parent book owner).
	dave := f.user("user", "active", "en")
	f.grant(acl.Table, ft.id(), "user", dave.ID, acl.Owner)
	if _, err := f.service.CreateField(f.ctx, dave, ft.id(), FieldInput{Name: "Dave", UIType: "url"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.UpdateTable(f.ctx, dave, ft.id(), TablePatch{DisplayFieldID: &field.ID}); err != nil {
		t.Fatal(err)
	}
	_, err := f.service.UpdateTable(f.ctx, dave, ft.id(), TablePatch{Name: ptr("x"), DisplayFieldID: &field.ID})
	expectError(t, err, 403, "ERR_PERMISSION_DENIED")
}

func TestConcurrentFieldUpdatesWithOneVersionCommitOnce(t *testing.T) {
	f := newFixture(t)
	ft := f.fieldTable(f.user("user", "active", "en"))
	field := ft.create(FieldInput{Name: "Race", UIType: "text"})
	var wait sync.WaitGroup
	errs := make([]error, 6)
	for index := range errs {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, errs[index] = f.service.UpdateField(f.ctx, ft.owner, field.ID,
				FieldPatch{Version: field.Version, Name: ptr("Race " + string(rune('a'+index)))})
		}()
	}
	wait.Wait()
	ok := 0
	for _, err := range errs {
		if err == nil {
			ok++
			continue
		}
		expectError(t, err, 409, "ERR_VERSION_CONFLICT")
	}
	if ok != 1 || scanOne[int](f, "SELECT version FROM meta.fields WHERE id=$1", field.ID) != 2 {
		t.Fatalf("%d winners", ok)
	}
	// Concurrent creations of distinct fields all commit with distinct columns
	// and serialized schema versions.
	version := ft.schemaVersion()
	for index := range errs {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, errs[index] = f.service.CreateField(f.ctx, ft.owner, ft.id(),
				FieldInput{Name: "Parallel " + string(rune('a'+index)), UIType: "integer"})
		}()
	}
	wait.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if ft.schemaVersion() != version+len(errs) ||
		scanOne[int](f, "SELECT count(DISTINCT pg_column) FROM meta.fields WHERE table_id=$1", ft.id()) != 7+len(errs) {
		t.Fatal("parallel creations lost updates")
	}
}

func TestFieldColumnCollisionsRetryWholeTransactions(t *testing.T) {
	f := newFixture(t)
	ft := f.fieldTable(f.user("user", "active", "en"))
	existing := ft.column(ft.schema.Fields[0])
	f.exec("ALTER TABLE " + ft.quoted() + " ADD COLUMN c_zzzzzzzzzz text")
	version := ft.schemaVersion()
	script := f.script(map[ddl.ObjectKind][]string{ddl.Column: {existing, "c_zzzzzzzzzz"}})
	field := ft.create(FieldInput{Name: "Retried", UIType: "text"})
	if script.calls[ddl.Column] != 3 || ft.schemaVersion() != version+1 {
		t.Fatalf("column names generated %d times, version %d", script.calls[ddl.Column], ft.schemaVersion())
	}
	if got := ft.column(field); got == existing || got == "c_zzzzzzzzzz" {
		t.Fatalf("final column reused %s", got)
	}
	f.script(map[ddl.ObjectKind][]string{ddl.Column: {existing, existing, existing}})
	_, err := f.service.CreateField(f.ctx, ft.owner, ft.id(), FieldInput{Name: "Never", UIType: "text"})
	expectError(t, err, 409, "ERR_DDL_CONFLICT")
	if scanOne[int](f, "SELECT count(*) FROM meta.fields WHERE table_id=$1 AND name='Never'", ft.id()) != 0 ||
		ft.schemaVersion() != version+1 {
		t.Fatal("exhausted retries left state")
	}
}

func TestFailedFieldDDLRollsBackMetadata(t *testing.T) {
	f := newFixture(t)
	ft := f.fieldTable(f.user("user", "active", "en"))
	field := ft.create(FieldInput{Name: "Code", UIType: "text"})
	col := ft.column(field)
	// Hold a conflicting lock so the DDL inside the transaction times out.
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(f.ctx) }()
	if _, err := tx.Exec(f.ctx, "LOCK TABLE "+ft.quoted()+" IN ACCESS SHARE MODE"); err != nil {
		t.Fatal(err)
	}
	version := ft.schemaVersion()
	_, err = ft.update(field, FieldPatch{Name: ptr("Renamed"), IsUnique: ptr(true)})
	if err == nil {
		t.Fatal("DDL under a held lock must fail")
	}
	var app *apperr.Error
	if errors.As(err, &app) && strings.Contains(app.Message, col) {
		t.Fatal("error leaks physical name")
	}
	_ = tx.Rollback(f.ctx)
	got := scanOne[string](f, "SELECT name FROM meta.fields WHERE id=$1", field.ID)
	if got != "Code" || ft.schemaVersion() != version || ft.hasUnique(field) {
		t.Fatalf("rollback left name=%s version=%d", got, ft.schemaVersion())
	}
}
