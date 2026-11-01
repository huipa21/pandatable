package structure

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"pandatable/internal/codec"
)

func TestLegacyPackageAttachmentNamesAreAmbiguous(t *testing.T) {
	bound, err := codec.Bind(codec.Attachment, nil)
	if err != nil {
		t.Fatal(err)
	}
	field := rowField{id: "files", kind: codec.Attachment, codec: bound}
	plan := exportPlan{location: time.UTC}
	files := [][]codec.AttachmentValue{
		{{ID: "00000000-0000-0000-0000-000000000001", Name: "report.pdf", MIME: "application/pdf", Size: 10}},
		{{ID: "00000000-0000-0000-0000-000000000002", Name: "report.pdf", MIME: "application/pdf", Size: 20}},
	}
	for _, attachments := range files {
		raw, err := json.Marshal(attachments)
		if err != nil {
			t.Fatal(err)
		}
		row := exportRow{
			record: RowRecord{Fields: map[string]json.RawMessage{field.id: raw}},
			values: map[string]any{field.id: raw},
		}
		value, err := plan.spreadsheetValue(row, field, nil)
		if err != nil || value != "report.pdf" {
			t.Fatalf("attachment package value = %v, %v", value, err)
		}
		column := importColumnPlan{field: field, source: ImportColumn{UIType: "attachment"}}
		if value, err := parseImportCell(column, "report.pdf", true, codec.Env{Location: time.UTC}); err == nil || value != nil {
			t.Fatalf("ambiguous attachment import = %v, %v", value, err)
		}
	}
}

func TestLegacyPackageDateTimeSerialCollision(t *testing.T) {
	instants := []time.Time{
		time.Date(9999, 12, 31, 12, 0, 0, 0, time.UTC),
		time.Date(9999, 12, 31, 12, 0, 0, 1000, time.UTC),
	}
	book := excelize.NewFile()
	stream, err := book.NewStreamWriter("Sheet1")
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.SetRow("A1", []any{instants[0], instants[1]}); err != nil {
		t.Fatal(err)
	}
	if err := stream.Flush(); err != nil {
		t.Fatal(err)
	}
	var data bytes.Buffer
	if err := book.Write(&data); err != nil {
		t.Fatal(err)
	}
	if err := book.Close(); err != nil {
		t.Fatal(err)
	}
	book, err = excelize.OpenReader(bytes.NewReader(data.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := book.Close(); err != nil {
			t.Errorf("close imported workbook: %v", err)
		}
	}()
	for index, cell := range []string{"A1", "B1"} {
		serial, err := book.GetCellValue("Sheet1", cell, excelize.Options{RawCellValue: true})
		if err != nil || serial != "2958465.5" {
			t.Fatalf("%s serial = %q, %v", cell, serial, err)
		}
		value, numeric, err := parsePackageTemporal(codec.DateTime, serial)
		if err != nil || !numeric {
			t.Fatalf("%s imported serial = %v, %t, %v", cell, value, numeric, err)
		}
		imported, ok := value.(time.Time)
		if !ok || !imported.Equal(instants[0]) || index == 1 && imported.Equal(instants[1]) {
			t.Fatalf("%s imported instant = %v; original = %v", cell, value, instants[index])
		}
	}
}
