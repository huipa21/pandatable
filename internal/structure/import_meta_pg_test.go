package structure

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"pandatable/internal/acl"
	"pandatable/internal/attachments"
	"pandatable/internal/codec"
)

func exportImportTestPackage(t *testing.T, f *fixture, owner Actor, tableID string) []byte {
	t.Helper()
	schema, err := f.service.Schema(f.ctx, owner, tableID)
	if err != nil {
		t.Fatal(err)
	}
	var data bytes.Buffer
	if err := f.service.Export(f.ctx, owner, schema.Views[0].ID, ExportRequest{Format: "xlsx"}, "test",
		func(_ ExportInfo, write func(io.Writer) error) error { return write(&data) }); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

// editImportWorkbook applies edit to an exported workbook and returns the new
// file path.
func editImportWorkbook(t *testing.T, data []byte, edit func(*excelize.File)) string {
	t.Helper()
	book, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	edit(book)
	var edited bytes.Buffer
	if err := book.Write(&edited); err != nil {
		t.Fatal(err)
	}
	if err := book.Close(); err != nil {
		t.Fatal(err)
	}
	return writeImportTestFile(t, "edited.xlsx", edited.Bytes())
}

// setImportMetaCell rewrites one _meta field-row cell, locating the row by
// field name and the column by header.
func setImportMetaCell(t *testing.T, book *excelize.File, fieldName, header string, value any) {
	t.Helper()
	column := -1
	for index, name := range XLSXMetaColumns() {
		if name == header {
			column = index + 1
		}
	}
	rows, err := book.GetRows(XLSXMetaSheet)
	if err != nil {
		t.Fatal(err)
	}
	for index := XLSXMetaFirstFieldRow - 1; index < len(rows); index++ {
		if len(rows[index]) > 2 && rows[index][2] == fieldName {
			cell, err := excelize.CoordinatesToCellName(column, index+1)
			if err != nil {
				t.Fatal(err)
			}
			if err := book.SetCellValue(XLSXMetaSheet, cell, value); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatalf("_meta row %q not found", fieldName)
}

func importAllColumns(t *testing.T, f *fixture, actor Actor, target ImportTarget, file *ImportFile, name string) ImportResult {
	t.Helper()
	if err := f.service.PrepareImport(f.ctx, actor, target, file); err != nil {
		t.Fatal(err)
	}
	request := ImportRequest{Name: name, HasHeader: true}
	for _, column := range file.Analysis.Columns {
		request.Columns = append(request.Columns, ImportMapping{Index: column.Index,
			Relink: column.UIType == "link" && column.RelinkTargetID != ""})
	}
	result, err := f.service.RunImport(f.ctx, actor, target, file, request, nil)
	if err != nil {
		t.Fatalf("run import %s: %v", name, err)
	}
	return result
}

func importFieldsByName(t *testing.T, f *fixture, actor Actor, tableID string) map[string]Field {
	t.Helper()
	schema, err := f.service.Schema(f.ctx, actor, tableID)
	if err != nil {
		t.Fatal(err)
	}
	fields := make(map[string]Field, len(schema.Fields))
	for _, field := range schema.Fields {
		fields[field.Name] = field
	}
	return fields
}

func hasImportNote(notes []ImportIssue, reason string) bool {
	for _, note := range notes {
		if note.Row == 0 && note.Reason == reason {
			return true
		}
	}
	return false
}

// GP-07.1: an exported v2 workbook re-imports with every field type, its
// options and its relation target restored from _meta.
func TestImportXLSXMetaRoundTripRestoresTypesAndRelations(t *testing.T) {
	f := newFixture(t)
	owner := f.user("user", "active", "en")
	owner.Timezone = "UTC"
	book := f.book(owner, f.workspace(owner).ID)
	source, err := f.service.CreateTable(f.ctx, owner, book.ID, "Tasks")
	if err != nil {
		t.Fatal(err)
	}
	projects, err := f.service.CreateTable(f.ctx, owner, book.ID, "Projects")
	if err != nil {
		t.Fatal(err)
	}
	inputs := []FieldInput{
		{Name: "Status", UIType: "single_select", Options: json.RawMessage(
			`{"choices":[{"id":"a","label":"Open","color":"green"},{"id":"b","label":"Closed","color":"red"}]}`)},
		{Name: "Tags", UIType: "multi_select", Options: json.RawMessage(
			`{"choices":[{"id":"x","label":"甲","color":"purple"},{"id":"y","label":"乙","color":"teal"}]}`)},
		{Name: "Amount", UIType: "decimal", Options: json.RawMessage(`{"precision":10,"scale":3}`), IsRequired: true,
			Default: json.RawMessage(`1`)},
		{Name: "Price", UIType: "currency", Options: json.RawMessage(`{"currency":"EUR","scale":2}`)},
		{Name: "Score", UIType: "rating"},
		{Name: "Day", UIType: "date"},
		{Name: "Notes", UIType: "long_text"},
		{Name: "Code", UIType: "uuid", IsUnique: true},
		{Name: "Files", UIType: "attachment"},
	}
	created := make(map[string]Field, len(inputs))
	for _, input := range inputs {
		result, err := f.service.CreateField(f.ctx, owner, source.Table.ID, input)
		if err != nil || result.Field == nil {
			t.Fatalf("create %s: %v", input.Name, err)
		}
		created[input.Name] = *result.Field
	}
	relation, err := f.service.CreateRelation(f.ctx, owner, source.Table.ID, RelationInput{
		Name: "Project", ReverseName: "Tasks", TargetTableID: projects.Table.ID, Kind: "one_many", SourceSide: "child",
	})
	if err != nil || relation.Field == nil {
		t.Fatalf("create relation: %v", err)
	}
	project, err := f.service.CreateRows(f.ctx, owner, projects.Table.ID, []RowInsert{{
		Fields: map[string]json.RawMessage{projects.Fields[0].ID: json.RawMessage(`"Apollo"`)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := f.service.CreateRows(f.ctx, owner, source.Table.ID, []RowInsert{{Fields: map[string]json.RawMessage{
		source.Fields[0].ID: json.RawMessage(`"Launch"`), created["Status"].ID: json.RawMessage(`"Closed"`),
		created["Tags"].ID: json.RawMessage(`["乙"]`), created["Amount"].ID: json.RawMessage(`"3.125"`),
		created["Score"].ID: json.RawMessage(`4`), created["Day"].ID: json.RawMessage(`"2026-03-04"`),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ChangeLinks(f.ctx, owner, source.Table.ID, rows[0].ID, relation.Field.ID,
		LinkChange{Add: []string{project[0].ID}}); err != nil {
		t.Fatal(err)
	}
	// A second table with the same name must not capture the relink: the
	// target is resolved by relation_target_table_id first.
	renamed := "Programs"
	if _, err := f.service.UpdateTable(f.ctx, owner, projects.Table.ID, TablePatch{Name: &renamed}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.CreateTable(f.ctx, owner, book.ID, "Projects"); err != nil {
		t.Fatal(err)
	}
	data := exportImportTestPackage(t, f, owner, source.Table.ID)
	original := importFieldsByName(t, f, owner, source.Table.ID)
	original["Project"] = *relation.Field
	assertRestored := func(t *testing.T, tableID string) {
		t.Helper()
		imported := importFieldsByName(t, f, owner, tableID)
		for name, want := range original {
			got, ok := imported[name]
			if !ok {
				t.Errorf("imported field %q is missing", name)
				continue
			}
			if got.UIType != want.UIType || got.IsRequired != want.IsRequired || got.IsUnique != want.IsUnique {
				t.Errorf("imported %s = %s required=%t unique=%t, want %s required=%t unique=%t", name,
					got.UIType, got.IsRequired, got.IsUnique, want.UIType, want.IsRequired, want.IsUnique)
			}
			if want.UIType != "link" && !jsonEqual(got.Options, want.Options) {
				t.Errorf("imported %s options = %s, want %s", name, got.Options, want.Options)
			}
		}
		row, err := f.service.GetRow(f.ctx, owner, tableID, rows[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		if link := string(row.Fields[imported["Project"].ID]); !strings.Contains(link, project[0].ID) {
			t.Errorf("imported link = %s, want row %s", link, project[0].ID)
		}
		if status := string(row.Fields[imported["Status"].ID]); status != `"Closed"` {
			t.Errorf("imported status = %s", status)
		}
	}

	t.Run("manifest and meta", func(t *testing.T) {
		file, err := AnalyzeImportFile(f.ctx, writeImportTestFile(t, "package.xlsx", data), "package.xlsx", owner)
		if err != nil {
			t.Fatal(err)
		}
		if file.Analysis.Format != "package" || file.Analysis.MetaVersion != "v2" || len(file.Analysis.Notes) != 0 {
			t.Fatalf("analysis = %+v", file.Analysis)
		}
		for _, column := range file.Analysis.Columns {
			if column.TypeSource != "meta" {
				t.Errorf("column %s type source = %q", column.Name, column.TypeSource)
			}
		}
		target := ImportTarget{Kind: "book", ID: book.ID}
		if err := f.service.PrepareImport(f.ctx, owner, target, file); err != nil {
			t.Fatal(err)
		}
		for _, column := range file.Analysis.Columns {
			if column.UIType == "link" && (column.RelinkTargetID != projects.Table.ID || column.LinkTarget != "Programs") {
				t.Fatalf("link column = %+v, want target %s", column, projects.Table.ID)
			}
		}
		result := importAllColumns(t, f, owner, target, file, "Round trip")
		if result.Rows != 1 || result.InvalidCells != 0 || len(result.Issues) != 0 {
			t.Fatalf("result = %+v", result)
		}
		assertRestored(t, result.TableID)
	})

	t.Run("meta only", func(t *testing.T) {
		path := editImportWorkbook(t, data, func(book *excelize.File) {
			if err := book.DeleteSheet("_pandatable"); err != nil {
				t.Fatal(err)
			}
		})
		file, err := AnalyzeImportFile(f.ctx, path, "meta-only.xlsx", owner)
		if err != nil {
			t.Fatal(err)
		}
		if file.Analysis.Format != "package" || file.Analysis.MetaVersion != "v2" ||
			!hasImportNote(file.Analysis.Notes, ImportReasonMetaEdited) {
			t.Fatalf("meta-only analysis = %+v", file.Analysis)
		}
		result := importAllColumns(t, f, owner, ImportTarget{Kind: "book", ID: book.ID}, file, "Meta only")
		if result.Rows != 1 || result.InvalidCells != 0 || !hasImportNote(result.Issues, ImportReasonMetaEdited) {
			t.Fatalf("meta-only result = %+v", result)
		}
		assertRestored(t, result.TableID)
		schema, err := f.service.Schema(f.ctx, owner, result.TableID)
		if err != nil || len(schema.Views) == 0 {
			t.Fatalf("meta-only views = %+v, %v", schema.Views, err)
		}
	})

	t.Run("edited meta wins and invalid rows degrade", func(t *testing.T) {
		path := editImportWorkbook(t, data, func(book *excelize.File) {
			setImportMetaCell(t, book, "Status", "options_json",
				`{"choices":[{"id":"a","label":"Open","color":"green"},{"id":"b","label":"Closed","color":"red"},`+
					`{"id":"c","label":"Blocked","color":"pink"}]}`)
			setImportMetaCell(t, book, "Amount", "ui_type", "no_such_type")
			setImportMetaCell(t, book, "Score", "required", "maybe")
		})
		file, err := AnalyzeImportFile(f.ctx, path, "edited.xlsx", owner)
		if err != nil {
			t.Fatal(err)
		}
		if !hasImportNote(file.Analysis.Notes, ImportReasonMetaEdited) ||
			!hasImportNote(file.Analysis.Notes, ImportReasonMetaInvalid) {
			t.Fatalf("edited notes = %+v", file.Analysis.Notes)
		}
		sources := make(map[string]ImportColumn)
		for _, column := range file.Analysis.Columns {
			sources[column.Name] = column
		}
		if sources["Status"].TypeSource != "meta" || !strings.Contains(string(sources["Status"].Options), "Blocked") {
			t.Errorf("edited status column = %+v", sources["Status"])
		}
		for _, name := range []string{"Amount", "Score"} {
			if sources[name].TypeSource != "manifest" || sources[name].UIType != original[name].UIType {
				t.Errorf("invalid meta column %s = %+v", name, sources[name])
			}
		}
		result := importAllColumns(t, f, owner, ImportTarget{Kind: "book", ID: book.ID}, file, "Edited")
		if result.Rows != 1 || result.InvalidCells != 0 || result.Issues[0].Row != 0 {
			t.Fatalf("edited result = %+v", result)
		}
		imported := importFieldsByName(t, f, owner, result.TableID)
		if !strings.Contains(string(imported["Status"].Options), "Blocked") ||
			!jsonEqual(imported["Amount"].Options, original["Amount"].Options) {
			t.Fatalf("edited fields = %s / %s", imported["Status"].Options, imported["Amount"].Options)
		}
	})

	t.Run("mismatched data header falls back to plain inference", func(t *testing.T) {
		path := editImportWorkbook(t, data, func(book *excelize.File) {
			if err := book.DeleteSheet("_pandatable"); err != nil {
				t.Fatal(err)
			}
			if err := book.SetCellValue("Data", "B1", "Renamed"); err != nil {
				t.Fatal(err)
			}
		})
		file, err := AnalyzeImportFile(f.ctx, path, "renamed.xlsx", owner)
		if err != nil {
			t.Fatal(err)
		}
		if file.Analysis.Format != "xlsx" || file.Analysis.MetaVersion != "" ||
			!hasImportNote(file.Analysis.Notes, ImportReasonMetaInvalid) {
			t.Fatalf("fallback analysis = %+v", file.Analysis)
		}
		for _, column := range file.Analysis.Columns {
			if column.TypeSource != "inferred" {
				t.Fatalf("fallback column = %+v", column)
			}
		}
	})
}

func TestImportXLSXMetaFallbackTypeOverrides(t *testing.T) {
	for _, test := range []struct {
		name        string
		removeSheet bool
		source      string
	}{
		{name: "manifest", source: "manifest"},
		{name: "inferred", removeSheet: true, source: "inferred"},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newFixture(t)
			owner := f.user("user", "active", "en")
			owner.Timezone = "UTC"
			book := f.book(owner, f.workspace(owner).ID)
			source, err := f.service.CreateTable(f.ctx, owner, book.ID, "Fallback source")
			if err != nil {
				t.Fatal(err)
			}
			score, err := f.service.CreateField(f.ctx, owner, source.Table.ID, FieldInput{
				Name: "Score", UIType: "integer", Options: json.RawMessage(`{"size":"big"}`),
			})
			if err != nil || score.Field == nil {
				t.Fatalf("create score: %v", err)
			}
			rows, err := f.service.CreateRows(f.ctx, owner, source.Table.ID, []RowInsert{{
				Fields: map[string]json.RawMessage{
					source.Fields[0].ID: json.RawMessage(`"Item"`),
					score.Field.ID:      json.RawMessage(`4`),
				},
			}})
			if err != nil {
				t.Fatal(err)
			}
			data := exportImportTestPackage(t, f, owner, source.Table.ID)
			path := editImportWorkbook(t, data, func(workbook *excelize.File) {
				setImportMetaCell(t, workbook, "Score", "ui_type", "invalid_type")
				if test.removeSheet {
					if err := workbook.DeleteSheet("_pandatable"); err != nil {
						t.Fatal(err)
					}
				}
			})
			file, err := AnalyzeImportFile(f.ctx, path, "fallback.xlsx", owner)
			if err != nil {
				t.Fatal(err)
			}
			target := ImportTarget{Kind: "book", ID: book.ID}
			if err := f.service.PrepareImport(f.ctx, owner, target, file); err != nil {
				t.Fatal(err)
			}
			if file.Analysis.Format != "package" || file.Analysis.MetaVersion != "v2" {
				t.Fatalf("fallback analysis = %+v", file.Analysis)
			}
			request := ImportRequest{Name: "Fallback imported", HasHeader: true}
			for _, column := range file.Analysis.Columns {
				mapping := ImportMapping{Index: column.Index}
				if column.Name == "Score" {
					if column.TypeSource != test.source {
						t.Fatalf("score source = %q, want %q", column.TypeSource, test.source)
					}
					mapping.UIType, mapping.Options = "text", json.RawMessage(`{}`)
				} else if test.source == "manifest" && column.Name == source.Fields[0].Name {
					if column.TypeSource != "meta" {
						t.Fatalf("display source = %q, want meta", column.TypeSource)
					}
					mapping.UIType, mapping.Options = "integer", json.RawMessage(`{"size":"big"}`)
				}
				request.Columns = append(request.Columns, mapping)
			}
			result, err := f.service.RunImport(f.ctx, owner, target, file, request, nil)
			if err != nil {
				t.Fatalf("import fallback: %#v", err)
			}
			fields := importFieldsByName(t, f, owner, result.TableID)
			if fields["Score"].UIType != "text" || fields[source.Fields[0].Name].UIType != "text" {
				t.Fatalf("imported types = %+v", fields)
			}
			row, err := f.service.GetRow(f.ctx, owner, result.TableID, rows[0].ID)
			if err != nil || string(row.Fields[fields["Score"].ID]) != `"4"` {
				t.Fatalf("imported score = %s, error = %v", row.Fields[fields["Score"].ID], err)
			}
			for index := range request.Columns {
				if request.Columns[index].UIType == "text" {
					request.Columns[index].UIType = "attachment"
				}
			}
			if _, err := f.service.RunImport(f.ctx, owner, target, file, request, nil); err == nil {
				t.Fatal("structural fallback override unexpectedly succeeded")
			}
		})
	}
}

// GP-07.2: same-instance attachment references relink to new attachment rows
// sharing the stored file, only for importers who can read the source table.
func TestImportXLSXMetaRelinksReadableAttachments(t *testing.T) {
	f := newFixture(t)
	owner := f.user("user", "active", "en")
	owner.Timezone = "UTC"
	book := f.book(owner, f.workspace(owner).ID)
	source, err := f.service.CreateTable(f.ctx, owner, book.ID, "Library")
	if err != nil {
		t.Fatal(err)
	}
	field, err := f.service.CreateField(f.ctx, owner, source.Table.ID, FieldInput{Name: "Files", UIType: "attachment"})
	if err != nil || field.Field == nil {
		t.Fatalf("create attachment field: %v", err)
	}
	store, uploaded, sourcePath := uploadTrashImage(t, f, owner, source.Table.ID, field.Field.ID)
	encoded, err := json.Marshal([]attachments.File{uploaded})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := f.service.CreateRows(f.ctx, owner, source.Table.ID, []RowInsert{{
		Fields: map[string]json.RawMessage{field.Field.ID: encoded},
	}})
	if err != nil {
		t.Fatal(err)
	}
	data := exportImportTestPackage(t, f, owner, source.Table.ID)
	path := writeImportTestFile(t, "files.xlsx", data)
	analyze := func(t *testing.T, actor Actor, path string) *ImportFile {
		t.Helper()
		file, err := AnalyzeImportFile(f.ctx, path, "files.xlsx", actor)
		if err != nil {
			t.Fatal(err)
		}
		file.UseAttachmentStore(store)
		return file
	}
	attachmentCount := func() int64 {
		return scanOne[int64](f, `SELECT count(*) FROM meta.attachments`)
	}

	t.Run("readable source relinks", func(t *testing.T) {
		before := attachmentCount()
		result := importAllColumns(t, f, owner, ImportTarget{Kind: "book", ID: book.ID},
			analyze(t, owner, path), "Relinked")
		if result.Rows != 1 || result.InvalidCells != 0 || len(result.Issues) != 0 {
			t.Fatalf("relink result = %+v", result)
		}
		imported := importFieldsByName(t, f, owner, result.TableID)["Files"]
		row, err := f.service.GetRow(f.ctx, owner, result.TableID, rows[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		var values []codec.AttachmentValue
		if err := json.Unmarshal(row.Fields[imported.ID], &values); err != nil || len(values) != 1 {
			t.Fatalf("relinked cell = %s, %v", row.Fields[imported.ID], err)
		}
		relinked := values[0]
		if relinked.ID == uploaded.ID || relinked.Name != uploaded.Name || relinked.Size != uploaded.Size ||
			relinked.Thumb == nil {
			t.Fatalf("relinked value = %+v, source %+v", relinked, uploaded)
		}
		if after := attachmentCount(); after != before+1 {
			t.Fatalf("attachment rows = %d, want %d", after, before+1)
		}
		if tableID := scanOne[string](f, `SELECT table_id::text FROM meta.attachments WHERE id=$1`,
			relinked.ID); tableID != result.TableID {
			t.Fatalf("relinked attachment table = %s, want %s", tableID, result.TableID)
		}
		for _, thumbnail := range []bool{false, true} {
			opened, err := store.Open(f.ctx, owner.ID, relinked.ID, thumbnail)
			if err != nil {
				t.Fatalf("open relinked attachment thumbnail=%t: %v", thumbnail, err)
			}
			if err := opened.Reader.Close(); err != nil {
				t.Fatal(err)
			}
		}
		assertPathExists(t, sourcePath, true)
	})

	t.Run("unreadable source keeps the file name", func(t *testing.T) {
		stranger := f.user("user", "active", "en")
		strangerBook := f.book(stranger, f.workspace(stranger).ID)
		before := attachmentCount()
		result := importAllColumns(t, f, stranger, ImportTarget{Kind: "book", ID: strangerBook.ID},
			analyze(t, stranger, path), "Stranger")
		want := ImportIssue{Row: 2, Column: "Files", Value: uploaded.Name, Reason: ImportReasonFileUnavailable}
		if result.Rows != 1 || result.InvalidCells != 1 || len(result.Issues) != 1 || result.Issues[0] != want {
			t.Fatalf("stranger result = %+v", result)
		}
		if after := attachmentCount(); after != before {
			t.Fatalf("stranger created attachment rows: %d -> %d", before, after)
		}
		f.grant(acl.Table, source.Table.ID, "user", stranger.ID, acl.Read)
		result = importAllColumns(t, f, stranger, ImportTarget{Kind: "book", ID: strangerBook.ID},
			analyze(t, stranger, path), "Granted")
		if result.InvalidCells != 0 || attachmentCount() != before+1 {
			t.Fatalf("granted result = %+v", result)
		}
	})

	t.Run("another instance keeps the file name", func(t *testing.T) {
		other := editImportWorkbook(t, data, func(book *excelize.File) {
			rows, err := book.GetRows(XLSXMetaSheet)
			if err != nil {
				t.Fatal(err)
			}
			for index, row := range rows[:XLSXMetaHeaderRow-1] {
				for column := 0; column+1 < len(row); column += 2 {
					if row[column] == "source_instance_id" {
						cell, _ := excelize.CoordinatesToCellName(column+2, index+1)
						if err := book.SetCellValue(XLSXMetaSheet, cell, "01900000-0000-7000-8000-00000000abcd"); err != nil {
							t.Fatal(err)
						}
					}
				}
			}
		})
		before := attachmentCount()
		result := importAllColumns(t, f, owner, ImportTarget{Kind: "book", ID: book.ID},
			analyze(t, owner, other), "Other instance")
		if result.InvalidCells != 1 || !hasImportReason(result.Issues, ImportReasonFileUnavailable) ||
			attachmentCount() != before {
			t.Fatalf("other instance result = %+v", result)
		}
	})
}

func hasImportReason(issues []ImportIssue, reason string) bool {
	for _, issue := range issues {
		if issue.Reason == reason {
			return true
		}
	}
	return false
}

// GP-07.3: a plain workbook without metadata still infers its types, and
// inferred select choices receive distinct, non-gray colors.
func TestImportPlainXLSXInfersColoredChoices(t *testing.T) {
	book := excelize.NewFile()
	sheet := book.GetSheetName(0)
	if err := book.SetSheetRow(sheet, "A1", &[]any{"Name", "Count", "Stage"}); err != nil {
		t.Fatal(err)
	}
	stages := []string{"Open", "Review", "Closed", "Blocked"}
	for index := range 40 {
		cell, _ := excelize.CoordinatesToCellName(1, index+2)
		if err := book.SetSheetRow(sheet, cell, &[]any{"Item " + string(rune('A'+index%26)) + strings.Repeat("x", index),
			index + 1, stages[index%len(stages)]}); err != nil {
			t.Fatal(err)
		}
	}
	var data bytes.Buffer
	if err := book.Write(&data); err != nil {
		t.Fatal(err)
	}
	file, err := AnalyzeImportFile(t.Context(), writeImportTestFile(t, "plain.xlsx", data.Bytes()), "plain.xlsx",
		Actor{Locale: "en", Timezone: "UTC"})
	if err != nil {
		t.Fatal(err)
	}
	if file.Analysis.Format != "xlsx" || file.Analysis.MetaVersion != "" || len(file.Analysis.Notes) != 0 {
		t.Fatalf("plain analysis = %+v", file.Analysis)
	}
	want := []string{"text", "integer", "single_select"}
	for index, column := range file.Analysis.Columns {
		if column.UIType != want[index] || column.TypeSource != "inferred" {
			t.Errorf("plain column %d = %+v", index, column)
		}
	}
	var options struct {
		Choices []codec.Choice `json:"choices"`
	}
	if err := json.Unmarshal(file.Analysis.Columns[2].Options, &options); err != nil {
		t.Fatal(err)
	}
	if len(options.Choices) != len(stages) {
		t.Fatalf("inferred choices = %+v", options.Choices)
	}
	seen := make(map[string]bool)
	for _, choice := range options.Choices {
		if choice.Color == "" || choice.Color == "gray" || seen[choice.Color] {
			t.Errorf("inferred choice color = %+v", choice)
		}
		seen[choice.Color] = true
	}
}
