package structure

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/xuri/excelize/v2"

	"pandatable/internal/acl"
)

func TestImportNewTableIssuesAndHistory(t *testing.T) {
	f := newFixture(t)
	owner := f.user("user", "active", "en")
	book := f.book(owner, f.workspace(owner).ID)
	var source strings.Builder
	source.WriteString("Name,Amount\n")
	for index := range 1005 {
		fmt.Fprintf(&source, "row %d,bad-%d\n", index, index)
	}
	path := writeImportTestFile(t, "invalid.csv", []byte(source.String()))
	file, err := AnalyzeImportFile(f.ctx, path, "invalid.csv", owner)
	if err != nil {
		t.Fatal(err)
	}
	request := ImportRequest{Name: "Imported", HasHeader: true, Columns: []ImportMapping{
		{Index: 0, Name: "Name", UIType: "text"},
		{Index: 1, Name: "Amount", UIType: "integer"},
	}}
	result, err := f.service.RunImport(f.ctx, owner, ImportTarget{Kind: "book", ID: book.ID}, file, request, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Rows != 1005 || result.InvalidCells != 1005 || len(result.Issues) != 1000 ||
		result.Issues[0] != (ImportIssue{Row: 2, Column: "Amount", Value: "bad-0"}) ||
		result.TableID == "" || result.ViewID == "" {
		t.Fatalf("result = %+v", result)
	}
	schema, err := f.service.Schema(f.ctx, owner, result.TableID)
	if err != nil {
		t.Fatal(err)
	}
	if len(schema.Fields) != 7 || len(schema.Views) != 1 {
		t.Fatalf("schema = %+v", schema)
	}
	pgSchema, table := f.physicalNames(result.TableID)
	var amountColumn string
	if err := f.pool.QueryRow(f.ctx, `SELECT pg_column FROM meta.fields WHERE table_id=$1 AND name='Amount'`,
		result.TableID).Scan(&amountColumn); err != nil {
		t.Fatal(err)
	}
	qualified := pgx.Identifier{pgSchema, table}.Sanitize()
	column := pgx.Identifier{amountColumn}.Sanitize()
	if got := scanOne[int64](f, "SELECT count(*) FROM "+qualified+" WHERE "+column+" IS NULL"); got != 1005 {
		t.Errorf("NULL values = %d", got)
	}
	if got := scanOne[int64](f, `SELECT count(*) FROM meta.row_history WHERE table_id=$1 AND action='insert'`, result.TableID); got != 1005 {
		t.Errorf("row history = %d", got)
	}
}

func TestImportPackageRoundTrip(t *testing.T) {
	f := newFixture(t)
	owner := f.user("user", "active", "en")
	owner.Timezone = "UTC"
	book := f.book(owner, f.workspace(owner).ID)
	source, err := f.service.CreateTable(f.ctx, owner, book.ID, "Tasks")
	if err != nil {
		t.Fatal(err)
	}
	target, err := f.service.CreateTable(f.ctx, owner, book.ID, "Projects")
	if err != nil {
		t.Fatal(err)
	}
	targetOwner, err := f.service.CreateField(f.ctx, owner, target.Table.ID, FieldInput{Name: "Owner", UIType: "text"})
	if err != nil || targetOwner.Field == nil {
		t.Fatalf("create target owner: %v", err)
	}
	addField := func(input FieldInput) Field {
		t.Helper()
		result, err := f.service.CreateField(f.ctx, owner, source.Table.ID, input)
		if err != nil || result.Field == nil {
			t.Fatalf("create %s: %v", input.Name, err)
		}
		return *result.Field
	}
	amount := addField(FieldInput{Name: "Amount", UIType: "decimal",
		Options: json.RawMessage(`{"precision":12,"scale":2}`), IsRequired: true, Default: json.RawMessage(`7`)})
	tags := addField(FieldInput{Name: "Tags", UIType: "multi_select",
		Options: json.RawMessage(`{"choices":[{"id":"a","label":"甲"},{"id":"b","label":"乙"}]}`)})
	list := addField(FieldInput{Name: "List", UIType: "text_array"})
	when := addField(FieldInput{Name: "When", UIType: "datetime"})
	auto := addField(FieldInput{Name: "Number", UIType: "autonumber",
		Options: json.RawMessage(`{"prefix":"DOC-","padding":4}`)})
	formulaField := addField(FieldInput{Name: "Double", UIType: "formula",
		Options: json.RawMessage(`{"expr":"{Amount} * 2"}`)})
	relation, err := f.service.CreateRelation(f.ctx, owner, source.Table.ID, RelationInput{
		Name: "Project", ReverseName: "Tasks", TargetTableID: target.Table.ID, Kind: "one_many", SourceSide: "child",
	})
	if err != nil || relation.Field == nil {
		t.Fatalf("create relation: %v", err)
	}
	asJSON := func(value any) json.RawMessage {
		t.Helper()
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	extraFields := make(map[string]Field)
	extraValues := make(map[string]json.RawMessage)
	for _, item := range []struct {
		input FieldInput
		value any
	}{
		{FieldInput{Name: "Long text", UIType: "long_text"}, "first\nsecond"},
		{FieldInput{Name: "Large integer", UIType: "integer", Options: json.RawMessage(`{"size":"big"}`)}, "9007199254740993"},
		{FieldInput{Name: "Float", UIType: "float"}, 12.5},
		{FieldInput{Name: "Done", UIType: "checkbox"}, true},
		{FieldInput{Name: "Day", UIType: "date"}, "2026-03-04"},
		{FieldInput{Name: "Clock", UIType: "time"}, "12:34:56.1234"},
		{FieldInput{Name: "Duration", UIType: "duration"}, "-PT2H3M4.5S"},
		{FieldInput{Name: "Currency", UIType: "currency", Options: json.RawMessage(`{"currency":"USD","scale":2}`)}, "99.95"},
		{FieldInput{Name: "Percent", UIType: "percent"}, "0.2500"},
		{FieldInput{Name: "Rating", UIType: "rating"}, 4},
		{FieldInput{Name: "Phone", UIType: "phone"}, "+1 (212) 555-0100 x5"},
		{FieldInput{Name: "UUID", UIType: "uuid", IsUnique: true}, "00000000-0000-0000-0000-000000000001"},
		{FieldInput{Name: "JSON", UIType: "json"}, map[string]any{"owner": "Panda", "items": []any{true, 2}}},
		{FieldInput{Name: "IP", UIType: "inet"}, "2001:db8::1"},
		{FieldInput{Name: "Network", UIType: "cidr"}, "192.0.2.0/24"},
		{FieldInput{Name: "Integer list", UIType: "int_array"}, []string{"9223372036854775807", "-2"}},
		{FieldInput{Name: "Assignee", UIType: "user"}, owner.ID},
		{FieldInput{Name: "Users", UIType: "multi_user"}, []string{owner.ID}},
		{FieldInput{Name: "Email", UIType: "email"}, "panda@example.test"},
		{FieldInput{Name: "URL", UIType: "url"}, "https://example.test/path"},
		{FieldInput{Name: "Files", UIType: "attachment"}, nil},
		{FieldInput{Name: "Status", UIType: "single_select",
			Options: json.RawMessage(`{"choices":[{"id":"a","label":"Open"},{"id":"b","label":"Closed"}]}`)}, "Open"},
	} {
		field := addField(item.input)
		extraFields[item.input.Name] = field
		if item.value != nil {
			extraValues[field.ID] = asJSON(item.value)
		}
	}
	project, err := f.service.CreateRows(f.ctx, owner, target.Table.ID, []RowInsert{{
		Fields: map[string]json.RawMessage{target.Fields[0].ID: asJSON("项目, A"), targetOwner.Field.ID: asJSON("Alice")},
	}})
	if err != nil {
		t.Fatal(err)
	}
	rowValues := map[string]json.RawMessage{
		source.Fields[0].ID: asJSON("中文"), amount.ID: asJSON("12.50"), tags.ID: asJSON([]string{"甲", "乙"}),
		list.ID: asJSON([]string{"first, second", " spaced "}), when.ID: asJSON("2026-10-24T03:00:00.123456Z"),
	}
	for id, value := range extraValues {
		rowValues[id] = value
	}
	created, err := f.service.CreateRows(f.ctx, owner, source.Table.ID, []RowInsert{{Fields: rowValues}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ChangeLinks(f.ctx, owner, source.Table.ID, created[0].ID, relation.Field.ID,
		LinkChange{Add: []string{project[0].ID}}); err != nil {
		t.Fatal(err)
	}
	lookup := computedFieldFor(t, f, owner, source.Table.ID, "Project owner", "lookup", computedOptions{
		RelationFieldID: relation.Field.ID, TargetFieldID: targetOwner.Field.ID,
	})
	rollup := computedFieldFor(t, f, owner, source.Table.ID, "Project count", "rollup", computedOptions{
		RelationFieldID: relation.Field.ID, TargetFieldID: targetOwner.Field.ID, Aggregate: "count",
	})
	rollupFormula := addField(FieldInput{Name: "Doubled count", UIType: "formula",
		Options: json.RawMessage(`{"expr":"{Project count} * 2"}`)})
	chainedFormula := addField(FieldInput{Name: "Next count", UIType: "formula",
		Options: json.RawMessage(`{"expr":"{Doubled count} + 1"}`)})
	sourceSchema, err := f.service.Schema(f.ctx, owner, source.Table.ID)
	if err != nil {
		t.Fatal(err)
	}
	width := 305
	status := extraFields["Status"]
	grid, err := f.service.UpdateViewColumns(f.ctx, owner, sourceSchema.Views[0].ID, sourceSchema.Views[0].Version, ViewColumns{
		Fields: []ViewColumn{
			{ID: source.Fields[0].ID, Visible: true},
			{ID: amount.ID, Visible: true, Width: &width},
			{ID: status.ID, Visible: true},
			{ID: list.ID, Visible: false},
		},
		Frozen: 1, RowHeight: "tall",
		Sort:       []ViewSort{{Field: amount.ID, Dir: "desc"}},
		GroupBy:    []string{status.ID},
		Aggregates: []ViewAggregate{{FieldID: amount.ID, Aggregate: "sum"}},
		Filter:     json.RawMessage(fmt.Sprintf(`{"field":%q,"cmp":"gte","value":10}`, amount.ID)),
		FilterSet:  true,
		Highlight: []HighlightRule{{
			ID: "priority", When: &FilterNode{Field: status.ID, Cmp: "is", Value: json.RawMessage(`"Open"`)},
			Target: HighlightTarget{Type: "cells", Fields: []string{amount.ID}}, Color: "amber-3",
		}},
	})
	if err != nil {
		t.Fatalf("configure package grid view: %v", err)
	}
	locked := true
	lockedGrid, err := f.service.UpdateViewMetadata(f.ctx, owner, grid.ID, grid.Version, ViewPatch{Locked: &locked})
	if err != nil || !lockedGrid.Locked {
		t.Fatalf("lock package grid view = %+v, %v", lockedGrid, err)
	}
	form, err := f.service.CreateView(f.ctx, owner, source.Table.ID,
		ViewInput{Name: "Details form", Type: "form", Scope: "shared"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.service.UpdateViewForm(f.ctx, owner, form.ID, form.Version, FormConfig{
		Title: "Task form", Description: "Describe the task", AfterSubmit: "new",
		Fields: []FormField{
			{ID: source.Fields[0].ID, Label: "Title", Help: "Task title", Required: true},
			{ID: amount.ID, Label: "Amount", Help: "Estimated amount", Required: true},
		},
	})
	if err != nil {
		t.Fatalf("configure package form view: %v", err)
	}
	kanban, err := f.service.CreateView(f.ctx, owner, source.Table.ID,
		ViewInput{Name: "Task board", Type: "kanban", Scope: "shared"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.UpdateViewKanban(f.ctx, owner, kanban.ID, kanban.Version, KanbanConfig{
		GroupField: &status.ID, Fields: []string{source.Fields[0].ID, amount.ID},
	}); err != nil {
		t.Fatalf("configure package Kanban view: %v", err)
	}
	filtered, err := f.service.CreateView(f.ctx, owner, source.Table.ID,
		ViewInput{Name: "Computed filter", Type: "grid", Scope: "shared"})
	if err != nil {
		t.Fatal(err)
	}
	filtered.Config.Filter = json.RawMessage(fmt.Sprintf(
		`{"op":"and","items":[{"field":%q,"cmp":"gte","value":1},{"field":%q,"cmp":"gte","value":10}]}`,
		rollup.ID, amount.ID))
	filtered.Config.FilterSet = true
	if _, err := f.service.UpdateViewColumns(f.ctx, owner, filtered.ID, filtered.Version, filtered.Config); err != nil {
		t.Fatalf("configure computed filter: %v", err)
	}
	highlighted, err := f.service.CreateView(f.ctx, owner, source.Table.ID,
		ViewInput{Name: "Computed highlights", Type: "grid", Scope: "shared"})
	if err != nil {
		t.Fatal(err)
	}
	highlighted.Config.Highlight = []HighlightRule{{
		ID: "computed", When: &FilterNode{Field: rollup.ID, Cmp: "gte", Value: json.RawMessage(`1`)},
		Target: HighlightTarget{Type: "cells", Fields: []string{amount.ID}}, Color: "amber-3",
	}}
	highlighted.Config.Sort = []ViewSort{{Field: rollup.ID, Dir: "asc"}}
	highlighted.Config.GroupBy = []string{rollup.ID}
	if _, err := f.service.UpdateViewColumns(f.ctx, owner, highlighted.ID, highlighted.Version, highlighted.Config); err != nil {
		t.Fatalf("configure computed highlights: %v", err)
	}
	linkedView, err := f.service.CreateView(f.ctx, owner, source.Table.ID,
		ViewInput{Name: "Linked filter", Type: "grid", Scope: "shared"})
	if err != nil {
		t.Fatal(err)
	}
	linkedView.Config.Filter = json.RawMessage(fmt.Sprintf(
		`{"field":%q,"cmp":"has_any_of","value":[%q]}`, relation.Field.ID, project[0].ID))
	linkedView.Config.FilterSet = true
	if _, err := f.service.UpdateViewColumns(f.ctx, owner, linkedView.ID, linkedView.Version, linkedView.Config); err != nil {
		t.Fatalf("configure linked filter: %v", err)
	}
	if _, err := f.service.CreateView(f.ctx, owner, source.Table.ID,
		ViewInput{Name: "Private view", Type: "grid", Scope: "personal"}); err != nil {
		t.Fatal(err)
	}
	sourceSchema, err = f.service.Schema(f.ctx, owner, source.Table.ID)
	if err != nil {
		t.Fatal(err)
	}
	original, err := f.service.GetRow(f.ctx, owner, source.Table.ID, created[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	var data bytes.Buffer
	if err := f.service.Export(f.ctx, owner, source.Views[0].ID, ExportRequest{Format: "xlsx"}, "test",
		func(_ ExportInfo, write func(io.Writer) error) error { return write(&data) }); err != nil {
		t.Fatal(err)
	}
	path := writeImportTestFile(t, "package.xlsx", data.Bytes())
	replacePackageCell := func(t *testing.T, fieldName string, value any) string {
		t.Helper()
		file, err := AnalyzeImportFile(f.ctx, path, "package.xlsx", owner)
		if err != nil {
			t.Fatal(err)
		}
		index := -1
		for _, column := range file.Analysis.Columns {
			if column.Name == fieldName {
				index = column.Index
			}
		}
		if index < 0 {
			t.Fatalf("package field %q not found", fieldName)
		}
		spreadsheet, err := excelize.OpenReader(bytes.NewReader(data.Bytes()))
		if err != nil {
			t.Fatal(err)
		}
		cell, err := excelize.CoordinatesToCellName(index+1, 2)
		if err != nil {
			t.Fatal(err)
		}
		if err := spreadsheet.SetCellValue("Data", cell, value); err != nil {
			t.Fatal(err)
		}
		var damaged bytes.Buffer
		if err := spreadsheet.Write(&damaged); err != nil {
			t.Fatal(err)
		}
		if err := spreadsheet.Close(); err != nil {
			t.Fatal(err)
		}
		return writeImportTestFile(t, "modified-package.xlsx", damaged.Bytes())
	}
	for _, relink := range []bool{false, true} {
		t.Run(fmt.Sprintf("relink-%t", relink), func(t *testing.T) {
			file, err := AnalyzeImportFile(f.ctx, path, "package.xlsx", owner)
			if err != nil {
				t.Fatal(err)
			}
			targetBook := ImportTarget{Kind: "book", ID: book.ID}
			if err := f.service.PrepareImport(f.ctx, owner, targetBook, file); err != nil {
				t.Fatal(err)
			}
			request := ImportRequest{Name: fmt.Sprintf("Imported %t", relink), HasHeader: true}
			for _, column := range file.Analysis.Columns {
				mapping := ImportMapping{Index: column.Index}
				if column.UIType == "link" && relink {
					if column.RelinkTargetID != target.Table.ID {
						t.Fatalf("link target = %+v", column)
					}
					mapping.Relink = true
				}
				request.Columns = append(request.Columns, mapping)
			}
			result, err := f.service.RunImport(f.ctx, owner, targetBook, file, request, nil)
			if err != nil {
				t.Fatalf("run package import: %#v", err)
			}
			if result.Rows != 1 || result.InvalidCells != 0 || result.ViewID == "" {
				t.Fatalf("result = %+v", result)
			}
			if relink && len(result.ConvertedLinks) != 0 || !relink && !reflect.DeepEqual(result.ConvertedLinks, []string{"Project"}) {
				t.Fatalf("converted links = %v", result.ConvertedLinks)
			}
			wantViews := []string{"Computed filter", "Computed highlights", "Linked filter"}
			if relink && len(result.UnavailableViews) != 0 || !relink && !reflect.DeepEqual(result.UnavailableViews, wantViews) {
				t.Fatalf("views requiring repair = %v", result.UnavailableViews)
			}
			schema, err := f.service.Schema(f.ctx, owner, result.TableID)
			if err != nil {
				t.Fatal(err)
			}
			byName := make(map[string]Field)
			for _, field := range schema.Fields {
				byName[field.Name] = field
			}
			row, err := f.service.GetRow(f.ctx, owner, result.TableID, created[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			sourceFields := map[string]Field{
				"Amount": amount, "Tags": tags, "List": list, "When": when, "Number": auto, "Double": formulaField,
				source.Fields[0].Name: source.Fields[0],
			}
			for name, field := range extraFields {
				sourceFields[name] = field
			}
			for name, sourceField := range sourceFields {
				field := byName[name]
				optionsMatch := jsonEqual(field.Options, sourceField.Options)
				if name == "Double" {
					var formula formulaOptions
					optionsMatch = json.Unmarshal(field.Options, &formula) == nil &&
						formula.Expression == "{Amount} * 2" &&
						reflect.DeepEqual(formula.FieldIDs, []string{byName["Amount"].ID})
				}
				if field.UIType != sourceField.UIType || field.IsRequired != sourceField.IsRequired ||
					field.IsUnique != sourceField.IsUnique || !optionsMatch || !jsonEqual(field.Default, sourceField.Default) {
					t.Errorf("%s metadata = %+v, original %+v", name, field, sourceField)
				}
				if !jsonEqual(row.Fields[field.ID], original.Fields[sourceField.ID]) {
					t.Errorf("%s value = %s, original %s", name, row.Fields[field.ID], original.Fields[sourceField.ID])
				}
			}
			sourceKeys, importedKeys := make(map[string]string), make(map[string]string)
			for _, field := range sourceSchema.Fields {
				sourceKeys[field.ID] = field.Name
				if field.IsSystem {
					imported := byName[field.Name]
					if imported.UIType != field.UIType || !jsonEqual(row.Fields[imported.ID], original.Fields[field.ID]) {
						t.Errorf("system field %s = %s, original %s", field.Name, row.Fields[imported.ID], original.Fields[field.ID])
					}
				}
			}
			for _, field := range schema.Fields {
				importedKeys[field.ID] = field.Name
			}
			importedViews := make(map[string]View)
			for _, view := range schema.Views {
				importedViews[view.Name] = view
			}
			sharedCount := 0
			for _, sourceView := range sourceSchema.Views {
				if sourceView.Scope != "shared" {
					continue
				}
				sharedCount++
				imported, ok := importedViews[sourceView.Name]
				if !ok || imported.Type != sourceView.Type || imported.Scope != sourceView.Scope {
					t.Errorf("view %s = %+v, original %+v", sourceView.Name, imported, sourceView)
					continue
				}
				if imported.Locked {
					t.Errorf("legacy package unexpectedly restored a lock on view %s", sourceView.Name)
				}
				expected, err := packageViewConfig(sourceView.Config, sourceKeys)
				if err != nil {
					t.Fatal(err)
				}
				actual, err := packageViewConfig(imported.Config, importedKeys)
				if err != nil {
					t.Fatal(err)
				}
				if !jsonEqual(asJSON(actual), asJSON(expected)) {
					t.Errorf("view %s config = %+v, original %+v", sourceView.Name, actual, expected)
				}
			}
			if len(schema.Views) != sharedCount {
				t.Errorf("restored views = %d, shared source views = %d", len(schema.Views), sharedCount)
			}
			if !relink {
				for _, name := range wantViews {
					view := importedViews[name]
					var filter *FilterNode
					if len(view.Config.Filter) > 0 && string(view.Config.Filter) != "null" {
						filter = &FilterNode{}
						if err := json.Unmarshal(view.Config.Filter, filter); err != nil {
							t.Fatal(err)
						}
					}
					if _, err := f.service.QueryRows(f.ctx, owner, view.ID, RowQueryRequest{Filter: filter}); err == nil {
						t.Errorf("unrepaired view %s unexpectedly queried", name)
					}
					view.Config.Filter = json.RawMessage(`null`)
					view.Config.FilterSet = true
					view.Config.Highlight = []HighlightRule{}
					view.Config.Sort = []ViewSort{}
					view.Config.GroupBy = []string{}
					repaired, err := f.service.UpdateViewColumns(f.ctx, owner, view.ID, view.Version, view.Config)
					if err != nil {
						t.Fatalf("repair view %s: %#v", name, err)
					}
					if _, err := f.service.QueryRows(f.ctx, owner, repaired.ID, RowQueryRequest{}); err != nil {
						t.Fatalf("query repaired view %s: %#v", name, err)
					}
				}
			}
			inserted, err := f.service.CreateRows(f.ctx, owner, result.TableID, []RowInsert{{
				Fields: map[string]json.RawMessage{byName["UUID"].ID: asJSON("00000000-0000-0000-0000-000000000002")},
			}})
			if err != nil || len(inserted) != 1 {
				t.Fatalf("insert into restored table = %+v, %v", inserted, err)
			}
			if !jsonEqual(inserted[0].Fields[byName["Amount"].ID], byName["Amount"].Default) {
				t.Errorf("restored default = %s, expected %s", inserted[0].Fields[byName["Amount"].ID], byName["Amount"].Default)
			}
			if string(inserted[0].Fields[byName["Number"].ID]) != `"2"` {
				t.Errorf("restored sequence = %s", inserted[0].Fields[byName["Number"].ID])
			}
			_, err = f.service.CreateRows(f.ctx, owner, result.TableID, []RowInsert{{
				Fields: map[string]json.RawMessage{byName["UUID"].ID: original.Fields[extraFields["UUID"].ID]},
			}})
			expectError(t, err, 409, "ERR_UNIQUE_CONFLICT")
			if relink {
				if !jsonEqual(row.Fields[byName["Project"].ID], original.Fields[relation.Field.ID]) {
					t.Errorf("link = %s, original %s", row.Fields[byName["Project"].ID], original.Fields[relation.Field.ID])
				}
			} else if string(row.Fields[byName["Project"].ID]) != `"项目, A"` {
				t.Errorf("text link = %s", row.Fields[byName["Project"].ID])
			}
			for _, sourceField := range []Field{lookup, rollup} {
				imported := byName[sourceField.Name]
				var options computedOptions
				if imported.UIType != sourceField.UIType || json.Unmarshal(imported.Options, &options) != nil ||
					options.RelationFieldID != byName["Project"].ID {
					t.Errorf("computed metadata = %+v", imported)
				}
				if relink && (!jsonEqual(row.Fields[imported.ID], original.Fields[sourceField.ID]) ||
					options.TargetFieldID != targetOwner.Field.ID) {
					t.Errorf("computed %s = %s, original %s", sourceField.Name, row.Fields[imported.ID], original.Fields[sourceField.ID])
				}
				if !relink && options.Error != "dependency_missing" {
					t.Errorf("unlinked computed options = %+v", options)
				}
			}
			for _, test := range []struct {
				field      Field
				dependency string
				expression string
			}{
				{rollupFormula, "Project count", "{Project count} * 2"},
				{chainedFormula, "Doubled count", "{Doubled count} + 1"},
			} {
				imported := byName[test.field.Name]
				var options formulaOptions
				if imported.UIType != "formula" || json.Unmarshal(imported.Options, &options) != nil ||
					options.Expression != test.expression || !reflect.DeepEqual(options.FieldIDs, []string{byName[test.dependency].ID}) {
					t.Errorf("dependent formula metadata = %+v", imported)
				}
				if relink {
					if options.Error != "" || !jsonEqual(row.Fields[imported.ID], original.Fields[test.field.ID]) {
						t.Errorf("relinked formula %s = %s, original %s", test.field.Name, row.Fields[imported.ID], original.Fields[test.field.ID])
					}
				} else if options.Error != "dependency_missing" || string(row.Fields[imported.ID]) != `"#ERROR"` {
					t.Errorf("unlinked formula %s = %s, options %+v", test.field.Name, row.Fields[imported.ID], options)
				}
			}
			if !relink {
				for _, test := range []struct {
					name       string
					expression string
				}{
					{"Doubled count", "{Amount} * 2"},
					{"Next count", "{Doubled count} + 1"},
				} {
					field := byName[test.name]
					if _, err := f.service.UpdateField(f.ctx, owner, field.ID,
						FieldPatch{Version: field.Version, Options: asJSON(map[string]string{"expr": test.expression})}); err != nil {
						t.Fatalf("repair formula %s: %v", test.name, err)
					}
				}
				repaired, err := f.service.GetRow(f.ctx, owner, result.TableID, created[0].ID)
				if err != nil || !jsonEqual(repaired.Fields[byName["Next count"].ID], asJSON(26)) {
					t.Fatalf("repaired formula = %s, %v", repaired.Fields[byName["Next count"].ID], err)
				}
			}
		})
	}

	t.Run("package append pre-maps compatible fields without restoring views", func(t *testing.T) {
		destination, err := f.service.CreateTable(f.ctx, owner, book.ID, "Append package")
		if err != nil {
			t.Fatal(err)
		}
		mappedFields := make(map[string]Field)
		for _, input := range []FieldInput{
			{Name: "Amount", UIType: "decimal", Options: json.RawMessage(`{"precision":12,"scale":2}`)},
			{Name: "When", UIType: "datetime"},
			{Name: "Tags", UIType: "multi_select",
				Options: json.RawMessage(`{"choices":[{"id":"a","label":"甲"},{"id":"b","label":"乙"}]}`)},
		} {
			created, err := f.service.CreateField(f.ctx, owner, destination.Table.ID, input)
			if err != nil || created.Field == nil {
				t.Fatalf("create append field %s: %v", input.Name, err)
			}
			mappedFields[input.Name] = *created.Field
		}
		before, err := f.service.Schema(f.ctx, owner, destination.Table.ID)
		if err != nil {
			t.Fatal(err)
		}
		file, err := AnalyzeImportFile(f.ctx, path, "package.xlsx", owner)
		if err != nil {
			t.Fatal(err)
		}
		appendTarget := ImportTarget{Kind: "table", ID: destination.Table.ID}
		if err := f.service.PrepareImport(f.ctx, owner, appendTarget, file); err != nil {
			t.Fatal(err)
		}
		request := ImportRequest{HasHeader: true}
		for _, column := range file.Analysis.Columns {
			field, mapped := mappedFields[column.Name]
			mapping := ImportMapping{Index: column.Index, Skip: !mapped}
			if mapped {
				if column.TargetFieldID != field.ID {
					t.Fatalf("append suggestion for %s = %q, want %q", column.Name, column.TargetFieldID, field.ID)
				}
				mapping.TargetFieldID = column.TargetFieldID
			}
			request.Columns = append(request.Columns, mapping)
		}
		reader := f.user("user", "active", "en")
		f.grant(acl.Table, destination.Table.ID, "user", reader.ID, acl.Read)
		if _, err := f.service.RunImport(f.ctx, reader, appendTarget, file, request, nil); err == nil {
			t.Fatal("read-only user appended a package")
		} else {
			expectError(t, err, 403, "ERR_PERMISSION_DENIED")
		}
		result, err := f.service.RunImport(f.ctx, owner, appendTarget, file, request, nil)
		if err != nil || result.Rows != 1 || result.InvalidCells != 0 {
			t.Fatalf("package append result = %+v, %v", result, err)
		}
		after, err := f.service.Schema(f.ctx, owner, destination.Table.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(before.Views, after.Views) || len(before.Fields) != len(after.Fields) {
			t.Fatal("appending the package changed destination fields or views")
		}
		row, err := f.service.GetRow(f.ctx, owner, destination.Table.ID, "1")
		if err != nil {
			t.Fatal(err)
		}
		for name, sourceField := range map[string]Field{"Amount": amount, "When": when, "Tags": tags} {
			if !jsonEqual(row.Fields[mappedFields[name].ID], original.Fields[sourceField.ID]) {
				t.Errorf("appended %s = %s, original %s", name, row.Fields[mappedFields[name].ID], original.Fields[sourceField.ID])
			}
		}
	})

	t.Run("attachment names are reported instead of guessing file identities", func(t *testing.T) {
		modifiedPath := replacePackageCell(t, "Files", "report.pdf")
		file, err := AnalyzeImportFile(f.ctx, modifiedPath, "package.xlsx", owner)
		if err != nil {
			t.Fatal(err)
		}
		request := ImportRequest{Name: "Attachments", HasHeader: true}
		for _, column := range file.Analysis.Columns {
			request.Columns = append(request.Columns, ImportMapping{Index: column.Index})
		}
		result, err := f.service.RunImport(f.ctx, owner, ImportTarget{Kind: "book", ID: book.ID}, file, request, nil)
		if err != nil || result.Rows != 1 || result.InvalidCells != 1 || len(result.Issues) != 1 ||
			result.Issues[0] != (ImportIssue{Row: 2, Column: "Files", Value: "report.pdf"}) {
			t.Fatalf("attachment result = %+v, %#v", result, err)
		}
		schema, err := f.service.Schema(f.ctx, owner, result.TableID)
		if err != nil {
			t.Fatal(err)
		}
		var attachmentID string
		for _, field := range schema.Fields {
			if field.Name == "Files" {
				attachmentID = field.ID
			}
		}
		row, err := f.service.GetRow(f.ctx, owner, result.TableID, created[0].ID)
		if err != nil || string(row.Fields[attachmentID]) != "null" {
			t.Fatalf("imported attachment value = %s, %v", row.Fields[attachmentID], err)
		}
	})

	t.Run("ambiguous link values become invalid instead of choosing a row", func(t *testing.T) {
		if _, err := f.service.CreateRows(f.ctx, owner, target.Table.ID, []RowInsert{{
			Fields: map[string]json.RawMessage{target.Fields[0].ID: asJSON("项目, A")},
		}}); err != nil {
			t.Fatal(err)
		}
		file, err := AnalyzeImportFile(f.ctx, path, "package.xlsx", owner)
		if err != nil {
			t.Fatal(err)
		}
		importTarget := ImportTarget{Kind: "book", ID: book.ID}
		if err := f.service.PrepareImport(f.ctx, owner, importTarget, file); err != nil {
			t.Fatal(err)
		}
		request := ImportRequest{Name: "Ambiguous", HasHeader: true}
		for _, column := range file.Analysis.Columns {
			request.Columns = append(request.Columns, ImportMapping{Index: column.Index, Relink: column.UIType == "link"})
		}
		result, err := f.service.RunImport(f.ctx, owner, importTarget, file, request, nil)
		if err != nil || result.Rows != 1 || result.InvalidCells != 1 || len(result.Issues) != 1 ||
			result.Issues[0].Column != "Project" {
			t.Fatalf("ambiguous result = %+v, %v", result, err)
		}
	})

	t.Run("invalid required package values roll back table and rows", func(t *testing.T) {
		damagedPath := replacePackageCell(t, "Amount", "not-a-number")
		file, err := AnalyzeImportFile(f.ctx, damagedPath, "invalid-package.xlsx", owner)
		if err != nil {
			t.Fatal(err)
		}
		request := ImportRequest{Name: "Must Roll Back", HasHeader: true}
		for _, column := range file.Analysis.Columns {
			request.Columns = append(request.Columns, ImportMapping{Index: column.Index})
		}
		result, err := f.service.RunImport(f.ctx, owner, ImportTarget{Kind: "book", ID: book.ID}, file, request, nil)
		if err == nil || result.TableID != "" || result.Rows != 0 {
			t.Fatalf("damaged package result = %+v, %v", result, err)
		}
		if count := scanOne[int64](f, `SELECT count(*) FROM meta.tables WHERE book_id=$1 AND name=$2`,
			book.ID, "Must Roll Back"); count != 0 {
			t.Fatalf("rolled-back tables = %d", count)
		}
	})
}

func TestImportAppendCommitsOnlyCompletedBatches(t *testing.T) {
	f := newFixture(t)
	owner := f.user("user", "active", "en")
	book := f.book(owner, f.workspace(owner).ID)
	table, err := f.service.CreateTable(f.ctx, owner, book.ID, "Append batches")
	if err != nil {
		t.Fatal(err)
	}
	flag, err := f.service.CreateField(f.ctx, owner, table.Table.ID, FieldInput{Name: "Flag", UIType: "checkbox"})
	if err != nil || flag.Field == nil {
		t.Fatalf("create flag: %v", err)
	}
	var source strings.Builder
	source.WriteString("Name,Flag\n")
	for index := range 5000 {
		fmt.Fprintf(&source, "row-%d,true\n", index)
	}
	source.WriteString("late,not-a-checkbox\n")
	path := writeImportTestFile(t, "append.csv", []byte(source.String()))
	file, err := AnalyzeImportFile(f.ctx, path, "append.csv", owner)
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.service.RunImport(f.ctx, owner, ImportTarget{Kind: "table", ID: table.Table.ID}, file,
		ImportRequest{HasHeader: true, Columns: []ImportMapping{
			{Index: 0, TargetFieldID: table.Fields[0].ID},
			{Index: 1, TargetFieldID: flag.Field.ID},
		}}, nil)
	if err == nil || result.Rows != 5000 || result.InvalidCells != 1 || len(result.Issues) != 1 {
		t.Fatalf("partial result = %+v, %v", result, err)
	}
	pgSchema, pgTable := f.physicalNames(table.Table.ID)
	if count := scanOne[int64](f, "SELECT count(*) FROM "+pgx.Identifier{pgSchema, pgTable}.Sanitize()); count != 5000 {
		t.Fatalf("committed rows = %d", count)
	}
}

func TestImportMissingUserIsAnInvalidCell(t *testing.T) {
	f := newFixture(t)
	owner := f.user("user", "active", "en")
	book := f.book(owner, f.workspace(owner).ID)
	const unknown = "00000000-0000-0000-0000-000000000001"
	path := writeImportTestFile(t, "users.csv",
		[]byte("Name,Assignee\nTask,"+unknown+"\nAssigned,"+owner.ID+"\n"))
	file, err := AnalyzeImportFile(f.ctx, path, "users.csv", owner)
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.service.RunImport(f.ctx, owner, ImportTarget{Kind: "book", ID: book.ID}, file,
		ImportRequest{Name: "User import", HasHeader: true, Columns: []ImportMapping{
			{Index: 0, Name: "Name", UIType: "text"},
			{Index: 1, Name: "Assignee", UIType: "user"},
		}}, nil)
	if err != nil || result.Rows != 2 || result.InvalidCells != 1 || len(result.Issues) != 1 ||
		result.Issues[0] != (ImportIssue{Row: 2, Column: "Assignee", Value: unknown}) {
		t.Fatalf("missing user result = %+v, %v", result, err)
	}
	schema, err := f.service.Schema(f.ctx, owner, result.TableID)
	if err != nil {
		t.Fatal(err)
	}
	var assignee string
	for _, field := range schema.Fields {
		if field.Name == "Assignee" {
			assignee = field.ID
		}
	}
	row, err := f.service.GetRow(f.ctx, owner, result.TableID, "1")
	if err != nil || string(row.Fields[assignee]) != "null" {
		t.Fatalf("imported user value = %s, %v", row.Fields[assignee], err)
	}
	row, err = f.service.GetRow(f.ctx, owner, result.TableID, "2")
	if err != nil || string(row.Fields[assignee]) != `"`+owner.ID+`"` {
		t.Fatalf("existing user value = %s, %v", row.Fields[assignee], err)
	}
}

func TestImportMissingRequiredUserFails(t *testing.T) {
	f := newFixture(t)
	owner := f.user("user", "active", "en")
	table := f.table(owner, f.book(owner, f.workspace(owner).ID).ID)
	field, err := f.service.CreateField(f.ctx, owner, table.Table.ID,
		FieldInput{Name: "Assignee", UIType: "user", IsRequired: true})
	if err != nil || field.Field == nil {
		t.Fatalf("create required user field: %v", err)
	}
	path := writeImportTestFile(t, "users.csv",
		[]byte("Name,Assignee\nTask,00000000-0000-0000-0000-000000000001\n"))
	file, err := AnalyzeImportFile(f.ctx, path, "users.csv", owner)
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.service.RunImport(f.ctx, owner, ImportTarget{Kind: "table", ID: table.Table.ID}, file,
		ImportRequest{HasHeader: true, Columns: []ImportMapping{
			{Index: 0, TargetFieldID: table.Fields[0].ID},
			{Index: 1, TargetFieldID: field.Field.ID},
		}}, nil)
	if err == nil || result.Rows != 0 || result.InvalidCells != 1 {
		t.Fatalf("required user result = %+v, %v", result, err)
	}
	expectError(t, err, 400, "ERR_REQUIRED_FIELD")
}

func TestImportReportsFirstInvalidCellsInSourceOrder(t *testing.T) {
	f := newFixture(t)
	owner := f.user("user", "active", "en")
	book := f.book(owner, f.workspace(owner).ID)
	const unknown = "00000000-0000-0000-0000-000000000001"
	var source strings.Builder
	source.WriteString("Name,Assignee,Amount\nfirst," + unknown + ",bad-0\n")
	for index := 1; index <= 1005; index++ {
		fmt.Fprintf(&source, "row-%d,,bad-%d\n", index, index)
	}
	path := writeImportTestFile(t, "ordered-issues.csv", []byte(source.String()))
	file, err := AnalyzeImportFile(f.ctx, path, "ordered-issues.csv", owner)
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.service.RunImport(f.ctx, owner, ImportTarget{Kind: "book", ID: book.ID}, file,
		ImportRequest{Name: "Ordered issues", HasHeader: true, Columns: []ImportMapping{
			{Index: 0, Name: "Name", UIType: "text"},
			{Index: 1, Name: "Assignee", UIType: "user"},
			{Index: 2, Name: "Amount", UIType: "integer"},
		}}, nil)
	if err != nil || result.Rows != 1006 || result.InvalidCells != 1007 || len(result.Issues) != 1000 {
		t.Fatalf("ordered result = %+v, %v", result, err)
	}
	if result.Issues[0] != (ImportIssue{Row: 2, Column: "Assignee", Value: unknown}) ||
		result.Issues[1] != (ImportIssue{Row: 2, Column: "Amount", Value: "bad-0"}) ||
		result.Issues[999] != (ImportIssue{Row: 1000, Column: "Amount", Value: "bad-998"}) {
		t.Fatalf("first issues = %+v, %+v; last = %+v", result.Issues[0], result.Issues[1], result.Issues[999])
	}
}

func jsonEqual(a, b json.RawMessage) bool {
	var first, second any
	return json.Unmarshal(a, &first) == nil && json.Unmarshal(b, &second) == nil && reflect.DeepEqual(first, second)
}

func TestImportAppendPermissionAndMappings(t *testing.T) {
	f := newFixture(t)
	owner := f.user("user", "active", "en")
	writer := f.user("user", "active", "en")
	reader := f.user("user", "active", "en")
	table := f.table(owner, f.book(owner, f.workspace(owner).ID).ID)
	f.grant(acl.Table, table.Table.ID, "user", writer.ID, acl.Write)
	f.grant(acl.Table, table.Table.ID, "user", reader.ID, acl.Read)
	name := table.Fields[0]
	path := writeImportTestFile(t, "append.csv", []byte("Source,Ignored\none,x\ntwo,y\n"))
	file, err := AnalyzeImportFile(f.ctx, path, "append.csv", owner)
	if err != nil {
		t.Fatal(err)
	}
	request := ImportRequest{HasHeader: true, Columns: []ImportMapping{
		{Index: 0, TargetFieldID: name.ID}, {Index: 1, Skip: true},
	}}
	target := ImportTarget{Kind: "table", ID: table.Table.ID}
	if _, err := f.service.RunImport(f.ctx, reader, target, file, request, nil); err == nil {
		t.Fatal("read-only import unexpectedly succeeded")
	} else {
		expectError(t, err, 403, "ERR_PERMISSION_DENIED")
	}
	result, err := f.service.RunImport(f.ctx, writer, target, file, request, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Rows != 2 || result.InvalidCells != 0 {
		t.Fatalf("result = %+v", result)
	}
	pgSchema, pgTable := f.physicalNames(table.Table.ID)
	if got := scanOne[int64](f, "SELECT count(*) FROM "+pgx.Identifier{pgSchema, pgTable}.Sanitize()); got != 2 {
		t.Fatalf("rows = %d", got)
	}
}
