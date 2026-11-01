package httpapi

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"pandatable/internal/acl"
	"pandatable/internal/structure"
)

func prepareExportForTest(f *aclFixture, user *aclUser, view string, input any) string {
	f.t.Helper()
	body, err := json.Marshal(input)
	if err != nil {
		f.t.Fatal(err)
	}
	result := decode[struct {
		Ticket string `json:"ticket"`
	}](f, f.api(user, "POST", "/views/"+view+"/export", string(body)), http.StatusCreated)
	if result.Ticket == "" {
		f.t.Fatal("export ticket is empty")
	}
	return result.Ticket
}

func TestExportHTTPViewAndPackage(t *testing.T) {
	f := newACLFixture(t)
	owner, reader, stranger := f.user("user", false), f.user("user", false), f.user("user", false)
	f.exec("UPDATE meta.users SET display_name=$1, timezone=$2 WHERE id=$3", "审阅者", "Asia/Shanghai", reader.id)
	book := f.newBook(owner, f.newWorkspace(owner, "Export").ID, "Book")
	table := f.newTable(owner, book.ID, "任务")
	target := f.newTable(owner, book.ID, "Private projects")
	f.grantOK(owner, acl.Table, table.Table.ID, "user", reader.id, acl.Read)
	name := relationLabelField(table.TableSchema)
	targetName := relationLabelField(target.TableSchema)
	group := f.newField(owner, table.Table.ID, `{"name":"Group","uiType":"text"}`)
	status := f.newField(owner, table.Table.ID, `{"name":"Status","uiType":"text"}`)
	number := f.newField(owner, table.Table.ID, `{"name":"Amount","uiType":"decimal","options":{"precision":12,"scale":2}}`)
	large := f.newField(owner, table.Table.ID, `{"name":"Large","uiType":"integer","options":{"size":"big"}}`)
	checked := f.newField(owner, table.Table.ID, `{"name":"Done","uiType":"checkbox"}`)
	when := f.newField(owner, table.Table.ID, `{"name":"When","uiType":"datetime"}`)
	user := f.newField(owner, table.Table.ID, `{"name":"Assignee","uiType":"user"}`)
	choices := f.newField(owner, table.Table.ID, `{"name":"Tags","uiType":"multi_select","options":{"choices":[{"id":"a","label":"甲"},{"id":"b","label":"乙"}]}}`)
	textList := f.newField(owner, table.Table.ID, `{"name":"Text list","uiType":"text_array"}`)
	intList := f.newField(owner, table.Table.ID, `{"name":"Integer list","uiType":"int_array"}`)
	users := f.newField(owner, table.Table.ID, `{"name":"Users","uiType":"multi_user"}`)
	secret := f.newField(owner, table.Table.ID, `{"name":"Hidden","uiType":"text"}`)
	notes := f.newField(owner, table.Table.ID, `{"name":"Notes","uiType":"long_text"}`)
	defaulted := f.newField(owner, table.Table.ID, `{"name":"Defaulted","uiType":"integer","isRequired":true,"default":7}`)
	unique := f.newField(owner, table.Table.ID, `{"name":"Code","uiType":"text","isUnique":true}`)
	formulaField := f.newField(owner, table.Table.ID,
		`{"name":"Double","uiType":"formula","options":{"expr":"{Amount} * 2"}}`)
	relation := decode[relationBody](f, f.api(owner, "POST", "/tables/"+table.Table.ID+"/relations",
		fmt.Sprintf(`{"name":"Project","reverseName":"Tasks","targetTableId":%q,"kind":"one_many","sourceSide":"child"}`,
			target.Table.ID)), 201)
	if relation.Field == nil {
		t.Fatal("relation field missing")
	}

	project := decode[rowsResponse](f, f.api(owner, "POST", "/tables/"+target.Table.ID+"/rows",
		fmt.Sprintf(`{"rows":[{"fields":{%q:"项目, A"}}]}`, targetName.ID)), 201).Rows[0]
	rowInput := map[string]any{"rows": []any{
		map[string]any{"fields": map[string]any{
			name.ID: "中文,\"引号\"", group.ID: "B", status.ID: "Open", number.ID: "12.50",
			large.ID: "9007199254740993", checked.ID: true, when.ID: "2026-10-24T03:00:00.123456Z",
			user.ID: reader.id, choices.ID: []string{"甲", "乙"}, textList.ID: []string{"first, second", " spaced ", `"quoted"`},
			intList.ID: []string{"9223372036854775807", "-2"}, users.ID: []string{reader.id, owner.id},
			secret.ID: "hidden one", notes.ID: "第一行\n第二行", unique.ID: "one",
		}},
		map[string]any{"fields": map[string]any{
			name.ID: "Second", group.ID: "A", status.ID: "Open", number.ID: "3.00", secret.ID: "hidden two", unique.ID: "two",
		}},
		map[string]any{"fields": map[string]any{
			name.ID: "Excluded", group.ID: "A", status.ID: "Closed", number.ID: "99.00", secret.ID: "hidden three", unique.ID: "three",
		}},
	}}
	rawRows, err := json.Marshal(rowInput)
	if err != nil {
		t.Fatal(err)
	}
	rows := decode[rowsResponse](f, f.api(owner, "POST", "/tables/"+table.Table.ID+"/rows", string(rawRows)), 201).Rows
	f.expectStatus(f.api(owner, "POST", "/tables/"+table.Table.ID+"/rows/"+rows[0].ID+
		"/links/"+relation.Field.ID, fmt.Sprintf(`{"add":[%q]}`, project.ID)), 200, "")
	privateView := decode[viewResponse](f, f.api(owner, "POST", "/tables/"+table.Table.ID+"/views",
		`{"name":"Owner private view","scope":"personal"}`), 201).View
	f.expectStatus(f.api(reader, "POST", "/views/"+privateView.ID+"/export", `{"format":"csv"}`), 404, "ERR_NOT_FOUND")

	schema := decode[structure.TableSchema](f, f.api(owner, "GET", "/tables/"+table.Table.ID+"/schema", ""), 200)
	view := schema.Views[0]
	columns := make([]structure.ViewColumn, 0, len(schema.Fields))
	for _, field := range schema.Fields {
		visible := field.ID == name.ID || field.ID == when.ID || field.ID == user.ID ||
			field.ID == choices.ID || field.ID == notes.ID || field.ID == relation.Field.ID
		columns = append(columns, structure.ViewColumn{ID: field.ID, Visible: visible})
	}
	config := map[string]any{
		"fields": columns, "frozen": 0, "groupBy": []string{group.ID},
		"sort":   []any{map[string]any{"field": number.ID, "dir": "desc"}},
		"filter": map[string]any{"field": status.ID, "cmp": "is", "value": "Open"},
	}
	rawConfig, err := json.Marshal(map[string]any{"config": config})
	if err != nil {
		t.Fatal(err)
	}
	f.expectStatus(f.fieldCall(owner, "PATCH", "/views/"+view.ID, string(rawConfig), fmt.Sprint(view.Version)), 200, "")
	f.expectStatus(f.api(reader, "POST", "/views/"+view.ID+"/export",
		fmt.Sprintf(`{"format":"csv","fieldIds":[%q,%q]}`, name.ID, name.ID)), 400, "ERR_VALIDATION")
	f.expectStatus(f.api(reader, "POST", "/views/"+view.ID+"/export",
		fmt.Sprintf(`{"format":"xlsx","fieldIds":[%q]}`, name.ID)), 400, "ERR_VALIDATION")

	ticket := prepareExportForTest(f, reader, view.ID, map[string]any{"format": "csv"})
	response := f.api(reader, "GET", "/exports/"+ticket, "")
	f.expectStatus(response, 200, "")
	if response.Header().Get("Content-Type") != "text/csv; charset=utf-8" ||
		!strings.Contains(response.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("CSV headers = %v", response.Header())
	}
	data := response.Body.Bytes()
	if !bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}) || !bytes.Contains(data, []byte("\r\n")) {
		t.Fatalf("CSV missing BOM or CRLF: %q", data)
	}
	records, err := csv.NewReader(bytes.NewReader(data[3:])).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	wantHeader := []string{name.Name, "When", "Assignee", "Tags", "Notes", "Project"}
	if fmt.Sprint(records[0]) != fmt.Sprint(wantHeader) || len(records) != 3 {
		t.Fatalf("CSV records = %#v", records)
	}
	if records[1][0] != "Second" || records[2][0] != "中文,\"引号\"" ||
		records[2][1] != "2026-10-24 11:00:00" || records[2][2] != "审阅者" ||
		records[2][3] != "甲, 乙" || records[2][4] != "第一行\n第二行" || records[2][5] != "项目, A" {
		t.Fatalf("CSV values/order = %#v", records)
	}
	if bytes.Contains(data, []byte("hidden")) || bytes.Contains(data, []byte("Excluded")) {
		t.Fatal("CSV leaked hidden fields or filtered rows")
	}
	f.expectStatus(f.api(reader, "GET", "/exports/"+ticket, ""), 404, "ERR_NOT_FOUND")
	f.expectStatus(f.api(stranger, "POST", "/views/"+view.ID+"/export", `{"format":"csv"}`), 404, "ERR_NOT_FOUND")

	ticket = prepareExportForTest(f, reader, view.ID, map[string]any{"format": "xlsx"})
	response = f.api(reader, "GET", "/exports/"+ticket, "")
	f.expectStatus(response, 200, "")
	if response.Header().Get("Content-Type") != xlsxContentType {
		t.Fatalf("package content type = %q", response.Header().Get("Content-Type"))
	}
	bookFile, err := excelize.OpenReader(bytes.NewReader(response.Body.Bytes()))
	if err != nil {
		t.Fatalf("open package: %v", err)
	}
	defer bookFile.Close()
	sheets := bookFile.GetSheetList()
	if fmt.Sprint(sheets) != "[Data _pandatable]" {
		t.Fatalf("package sheets = %v", sheets)
	}
	visible, err := bookFile.GetSheetVisible("_pandatable")
	if err != nil || visible {
		t.Fatalf("manifest visibility = %v, %v", visible, err)
	}
	archive, err := zip.NewReader(bytes.NewReader(response.Body.Bytes()), int64(response.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	var workbook string
	for _, entry := range archive.File {
		if entry.Name == "xl/workbook.xml" {
			opened, err := entry.Open()
			if err != nil {
				t.Fatal(err)
			}
			content, err := io.ReadAll(opened)
			_ = opened.Close()
			if err != nil {
				t.Fatal(err)
			}
			workbook = string(content)
		}
	}
	if !strings.Contains(workbook, `state="veryHidden"`) {
		t.Fatalf("manifest sheet is not very hidden: %s", workbook)
	}
	manifestRows, err := bookFile.GetRows("_pandatable")
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
		AppVersion    string `json:"appVersion"`
		Table         struct {
			Name         string `json:"name"`
			DisplayField string `json:"displayField"`
		} `json:"table"`
		Fields []struct {
			Key      string          `json:"key"`
			Name     string          `json:"name"`
			UIType   string          `json:"uiType"`
			Options  map[string]any  `json:"options"`
			Required bool            `json:"required"`
			Unique   bool            `json:"unique"`
			Default  json.RawMessage `json:"default"`
		} `json:"fields"`
		Views []struct {
			Name   string                `json:"name"`
			Config structure.ViewColumns `json:"config"`
		} `json:"views"`
		RowIDColumn bool `json:"rowIdColumn"`
	}
	if err := json.Unmarshal([]byte(manifestText.String()), &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Format != "pandatable.table" || manifest.FormatVersion != 1 ||
		manifest.AppVersion != "0.0.0-dev" || manifest.Table.Name != "任务" ||
		!manifest.RowIDColumn || len(manifest.Fields) != len(schema.Fields) || len(manifest.Views) != 1 {
		t.Fatalf("package manifest = %+v", manifest)
	}
	dataRows, err := bookFile.GetRows("Data")
	if err != nil {
		t.Fatal(err)
	}
	if len(dataRows) != 4 || dataRows[0][0] != "_id" || dataRows[1][0] != rows[0].ID {
		t.Fatalf("package rows = %#v", dataRows)
	}
	fieldColumns := make(map[string]int)
	fieldKeys := make(map[string]string)
	for index, field := range manifest.Fields {
		if field.Key != fmt.Sprintf("f%d", index+1) || dataRows[0][index+1] != field.Name {
			t.Fatalf("manifest/data field order mismatch at %d: %+v", index, field)
		}
		fieldColumns[field.Name] = index + 2
		fieldKeys[field.Name] = field.Key
		if field.UIType == "link" && field.Options["targetTable"] != nil {
			t.Fatalf("unreadable linked table leaked in manifest: %+v", field.Options)
		}
		if field.Name == formulaField.Name && field.Options["expr"] != "{Amount} * 2" {
			t.Fatalf("formula definition missing: %+v", field.Options)
		}
		if field.Name == defaulted.Name && (!field.Required || string(field.Default) != "7") {
			t.Fatalf("required field/default not preserved: %+v", field)
		}
		if field.Name == unique.Name && !field.Unique {
			t.Fatalf("unique field constraint not preserved: %+v", field)
		}
	}
	packageView := manifest.Views[0].Config
	var packageFilter structure.FilterNode
	if err := json.Unmarshal(packageView.Filter, &packageFilter); err != nil {
		t.Fatal(err)
	}
	if manifest.Table.DisplayField != fieldKeys[name.Name] || len(packageView.GroupBy) != 1 ||
		packageView.GroupBy[0] != fieldKeys[group.Name] || len(packageView.Sort) != 1 ||
		packageView.Sort[0].Field != fieldKeys[number.Name] || packageFilter.Field != fieldKeys[status.Name] {
		t.Fatalf("package did not remap display/view field references: %+v / %+v", manifest.Table, packageView)
	}
	if strings.Contains(manifestText.String(), "Private projects") {
		t.Fatal("manifest leaked an unreadable linked table name")
	}
	if dataRows[1][fieldColumns["Hidden"]-1] != "hidden one" ||
		dataRows[3][fieldColumns[name.Name]-1] != "Excluded" {
		t.Fatalf("package did not include full table: %#v", dataRows)
	}
	cell := func(name string) string {
		address, err := excelize.CoordinatesToCellName(fieldColumns[name], 2)
		if err != nil {
			t.Fatal(err)
		}
		return address
	}
	for _, item := range []struct {
		name string
		want excelize.CellType
	}{
		{"Done", excelize.CellTypeBool},
		{"Large", excelize.CellTypeInlineString},
		{"Amount", excelize.CellTypeUnset},
		{"Double", excelize.CellTypeUnset},
		{"When", excelize.CellTypeUnset},
	} {
		got, err := bookFile.GetCellType("Data", cell(item.name))
		if err != nil || got != item.want {
			t.Fatalf("%s cell type = %v (%v), want %v", item.name, got, err, item.want)
		}
		if item.want == excelize.CellTypeUnset {
			raw, err := bookFile.GetCellValue("Data", cell(item.name), excelize.Options{RawCellValue: true})
			if _, parseErr := strconv.ParseFloat(raw, 64); err != nil || parseErr != nil {
				t.Fatalf("%s numeric cell = %q (%v, %v)", item.name, raw, err, parseErr)
			}
		}
	}
	serialText, err := bookFile.GetCellValue("Data", cell("When"), excelize.Options{RawCellValue: true})
	if err != nil {
		t.Fatal(err)
	}
	serial, err := strconv.ParseFloat(serialText, 64)
	if err != nil {
		t.Fatal(err)
	}
	expectedSerial := time.Date(2026, 10, 24, 3, 0, 0, 123456000, time.UTC).Sub(
		time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)).Seconds() / (24 * 60 * 60)
	if difference := math.Abs(serial-expectedSerial) * 24 * 60 * 60; difference > 0.000001 {
		t.Fatalf("typed datetime loses precision: serial %s, error %.9f seconds", serialText, difference)
	}
	styleID, err := bookFile.GetCellStyle("Data", cell("When"))
	if err != nil {
		t.Fatal(err)
	}
	style, err := bookFile.GetStyle(styleID)
	if err != nil || style == nil || style.CustomNumFmt == nil || *style.CustomNumFmt != "yyyy-mm-dd hh:mm:ss.000" {
		t.Fatalf("Excel-compatible datetime format = %+v, %v", style, err)
	}
	if got, err := bookFile.GetCellValue("Data", cell("When")); err != nil || !strings.HasPrefix(got, "2026-10-24 03:00:00") {
		t.Fatalf("typed datetime = %q (%v)", got, err)
	}
	if got, err := bookFile.GetCellValue("Data", cell("Assignee")); err != nil || got != reader.id {
		t.Fatalf("package user identity = %q (%v)", got, err)
	}
	for _, item := range []struct {
		name string
		want []string
	}{
		{"Tags", []string{"甲", "乙"}},
		{"Text list", []string{"first, second", " spaced ", `"quoted"`}},
		{"Integer list", []string{"9223372036854775807", "-2"}},
		{"Users", []string{reader.id, owner.id}},
		{"Project", []string{"项目, A"}},
	} {
		text, err := bookFile.GetCellValue("Data", cell(item.name))
		if err != nil {
			t.Fatal(err)
		}
		var values []string
		if err := json.Unmarshal([]byte(text), &values); err != nil || !reflect.DeepEqual(values, item.want) {
			t.Fatalf("%s package list = %q, %v; want %v", item.name, text, err, item.want)
		}
	}

	ticket = prepareExportForTest(f, owner, privateView.ID, map[string]any{"format": "xlsx"})
	ownerPackage := f.api(owner, "GET", "/exports/"+ticket, "")
	f.expectStatus(ownerPackage, 200, "")
	ownerBook, err := excelize.OpenReader(bytes.NewReader(ownerPackage.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	defer ownerBook.Close()
	privateManifest, err := ownerBook.GetCellValue("_pandatable", "A1")
	if err != nil || strings.Contains(privateManifest, "Owner private view") {
		t.Fatalf("private view included in package manifest: %v", err)
	}
}
