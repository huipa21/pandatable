package structure

import (
	"bytes"
	"encoding/json"
	"fmt"
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
	files := [][]codec.AttachmentValue{
		{{ID: "00000000-0000-0000-0000-000000000001", Name: "report.pdf", MIME: "application/pdf", Size: 10}},
		{{ID: "00000000-0000-0000-0000-000000000002", Name: "report.pdf", MIME: "application/pdf", Size: 20}},
	}
	for _, attachments := range files {
		raw, err := json.Marshal(attachments)
		if err != nil {
			t.Fatal(err)
		}
		value, err := field.codec.FormatText(raw, codec.Env{Location: time.UTC})
		if err != nil || value != "report.pdf" {
			t.Fatalf("legacy attachment name = %v, %v", value, err)
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

func TestPackageDateTimePreservesRepresentablePrecision(t *testing.T) {
	instants := []time.Time{
		time.Date(1900, 1, 1, 0, 0, 0, 1000, time.UTC),
		time.Date(1900, 2, 28, 23, 59, 59, 999999000, time.UTC),
		time.Date(1900, 3, 1, 0, 0, 0, 1000, time.UTC),
		time.Date(1970, 1, 1, 0, 0, 0, 123456000, time.UTC),
		time.Date(2070, 12, 31, 23, 59, 59, 999999000, time.UTC),
	}
	for index := range 1024 {
		instants = append(instants, time.Date(2026, 10, 24, 3, 0, 0, index*977000, time.UTC))
	}
	book := excelize.NewFile()
	stream, err := book.NewStreamWriter("Sheet1")
	if err != nil {
		t.Fatal(err)
	}
	for index, instant := range instants {
		if err := stream.SetRow(fmt.Sprintf("A%d", index+1), []any{instant}); err != nil {
			t.Fatal(err)
		}
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
	rows, err := book.Rows("Sheet1")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Errorf("close imported rows: %v", err)
		}
	}()
	for index, instant := range instants {
		if !rows.Next() {
			t.Fatalf("missing row %d: %v", index+1, rows.Error())
		}
		values, err := rows.Columns(excelize.Options{RawCellValue: true})
		if err != nil || len(values) != 1 {
			t.Fatalf("row %d values = %v, %v", index+1, values, err)
		}
		value, numeric, err := parsePackageTemporal(codec.DateTime, values[0])
		imported, ok := value.(time.Time)
		if err != nil || !numeric || !ok || !imported.Equal(instant) {
			t.Fatalf("row %d serial %s restored %v, original %v: %v", index+1, values[0], value, instant, err)
		}
	}
	if rows.Next() || rows.Error() != nil {
		t.Fatalf("unexpected extra rows: %v", rows.Error())
	}
}
