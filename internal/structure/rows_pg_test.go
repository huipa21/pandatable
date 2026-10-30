package structure

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"pandatable/internal/acl"
	"pandatable/internal/apperr"
	"pandatable/internal/codec"
)

func rawJSON(value string) json.RawMessage { return json.RawMessage(value) }

func fieldByType(fields []Field, kind string) Field {
	for _, field := range fields {
		if field.UIType == kind {
			return field
		}
	}
	return Field{}
}

func wireString(t *testing.T, value json.RawMessage) string {
	t.Helper()
	var text string
	if err := json.Unmarshal(value, &text); err != nil {
		t.Fatal(err)
	}
	return text
}

func expectRowError(t *testing.T, err error, status int, code string) {
	t.Helper()
	var app *apperr.Error
	if !errors.As(err, &app) || app.Status != status || app.Code != code {
		t.Fatalf("error = %v, want HTTP %d %s", err, status, code)
	}
}

func TestRowsRoundTripAllBatchOneKindsAndMaintainAuthorship(t *testing.T) {
	f := newFixture(t)
	owner := f.user("user", "active", "en")
	ft := f.fieldTable(owner)
	specs := []struct {
		name, kind, options string
		value               json.RawMessage
	}{
		{"Text", "text", `{}`, rawJSON(`"hello"`)},
		{"Long text", "long_text", `{}`, rawJSON(`"first\nsecond"`)},
		{"Integer", "integer", `{"size":"int"}`, rawJSON(`42`)},
		{"Decimal", "decimal", `{"precision":12,"scale":2}`, rawJSON(`"1234.50"`)},
		{"Float", "float", `{"size":"real"}`, rawJSON(`1.25`)},
		{"Checkbox", "checkbox", `{}`, rawJSON(`true`)},
		{"Date", "date", `{}`, rawJSON(`"2026-10-26"`)},
		{"Datetime", "datetime", `{}`, rawJSON(`"2026-10-26T02:20:30.123456Z"`)},
		{"Single", "single_select", `{"choices":[{"id":"open","label":"Open","color":"green"}]}`, rawJSON(`"Open"`)},
		{"Multiple", "multi_select", `{"choices":[{"id":"open","label":"Open","color":"green"},{"id":"done","label":"Done","color":"blue"}]}`,
			rawJSON(`["Open","Done"]`)},
		{"Email", "email", `{}`, rawJSON(`"person@example.com"`)},
		{"URL", "url", `{}`, rawJSON(`"https://example.com/path"`)},
	}
	input := make(map[string]json.RawMessage, len(specs))
	fieldsByType := make(map[string]Field, len(specs))
	for _, spec := range specs {
		field := ft.create(FieldInput{Name: spec.name, UIType: spec.kind, Options: rawJSON(spec.options)})
		fieldsByType[spec.kind] = field
		input[field.ID] = spec.value
	}
	created, err := f.service.CreateRows(f.ctx, owner, ft.id(), []RowInsert{{Fields: input}})
	if err != nil || len(created) != 1 {
		t.Fatalf("create rows = %v, %v", created, err)
	}
	row := created[0]
	if row.ID == "" || row.ID[0] == '0' {
		t.Fatalf("row id is not canonical: %q", row.ID)
	}
	for _, spec := range specs {
		field := fieldsByType[spec.kind]
		if got := string(row.Fields[field.ID]); got != string(spec.value) {
			t.Errorf("%s encoded %s, want %s", spec.kind, got, spec.value)
		}
	}
	createdBy := fieldByType(ft.schema.Fields, string(codec.CreatedBy))
	updatedBy := fieldByType(ft.schema.Fields, string(codec.UpdatedBy))
	createdAt := fieldByType(ft.schema.Fields, string(codec.CreatedAt))
	updatedAt := fieldByType(ft.schema.Fields, string(codec.UpdatedAt))
	rowIDField := fieldByType(ft.schema.Fields, string(codec.RowID))
	if wireString(t, row.Fields[rowIDField.ID]) != row.ID {
		t.Fatalf("system row id = %s, want %s", row.Fields[rowIDField.ID], row.ID)
	}
	if wireString(t, row.Fields[createdBy.ID]) != owner.ID || wireString(t, row.Fields[updatedBy.ID]) != owner.ID {
		t.Fatalf("insert authorship = %s / %s", row.Fields[createdBy.ID], row.Fields[updatedBy.ID])
	}
	if wireString(t, row.Fields[createdAt.ID]) != wireString(t, row.Fields[updatedAt.ID]) {
		t.Fatalf("insert timestamps differ: %s / %s", row.Fields[createdAt.ID], row.Fields[updatedAt.ID])
	}

	other := f.user("user", "active", "en")
	f.grant(acl.Table, ft.id(), "user", other.ID, acl.Write)
	patched, err := f.service.PatchRow(f.ctx, other, ft.id(), row.ID,
		map[string]json.RawMessage{fieldsByType["text"].ID: rawJSON(`"updated"`)})
	if err != nil {
		t.Fatal(err)
	}
	if wireString(t, patched.Fields[fieldsByType["text"].ID]) != "updated" ||
		wireString(t, patched.Fields[createdBy.ID]) != owner.ID || wireString(t, patched.Fields[updatedBy.ID]) != other.ID {
		t.Fatalf("patch fields/authorship = %+v", patched.Fields)
	}
	if !rowTimeAfter(t, patched.Fields[updatedAt.ID], row.Fields[updatedAt.ID]) {
		t.Fatal("updated_at did not advance after patch")
	}
	read, err := f.service.GetRow(f.ctx, owner, ft.id(), row.ID)
	if err != nil || !reflect.DeepEqual(read, patched) {
		t.Fatalf("get row = %+v, %v; want %+v", read, err, patched)
	}
}

func TestRowsRejectPrecisionTimezoneAndSelectViolations(t *testing.T) {
	f := newFixture(t)
	owner := f.user("user", "active", "en")
	ft := f.fieldTable(owner)
	decimal := ft.create(FieldInput{
		Name: "Amount", UIType: "decimal", Options: rawJSON(`{"precision":12,"scale":2}`),
	})
	datetime := ft.create(FieldInput{Name: "Moment", UIType: "datetime"})
	choice := ft.create(FieldInput{
		Name: "State", UIType: "single_select",
		Options: rawJSON(`{"choices":[{"id":"open","label":"Open","color":"green"}]}`),
	})

	rows, err := f.service.CreateRows(f.ctx, owner, ft.id(), []RowInsert{{Fields: map[string]json.RawMessage{
		decimal.ID:  rawJSON(`"12.300"`),
		datetime.ID: rawJSON(`"2026-10-26T03:20:30.123456+01:00"`),
		choice.ID:   rawJSON(`"Open"`),
	}}})
	if err != nil || len(rows) != 1 {
		t.Fatalf("valid edge values: rows=%+v err=%v", rows, err)
	}
	if wireString(t, rows[0].Fields[decimal.ID]) != "12.30" ||
		wireString(t, rows[0].Fields[datetime.ID]) != "2026-10-26T02:20:30.123456Z" {
		t.Fatalf("canonical edge values: decimal=%s datetime=%s",
			rows[0].Fields[decimal.ID], rows[0].Fields[datetime.ID])
	}

	for name, fields := range map[string]map[string]json.RawMessage{
		"decimal precision": {decimal.ID: rawJSON(`"12.301"`)},
		"datetime offset":   {datetime.ID: rawJSON(`"2026-10-26T03:20:30.123456"`)},
		"unknown select":    {choice.ID: rawJSON(`"Closed"`)},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := f.service.CreateRows(f.ctx, owner, ft.id(), []RowInsert{{Fields: fields}})
			expectRowError(t, err, 400, "ERR_VALIDATION")
		})
	}
}

func rowTimeAfter(t *testing.T, later, earlier json.RawMessage) bool {
	t.Helper()
	var laterText, earlierText string
	if err := json.Unmarshal(later, &laterText); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(earlier, &earlierText); err != nil {
		t.Fatal(err)
	}
	newer, err := time.Parse(time.RFC3339Nano, laterText)
	if err != nil {
		t.Fatal(err)
	}
	older, err := time.Parse(time.RFC3339Nano, earlierText)
	if err != nil {
		t.Fatal(err)
	}
	return newer.After(older)
}

func TestRowsPhysicalDefaultsConstraintsAndBulkRollback(t *testing.T) {
	f := newFixture(t)
	owner := f.user("user", "active", "en")
	ft := f.fieldTable(owner)
	required := ft.create(FieldInput{Name: "Required", UIType: "text", IsRequired: true})
	code := ft.create(FieldInput{Name: "Code", UIType: "text", IsUnique: true, Default: rawJSON(`"DEFAULT"`)})

	rows, err := f.service.CreateRows(f.ctx, owner, ft.id(), []RowInsert{{Fields: map[string]json.RawMessage{
		required.ID: rawJSON(`"present"`),
	}}})
	if err != nil || len(rows) != 1 || wireString(t, rows[0].Fields[code.ID]) != "DEFAULT" {
		t.Fatalf("default-on-insert = %+v, %v", rows, err)
	}
	expectRowError(t, func() error {
		_, err := f.service.CreateRows(f.ctx, owner, ft.id(), []RowInsert{{Fields: map[string]json.RawMessage{
			required.ID: rawJSON(`"next"`),
		}}})
		return err
	}(), 409, "ERR_UNIQUE_CONFLICT")
	expectRowError(t, func() error {
		_, err := f.service.CreateRows(f.ctx, owner, ft.id(), []RowInsert{{Fields: map[string]json.RawMessage{
			code.ID: rawJSON(`"EXPLICIT"`),
		}}})
		return err
	}(), 400, "ERR_REQUIRED_FIELD")

	integer := ft.create(FieldInput{Name: "Number", UIType: "integer"})
	before := scanOne[int](f, "SELECT count(*) FROM "+ft.quoted())
	_, err = f.service.CreateRows(f.ctx, owner, ft.id(), []RowInsert{
		{Fields: map[string]json.RawMessage{required.ID: rawJSON(`"first"`), code.ID: rawJSON(`"one"`), integer.ID: rawJSON(`1`)}},
		{Fields: map[string]json.RawMessage{required.ID: rawJSON(`"second"`), code.ID: rawJSON(`"two"`), integer.ID: rawJSON(`"not-an-int"`)}},
	})
	expectRowError(t, err, 400, "ERR_VALIDATION")
	if got := scanOne[int](f, "SELECT count(*) FROM "+ft.quoted()); got != before {
		t.Fatalf("invalid bulk insert partially committed: %d rows, expected %d", got, before)
	}

	first := ft.create(FieldInput{Name: "Bulk A", UIType: "text"})
	second := ft.create(FieldInput{Name: "Bulk B", UIType: "text"})
	inserted, err := f.service.CreateRows(f.ctx, owner, ft.id(), []RowInsert{
		{Fields: map[string]json.RawMessage{required.ID: rawJSON(`"a"`), code.ID: rawJSON(`"a"`), integer.ID: rawJSON(`1`),
			first.ID: rawJSON(`"before-a"`), second.ID: rawJSON(`"before-b"`)}},
		{Fields: map[string]json.RawMessage{required.ID: rawJSON(`"b"`), code.ID: rawJSON(`"b"`), integer.ID: rawJSON(`2`),
			first.ID: rawJSON(`"before-a2"`), second.ID: rawJSON(`"before-b2"`)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.service.PatchRows(f.ctx, owner, ft.id(), []RowPatch{
		{ID: inserted[0].ID, Fields: map[string]json.RawMessage{first.ID: rawJSON(`"after"`)}},
		{ID: inserted[1].ID, Fields: map[string]json.RawMessage{second.ID: rawJSON(`"bad"`), "00000000-0000-0000-0000-000000000000": rawJSON(`1`)}},
	})
	expectRowError(t, err, 400, "ERR_VALIDATION")
	unchanged, err := f.service.GetRow(f.ctx, owner, ft.id(), inserted[0].ID)
	if err != nil || wireString(t, unchanged.Fields[first.ID]) != "before-a" {
		t.Fatalf("invalid bulk patch partially committed: %+v, %v", unchanged, err)
	}
	reversed, err := f.service.PatchRows(f.ctx, owner, ft.id(), []RowPatch{
		{ID: inserted[1].ID, Fields: map[string]json.RawMessage{first.ID: rawJSON(`"reverse-second"`)}},
		{ID: inserted[0].ID, Fields: map[string]json.RawMessage{first.ID: rawJSON(`"reverse-first"`)}},
	})
	if err != nil || len(reversed) != 2 || reversed[0].ID != inserted[1].ID ||
		reversed[1].ID != inserted[0].ID {
		t.Fatalf("bulk patch response order changed: rows=%+v err=%v", reversed, err)
	}
	_, err = f.service.DeleteRows(f.ctx, owner, ft.id(), []string{inserted[0].ID, "999999999999"})
	expectRowError(t, err, 404, "ERR_NOT_FOUND")
	if scanOne[int](f, "SELECT count(*) FROM "+ft.quoted()) != before+2 {
		t.Fatal("failed bulk delete removed rows")
	}
	deleted, err := f.service.DeleteRows(f.ctx, owner, ft.id(), []string{inserted[0].ID, inserted[1].ID})
	if err != nil || len(deleted) != 2 {
		t.Fatalf("delete rows = %+v, %v", deleted, err)
	}
}

func TestQueryUsesViewColumnsCountSortAndBounds(t *testing.T) {
	f := newFixture(t)
	owner := f.user("user", "active", "en")
	ft := f.fieldTable(owner)
	text := ft.create(FieldInput{Name: "Text", UIType: "text"})
	number := ft.create(FieldInput{Name: "Number", UIType: "integer"})
	sysID := fieldByType(ft.schema.Fields, string(codec.RowID))
	nameField := Field{}
	for _, field := range ft.schema.Fields {
		if !field.IsSystem {
			nameField = field
			break
		}
	}
	for _, input := range []map[string]json.RawMessage{
		{text.ID: rawJSON(`"two"`), number.ID: rawJSON(`2`)},
		{text.ID: rawJSON(`"null"`), number.ID: rawJSON(`null`)},
		{text.ID: rawJSON(`"one"`), number.ID: rawJSON(`1`)},
	} {
		if _, err := f.service.CreateRows(f.ctx, owner, ft.id(), []RowInsert{{Fields: input}}); err != nil {
			t.Fatal(err)
		}
	}
	viewID := ft.schema.Views[0].ID
	var result RowQueryResult
	result, err := f.service.QueryRows(f.ctx, owner, viewID, RowQueryRequest{Sort: &RowSort{FieldID: number.ID, Direction: "asc"}})
	if err != nil || len(result.Rows) != 3 || result.Total == nil || *result.Total != 3 {
		t.Fatalf("query result = %+v, %v", result, err)
	}
	if got := []string{wireString(t, result.Rows[0].Fields[text.ID]), wireString(t, result.Rows[1].Fields[text.ID]),
		wireString(t, result.Rows[2].Fields[text.ID])}; !reflect.DeepEqual(got, []string{"one", "two", "null"}) {
		t.Fatalf("ascending/null-last order = %v", got)
	}
	if len(result.FieldIDs) != 3 || result.FieldIDs[0] != nameField.ID ||
		result.FieldIDs[1] != text.ID || result.FieldIDs[2] != number.ID {
		t.Fatalf("default field order = %v", result.FieldIDs)
	}
	if _, exists := result.Rows[0].Fields[sysID.ID]; exists {
		t.Fatal("system field appeared in the default view")
	}
	config, _ := json.Marshal(map[string]any{"fields": []map[string]any{
		{"id": number.ID, "visible": true},
		{"id": sysID.ID, "visible": true},
		{"id": text.ID, "visible": false},
		{"id": nameField.ID, "visible": false},
	}, "sort": []map[string]string{{"field": number.ID, "dir": "desc"}}})
	f.exec("UPDATE meta.views SET config=$1::jsonb WHERE id=$2", config, viewID)
	result, err = f.service.QueryRows(f.ctx, owner, viewID, RowQueryRequest{IncludeCount: boolPtr(false)})
	if err != nil || result.Total != nil || len(result.FieldIDs) != 2 ||
		result.FieldIDs[0] != number.ID || result.FieldIDs[1] != sysID.ID {
		t.Fatalf("configured view query = %+v, %v", result, err)
	}
	if len(result.Rows) != 3 || string(result.Rows[0].Fields[number.ID]) != "2" ||
		string(result.Rows[1].Fields[number.ID]) != "1" || string(result.Rows[2].Fields[number.ID]) != "null" ||
		wireString(t, result.Rows[0].Fields[sysID.ID]) != result.Rows[0].ID {
		t.Fatalf("configured sort/system values = %+v", result.Rows)
	}
	requested := []string{sysID.ID, text.ID, sysID.ID, "00000000-0000-0000-0000-000000000000"}
	result, err = f.service.QueryRows(f.ctx, owner, viewID, RowQueryRequest{FieldIDs: &requested})
	if err != nil || len(result.FieldIDs) != 2 ||
		result.FieldIDs[0] != sysID.ID || result.FieldIDs[1] != text.ID ||
		wireString(t, result.Rows[0].Fields[sysID.ID]) != result.Rows[0].ID {
		t.Fatalf("explicit field query = %+v, %v", result, err)
	}
	_, err = f.service.QueryRows(f.ctx, owner, viewID, RowQueryRequest{Offset: intPtr(-1)})
	expectRowError(t, err, 400, "ERR_VALIDATION")
	_, err = f.service.QueryRows(f.ctx, owner, viewID, RowQueryRequest{Limit: intPtr(maxQueryLimit + 1)})
	expectRowError(t, err, 400, "ERR_VALIDATION")
}

func TestQueryFiltersNestedGroupsAndEscapesLikeWildcards(t *testing.T) {
	f := newFixture(t)
	owner := f.user("user", "active", "en")
	ft := f.fieldTable(owner)
	label := ft.create(FieldInput{Name: "Label", UIType: "text"})
	status := ft.create(FieldInput{
		Name: "Status", UIType: "single_select",
		Options: rawJSON(`{"choices":[{"id":"open","label":"Open","color":"green"},{"id":"blocked","label":"Blocked","color":"red"},{"id":"done","label":"Done","color":"blue"}]}`),
	})
	for _, row := range []map[string]json.RawMessage{
		{label.ID: rawJSON(`"Acme%_North"`), status.ID: rawJSON(`"Open"`)},
		{label.ID: rawJSON(`"AcmeX_North"`), status.ID: rawJSON(`"Blocked"`)},
		{label.ID: rawJSON(`"Other"`), status.ID: rawJSON(`"Done"`)},
	} {
		if _, err := f.service.CreateRows(f.ctx, owner, ft.id(), []RowInsert{{Fields: row}}); err != nil {
			t.Fatal(err)
		}
	}

	filter := &FilterNode{Op: "and", Items: []FilterNode{
		{Field: status.ID, Cmp: codec.OpIsAnyOf, Value: rawJSON(`["Open","Blocked"]`)},
		{Op: "or", Items: []FilterNode{
			{Field: status.ID, Cmp: codec.OpIs, Value: rawJSON(`"Blocked"`)},
			{Field: label.ID, Cmp: codec.OpContains, Value: rawJSON(`"%_"`)},
		}},
	}}
	result, err := f.service.QueryRows(f.ctx, owner, ft.schema.Views[0].ID,
		RowQueryRequest{Filter: filter, Limit: intPtr(1)})
	if err != nil || len(result.Rows) != 1 || result.Total == nil || *result.Total != 2 {
		t.Fatalf("nested filter result = %+v, %v", result, err)
	}
	matches, err := f.service.QueryRows(f.ctx, owner, ft.schema.Views[0].ID, RowQueryRequest{
		Filter: &FilterNode{Field: label.ID, Cmp: codec.OpContains, Value: rawJSON(`"%_"`)},
	})
	if err != nil || matches.Total == nil || *matches.Total != 1 {
		t.Fatalf("escaped wildcard result = %+v, %v", matches, err)
	}

	invalid := &FilterNode{Field: label.ID, Cmp: codec.OpEq, Value: rawJSON(`"Acme"`)}
	_, err = f.service.QueryRows(f.ctx, owner, ft.schema.Views[0].ID, RowQueryRequest{Filter: invalid})
	expectRowError(t, err, 400, "ERR_VALIDATION")
}

func TestQueryJSONAndAddressFilters(t *testing.T) {
	f := newFixture(t)
	owner := f.user("user", "active", "en")
	ft := f.fieldTable(owner)
	payload := ft.create(FieldInput{Name: "Payload", UIType: "json"})
	address := ft.create(FieldInput{Name: "Address", UIType: "inet"})
	for _, row := range []map[string]json.RawMessage{
		{
			payload.ID: rawJSON(`{"owner":"Ada","profile":{"team":"Core"}}`),
			address.ID: rawJSON(`"192.0.2.1"`),
		},
		{
			payload.ID: rawJSON(`{"owner":"Lin","profile":{"team":"Ops"}}`),
			address.ID: rawJSON(`"198.51.100.2"`),
		},
	} {
		if _, err := f.service.CreateRows(f.ctx, owner, ft.id(), []RowInsert{{Fields: row}}); err != nil {
			t.Fatal(err)
		}
	}

	for _, test := range []struct {
		name   string
		filter *FilterNode
		total  int64
	}{
		{
			name:   "json key",
			filter: &FilterNode{Field: payload.ID, Cmp: codec.OpHasKey, Value: rawJSON(`"owner"`)},
			total:  2,
		},
		{
			name: "json path",
			filter: &FilterNode{Field: payload.ID, Cmp: codec.OpPathEquals,
				Value: rawJSON(`{"path":["profile","team"],"value":"Core"}`)},
			total: 1,
		},
		{
			name: "network containment",
			filter: &FilterNode{Field: address.ID, Cmp: codec.OpContainedIn,
				Value: rawJSON(`"192.0.2.0/24"`)},
			total: 1,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := f.service.QueryRows(f.ctx, owner, ft.schema.Views[0].ID,
				RowQueryRequest{Filter: test.filter})
			if err != nil || result.Total == nil || *result.Total != test.total {
				t.Fatalf("filter result = %+v, %v", result, err)
			}
		})
	}
}

func boolPtr(value bool) *bool { return &value }
func intPtr(value int) *int    { return &value }

func TestRowsPermissionsAndFieldDDLSerialization(t *testing.T) {
	f := newFixture(t)
	owner, writer, reader, stranger, admin := f.user("user", "active", "en"), f.user("user", "active", "en"),
		f.user("user", "active", "en"), f.user("user", "active", "en"), f.user("admin", "active", "en")
	ft := f.fieldTable(owner)
	field := ft.create(FieldInput{Name: "Value", UIType: "text"})
	f.grant(acl.Table, ft.id(), "user", writer.ID, acl.Write)
	f.grant(acl.Table, ft.id(), "user", reader.ID, acl.Read)
	row, err := f.service.CreateRows(f.ctx, owner, ft.id(), []RowInsert{{Fields: map[string]json.RawMessage{
		field.ID: rawJSON(`"value"`),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.GetRow(f.ctx, reader, ft.id(), row[0].ID); err != nil {
		t.Fatalf("reader cannot read row: %v", err)
	}
	if _, err := f.service.PatchRow(f.ctx, reader, ft.id(), row[0].ID, map[string]json.RawMessage{field.ID: rawJSON(`"no"`)}); err == nil {
		t.Fatal("reader unexpectedly patched a row")
	} else {
		expectRowError(t, err, 403, "ERR_PERMISSION_DENIED")
	}
	if _, err := f.service.PatchRow(f.ctx, writer, ft.id(), row[0].ID, map[string]json.RawMessage{field.ID: rawJSON(`"writer"`)}); err != nil {
		t.Fatalf("writer cannot patch row: %v", err)
	}
	for _, user := range []Actor{stranger, admin} {
		if _, err := f.service.GetRow(f.ctx, user, ft.id(), row[0].ID); err == nil {
			t.Fatalf("ungranted %s could read a row", user.ID)
		} else {
			expectRowError(t, err, 404, "ERR_NOT_FOUND")
		}
	}

	locked := make(chan struct{})
	release := make(chan struct{})
	readDone := make(chan error, 1)
	go func() {
		readDone <- f.service.withReadableTable(f.ctx, owner, ft.id(), func(pgx.Tx, rowTable) error {
			close(locked)
			<-release
			return nil
		})
	}()
	<-locked
	ddlDone := make(chan error, 1)
	go func() {
		_, err := f.service.UpdateField(f.ctx, owner, field.ID, FieldPatch{
			Version: field.Version, Name: stringPtr("After DDL wait"),
		})
		ddlDone <- err
	}()
	select {
	case err := <-ddlDone:
		t.Fatalf("field DDL passed an active schema read lock: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if err := <-readDone; err != nil {
		t.Fatal(err)
	}
	if err := <-ddlDone; err != nil {
		t.Fatal(err)
	}
}

func stringPtr(value string) *string { return &value }

func TestRowBoundsRejectDuplicateAndNoncanonicalIDs(t *testing.T) {
	f := newFixture(t)
	owner := f.user("user", "active", "en")
	ft := f.fieldTable(owner)
	if _, err := f.service.CreateRows(f.ctx, owner, ft.id(), make([]RowInsert, maxBulkRows+1)); err == nil {
		t.Fatal("oversized bulk insert accepted")
	}
	tooManyCells := make(map[string]json.RawMessage, maxBulkCells+1)
	for index := 0; index <= maxBulkCells; index++ {
		tooManyCells[fmt.Sprintf("%08d", index)] = rawJSON(`null`)
	}
	if _, err := f.service.CreateRows(f.ctx, owner, ft.id(), []RowInsert{{Fields: tooManyCells}}); err == nil {
		t.Fatal("oversized bulk cell count accepted")
	}
	if _, err := f.service.PatchRows(f.ctx, owner, ft.id(), []RowPatch{
		{ID: "1", Fields: map[string]json.RawMessage{"00000000-0000-0000-0000-000000000000": rawJSON(`1`)}},
		{ID: "1", Fields: map[string]json.RawMessage{"00000000-0000-0000-0000-000000000000": rawJSON(`1`)}},
	}); err == nil {
		t.Fatal("duplicate row ids accepted")
	}
	if _, err := f.service.GetRow(f.ctx, owner, ft.id(), "01"); err != nil {
		var app *apperr.Error
		if !errors.As(err, &app) || app.Status != 404 || app.Code != "ERR_NOT_FOUND" {
			t.Fatalf("noncanonical path id error = %v", err)
		}
	} else {
		t.Fatal("noncanonical path id unexpectedly succeeded")
	}
}
