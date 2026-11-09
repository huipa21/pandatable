package httpapi

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/xuri/excelize/v2"

	"pandatable/internal/acl"
	"pandatable/internal/codec"
	"pandatable/internal/structure"
)

func TestExportXLSXMetadataAndNativeCells(t *testing.T) {
	f := newACLFixture(t)
	owner, reader := f.user("user", false), f.user("user", false)
	book := f.newBook(owner, f.newWorkspace(owner, "Metadata").ID, "Book")
	source := f.newTable(owner, book.ID, "Tasks")
	target := f.newTable(owner, book.ID, "Private projects")
	f.grantOK(owner, acl.Table, source.Table.ID, "user", reader.id, acl.Read)
	name := relationLabelField(source.TableSchema)
	targetName := relationLabelField(target.TableSchema)
	targetField := f.newField(owner, target.Table.ID, `{"name":"Private detail","uiType":"text"}`)

	amount := f.newField(owner, source.Table.ID,
		`{"name":"Amount","description":"Cost < & units","uiType":"decimal","options":{"precision":12,"scale":2},"isRequired":true,"default":7}`)
	currency := f.newField(owner, source.Table.ID,
		`{"name":"Currency","uiType":"currency","options":{"currency":"USD","scale":2}}`)
	percent := f.newField(owner, source.Table.ID, `{"name":"Percent","uiType":"percent"}`)
	checked := f.newField(owner, source.Table.ID, `{"name":"Done","uiType":"checkbox"}`)
	date := f.newField(owner, source.Table.ID, `{"name":"Date","uiType":"date"}`)
	dateTime := f.newField(owner, source.Table.ID,
		`{"name":"When","uiType":"datetime","options":{"timezone":"fixed","fixedTz":"Asia/Shanghai"}}`)
	clock := f.newField(owner, source.Table.ID, `{"name":"Clock","uiType":"time"}`)
	single := f.newField(owner, source.Table.ID,
		`{"name":"Status","uiType":"single_select","options":{"choices":[{"id":"a","label":"Open","color":"red"},{"id":"b","label":"完成🎉","color":"blue"}]}}`)
	multi := f.newField(owner, source.Table.ID,
		`{"name":"Tags","uiType":"multi_select","options":{"choices":[{"id":"x","label":"甲","color":"green"},{"id":"y","label":"乙","color":"purple"}]}}`)
	files := f.newField(owner, source.Table.ID, `{"name":"Files","uiType":"attachment"}`)
	formula := f.newField(owner, source.Table.ID,
		`{"name":"Double","uiType":"formula","options":{"expr":"{Amount} * 2"}}`)
	relation := decode[relationBody](f, f.api(owner, "POST", "/tables/"+source.Table.ID+"/relations",
		fmt.Sprintf(`{"name":"Project","reverseName":"Tasks","targetTableId":%q,"kind":"one_many","sourceSide":"child"}`,
			target.Table.ID)), 201)
	if relation.Field == nil {
		t.Fatal("relation field missing")
	}
	lookup := f.newField(owner, source.Table.ID,
		fmt.Sprintf(`{"name":"Target detail","uiType":"lookup","options":{"relationFieldId":%q,"targetFieldId":%q}}`,
			relation.Field.ID, targetField.ID))
	file, err := f.attachmentService.Upload(f.ctx, owner.id, source.Table.ID, files.ID, "report, final.pdf",
		strings.NewReader("%PDF-1.4\nfixture\n%%EOF\n"), 1<<20, 1)
	if err != nil {
		t.Fatal(err)
	}
	project := decode[rowsResponse](f, f.api(owner, "POST", "/tables/"+target.Table.ID+"/rows",
		fmt.Sprintf(`{"rows":[{"fields":{%q:"Project A",%q:"classified"}}]}`, targetName.ID, targetField.ID)), 201).Rows[0]
	body, err := json.Marshal(map[string]any{"rows": []any{map[string]any{"fields": map[string]any{
		name.ID: "Example", amount.ID: "12.50", currency.ID: "99.95", percent.ID: "0.2500",
		checked.ID: true, date.ID: "2026-03-04", dateTime.ID: "2026-10-24T03:00:00.123456Z",
		clock.ID: "12:34:56.1234", single.ID: "完成🎉", multi.ID: []string{"甲", "乙"},
		files.ID: []any{file},
	}}}})
	if err != nil {
		t.Fatal(err)
	}
	row := decode[rowsResponse](f, f.api(owner, "POST", "/tables/"+source.Table.ID+"/rows", string(body)), 201).Rows[0]
	f.expectStatus(f.api(owner, "POST", "/tables/"+source.Table.ID+"/rows/"+row.ID+"/links/"+relation.Field.ID,
		fmt.Sprintf(`{"add":[%q]}`, project.ID)), 200, "")

	schema := decode[structure.TableSchema](f, f.api(owner, "GET", "/tables/"+source.Table.ID+"/schema", ""), 200)
	exportedAfter := time.Now().UTC().Add(-time.Second)
	workbook, archive := downloadExportWorkbook(t, f, owner, schema.Views[0].ID)
	defer workbook.Close()
	if sheets := workbook.GetSheetList(); !reflect.DeepEqual(sheets, []string{"Data", "_meta", "_pandatable"}) {
		t.Fatalf("sheets = %v", sheets)
	}
	if workbook.GetSheetName(workbook.GetActiveSheetIndex()) != "Data" {
		t.Fatal("Data is not the active sheet")
	}
	if visible, err := workbook.GetSheetVisible("_meta"); err != nil || !visible {
		t.Fatalf("metadata visibility = %v, %v", visible, err)
	}
	panes, err := workbook.GetPanes("_meta")
	if err != nil || !panes.Freeze || panes.YSplit != 5 || panes.TopLeftCell != "A6" {
		t.Fatalf("metadata panes = %+v, %v", panes, err)
	}
	var sheetXML string
	for _, entry := range archive.File {
		if entry.Name != "xl/worksheets/sheet2.xml" {
			continue
		}
		reader, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		raw, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if readErr != nil || closeErr != nil {
			t.Fatalf("read metadata XML: %v, %v", readErr, closeErr)
		}
		sheetXML = string(raw)
	}
	if !strings.Contains(sheetXML, "<sheetProtection") ||
		strings.Contains(sheetXML, `password=`) || strings.Contains(sheetXML, `hashValue=`) {
		t.Fatal("metadata sheet is not protected without a password")
	}

	read := func(book *excelize.File, sheet, cell string) string {
		t.Helper()
		value, err := book.GetCellValue(sheet, cell)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	for cell, want := range map[string]string{
		"A1": "format_version", "B1": "pandatable.xlsx/v2", "C1": "source_instance_id",
		"A2": "table_id", "B2": source.Table.ID, "C2": "table_name", "D2": "Tasks",
		"A3": "schema_version", "B3": strconv.Itoa(int(schema.SchemaVersion)), "C3": "exported_at",
		"A4": "datetime_timezone", "B4": "UTC",
	} {
		if got := read(workbook, "_meta", cell); got != want {
			t.Errorf("metadata %s = %q, want %q", cell, got, want)
		}
	}
	instanceID := read(workbook, "_meta", "D1")
	if parsed, ok := acl.ParseUUID(instanceID); !ok || parsed.String() != instanceID {
		t.Fatalf("invalid source instance ID %q", instanceID)
	}
	if stored := f.scan(`SELECT value #>> '{}' FROM meta.settings WHERE key='instance_id'`); stored != instanceID {
		t.Fatalf("source instance ID %q differs from stored %q", instanceID, stored)
	}
	exportedAt, err := time.Parse(time.RFC3339Nano, read(workbook, "_meta", "D3"))
	if err != nil || exportedAt.Before(exportedAfter) || exportedAt.After(time.Now().UTC()) {
		t.Fatalf("metadata export time = %v, %v", exportedAt, err)
	}
	rows, byID := readExportFieldMetadata(t, workbook, schema)
	manifestRows, err := workbook.GetRows("_pandatable")
	if err != nil {
		t.Fatal(err)
	}
	var manifestText strings.Builder
	for _, row := range manifestRows {
		if len(row) != 1 {
			t.Fatalf("manifest row = %v", row)
		}
		manifestText.WriteString(row[0])
	}
	var manifest struct {
		Format        string `json:"format"`
		FormatVersion int    `json:"formatVersion"`
		MetaSHA256    string `json:"metaSha256"`
		ExportedAt    string `json:"exportedAt"`
	}
	if err := json.Unmarshal([]byte(manifestText.String()), &manifest); err != nil {
		t.Fatal(err)
	}
	canonicalRows, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(canonicalRows)
	if manifest.Format != "pandatable.table" || manifest.FormatVersion != 1 ||
		manifest.MetaSHA256 != hex.EncodeToString(hash[:]) || manifest.ExportedAt != read(workbook, "_meta", "D3") {
		t.Fatalf("manifest metadata = %+v", manifest)
	}
	rows[0][2] = "edited"
	editedRows, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	editedHash := sha256.Sum256(editedRows)
	if hex.EncodeToString(editedHash[:]) == manifest.MetaSHA256 {
		t.Fatal("editing a field row did not change the metadata hash")
	}

	if entry := byID[amount.ID]; entry[4] != "numeric(12,2)" || entry[5] != true ||
		entry[7] != string(amount.Default) || entry[15] != "Cost < & units" {
		t.Fatalf("amount metadata = %v", entry)
	}
	if entry := byID[files.ID]; entry[13] != "same-instance-id" {
		t.Fatalf("attachment metadata = %v", entry)
	}
	if entry := byID[formula.ID]; entry[14] != "{Amount} * 2" {
		t.Fatalf("formula metadata = %v", entry)
	}
	if entry := byID[relation.Field.ID]; entry[9] != target.Table.ID ||
		entry[10] != target.Table.Name || entry[11] != targetName.Name || entry[12] != "one_many" {
		t.Fatalf("relation metadata = %v", entry)
	}
	var relationOptions struct {
		Kind     string `json:"kind"`
		Side     string `json:"side"`
		OnDelete string `json:"onDelete"`
	}
	if err := json.Unmarshal([]byte(byID[relation.Field.ID][8].(string)), &relationOptions); err != nil ||
		relationOptions.Kind != "one_many" || relationOptions.Side != "child" || relationOptions.OnDelete != "set_null" {
		t.Fatalf("relation options = %+v, %v", relationOptions, err)
	}
	if options := byID[lookup.ID][8].(string); !strings.Contains(options, targetField.ID) ||
		!strings.Contains(options, relation.Field.ID) {
		t.Fatalf("readable computed field options are incomplete: %s", options)
	}

	cell := func(field structure.Field) string {
		t.Helper()
		return byID[field.ID][0].(string) + "2"
	}
	for _, field := range []structure.Field{amount, currency, percent, date, dateTime, clock, formula} {
		address := cell(field)
		cellType, err := workbook.GetCellType("Data", address)
		raw, rawErr := workbook.GetCellValue("Data", address, excelize.Options{RawCellValue: true})
		if _, parseErr := strconv.ParseFloat(raw, 64); err != nil || rawErr != nil ||
			parseErr != nil || cellType != excelize.CellTypeUnset {
			t.Errorf("%s is not a native numeric cell: %q, %v, %v, %v", field.Name, raw, cellType, err, rawErr)
		}
	}
	if cellType, err := workbook.GetCellType("Data", cell(checked)); err != nil || cellType != excelize.CellTypeBool {
		t.Fatalf("checkbox type = %v, %v", cellType, err)
	}
	clockRaw, err := workbook.GetCellValue("Data", cell(clock), excelize.Options{RawCellValue: true})
	clockSerial, parseErr := strconv.ParseFloat(clockRaw, 64)
	if err != nil || parseErr != nil || math.Abs(clockSerial-45296123400.0/86400000000.0) > 1e-14 {
		t.Fatalf("clock serial = %q, %v, %v", clockRaw, err, parseErr)
	}
	styleID, err := workbook.GetCellStyle("Data", cell(clock))
	if err != nil {
		t.Fatal(err)
	}
	style, err := workbook.GetStyle(styleID)
	if err != nil || style == nil || style.CustomNumFmt == nil || *style.CustomNumFmt != "hh:mm:ss.000000" {
		t.Fatalf("clock style = %+v, %v", style, err)
	}
	if got := read(workbook, "Data", cell(single)); got != "完成🎉" {
		t.Fatalf("single select = %q", got)
	}

	var tags []string
	if err := json.Unmarshal([]byte(read(workbook, "Data", cell(multi))), &tags); err != nil ||
		!reflect.DeepEqual(tags, []string{"甲", "乙"}) {
		t.Fatalf("multi select = %v, %v", tags, err)
	}
	var attachments []map[string]string
	if err := json.Unmarshal([]byte(read(workbook, "Data", cell(files))), &attachments); err != nil ||
		!reflect.DeepEqual(attachments, []map[string]string{{"id": file.ID, "name": file.Name}}) {
		t.Fatalf("attachment references = %v, %v", attachments, err)
	}

	readerBook, _ := downloadExportWorkbook(t, f, reader, schema.Views[0].ID)
	defer readerBook.Close()
	if got := read(readerBook, "_meta", "D1"); got != instanceID {
		t.Fatalf("source instance changed between exports: %q, %q", instanceID, got)
	}
	_, readerFields := readExportFieldMetadata(t, readerBook, schema)
	if entry := readerFields[relation.Field.ID]; entry[9] != "" || entry[10] != "" || entry[11] != "" ||
		strings.Contains(entry[8].(string), target.Table.ID) || strings.Contains(entry[8].(string), target.Table.Name) {
		t.Fatalf("unreadable relation target leaked through metadata: %v", entry)
	}
	if options := readerFields[lookup.ID][8].(string); strings.Contains(options, targetField.ID) ||
		strings.Contains(options, targetField.Name) || strings.Contains(options, target.Table.Name) {
		t.Fatalf("unreadable computed target leaked through metadata: %s", options)
	}
}

func TestExportMetadataCellLimit(t *testing.T) {
	f := newACLFixture(t)
	owner := f.user("user", false)
	book := f.newBook(owner, f.newWorkspace(owner, "Large metadata").ID, "Book")
	table := f.newTable(owner, book.ID, "Table")
	choices := make([]map[string]string, 0, 300)
	optionsLength := func() int {
		t.Helper()
		raw, err := json.Marshal(map[string]any{"choices": choices})
		if err != nil {
			t.Fatal(err)
		}
		bound, err := codec.Bind(codec.SingleSelect, raw)
		if err != nil {
			t.Fatal(err)
		}
		return len(utf16.Encode([]rune(string(bound.Options()))))
	}
	for optionsLength() <= excelize.TotalCellChars && len(choices) < cap(choices) {
		index := len(choices)
		choices = append(choices, map[string]string{
			"id": fmt.Sprintf("choice%d", index), "label": fmt.Sprintf("%03d%s", index, strings.Repeat("x", 97)),
		})
	}
	overflow := optionsLength() - excelize.TotalCellChars
	if overflow <= 0 {
		t.Fatal("could not construct oversized choice metadata")
	}
	for index := len(choices) - 1; index >= 0 && overflow > 0; index-- {
		label := choices[index]["label"]
		remove := min(overflow, len(label)-3)
		choices[index]["label"] = label[:len(label)-remove]
		overflow -= remove
	}
	if overflow != 0 || optionsLength() != excelize.TotalCellChars {
		t.Fatal("could not construct metadata at the Excel cell limit")
	}
	body, err := json.Marshal(map[string]any{
		"name": "Maximum options", "uiType": "single_select", "options": map[string]any{"choices": choices},
	})
	if err != nil {
		t.Fatal(err)
	}
	maximum := f.newField(owner, table.Table.ID, string(body))
	schema := decode[structure.TableSchema](f, f.api(owner, "GET", "/tables/"+table.Table.ID+"/schema", ""), 200)
	workbook, _ := downloadExportWorkbook(t, f, owner, table.Views[0].ID)
	_, fields := readExportFieldMetadata(t, workbook, schema)
	options := fields[maximum.ID][8].(string)
	if got := len(utf16.Encode([]rune(options))); got != excelize.TotalCellChars || !json.Valid([]byte(options)) {
		t.Fatalf("exported metadata at Excel's cell limit = %d units, valid JSON %t", got, json.Valid([]byte(options)))
	}
	if err := workbook.Close(); err != nil {
		t.Fatal(err)
	}

	last := choices[len(choices)-1]["label"]
	if len(last) >= 100 {
		t.Fatal("last choice cannot be extended to exceed the limit")
	}
	choices[len(choices)-1]["label"] = last + "x"
	if got := optionsLength(); got != excelize.TotalCellChars+1 {
		t.Fatalf("oversized options = %d units", got)
	}
	body, err = json.Marshal(map[string]any{
		"name": "Oversized options", "uiType": "single_select", "options": map[string]any{"choices": choices},
	})
	if err != nil {
		t.Fatal(err)
	}
	f.newField(owner, table.Table.ID, string(body))
	response := f.api(owner, "POST", "/views/"+table.Views[0].ID+"/export", `{"format":"xlsx"}`)
	f.expectStatus(response, 400, "ERR_VALIDATION")
}

func downloadExportWorkbook(t *testing.T, f *aclFixture, user *aclUser, view string) (*excelize.File, *zip.Reader) {
	t.Helper()
	ticket := prepareExportForTest(f, user, view, map[string]any{"format": "xlsx"})
	response := f.api(user, "GET", "/exports/"+ticket, "")
	f.expectStatus(response, 200, "")
	raw := response.Body.Bytes()
	book, err := excelize.OpenReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	return book, archive
}

func readExportFieldMetadata(t *testing.T, book *excelize.File, schema structure.TableSchema) ([][]any, map[string][]any) {
	t.Helper()
	sheetRows, err := book.GetRows("_meta")
	if err != nil {
		t.Fatal(err)
	}
	wantHeader := []string{"column", "field_id", "name", "ui_type", "pg_type", "required", "unique",
		"default", "options_json", "relation_target_table_id", "relation_target_table_name",
		"relation_display_field", "relation_cardinality", "attachment_mode", "formula_or_expression", "description", "system"}
	if len(sheetRows) != len(schema.Fields)+5 || !reflect.DeepEqual(sheetRows[4], wantHeader) {
		t.Fatalf("metadata rows/header = %d, %v", len(sheetRows), sheetRows)
	}
	rows := make([][]any, 0, len(schema.Fields))
	byID := make(map[string][]any, len(schema.Fields))
	systemFields := 0
	for index, field := range schema.Fields {
		values := make([]any, len(wantHeader))
		for column := range values {
			address, err := excelize.CoordinatesToCellName(column+1, index+6)
			if err != nil {
				t.Fatal(err)
			}
			text, err := book.GetCellValue("_meta", address, excelize.Options{RawCellValue: true})
			if err != nil {
				t.Fatal(err)
			}
			if column == 5 || column == 6 || column == 16 {
				cellType, err := book.GetCellType("_meta", address)
				boolean, parseErr := strconv.ParseBool(text)
				if err != nil || parseErr != nil || cellType != excelize.CellTypeBool {
					t.Fatalf("metadata boolean %s = %q, %v, %v", address, text, cellType, err)
				}
				values[column] = boolean
			} else {
				values[column] = text
			}
		}
		dataColumn, err := excelize.ColumnNumberToName(index + 2)
		if err != nil || values[0] != dataColumn || values[1] != field.ID || values[2] != field.Name ||
			values[3] != field.UIType || values[5] != field.IsRequired || values[6] != field.IsUnique ||
			values[15] != field.Description || values[16] != field.IsSystem {
			t.Fatalf("field %s metadata = %v", field.Name, values)
		}
		if field.UIType != "link" && field.UIType != "lookup" && field.UIType != "rollup" {
			var source, exported any
			if err := json.Unmarshal(field.Options, &source); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(values[8].(string)), &exported); err != nil || !reflect.DeepEqual(source, exported) {
				t.Fatalf("field %s options = %v; want %s (%v)", field.Name, values[8], field.Options, err)
			}
		}
		rows = append(rows, values)
		byID[field.ID] = values
		if field.IsSystem {
			systemFields++
		}
	}
	if systemFields == 0 {
		t.Fatal("expected system fields in exported metadata")
	}
	return rows, byID
}
