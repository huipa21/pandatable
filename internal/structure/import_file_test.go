package structure

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

func writeImportTestFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestImportFileCSVDetectionAndSampling(t *testing.T) {
	text := "编号;日期;状态;备注\n"
	for index := range 1100 {
		text += fmt.Sprintf("%d;2026-10-25;进行中;记录 %d\n", index+1, index)
	}
	encoded, _, err := transform.Bytes(simplifiedchinese.GBK.NewEncoder(), []byte(text))
	if err != nil {
		t.Fatal(err)
	}
	path := writeImportTestFile(t, "sample.csv", encoded)
	file, err := AnalyzeImportFile(context.Background(), path, "sample.csv", Actor{Locale: "zh-CN", Timezone: "Asia/Shanghai"})
	if err != nil {
		t.Fatal(err)
	}
	if file.Analysis.Encoding != "gbk" || file.Analysis.Delimiter != ";" ||
		!file.Analysis.HasHeader || file.Analysis.SampledRows != 1000 {
		t.Fatalf("analysis = %+v", file.Analysis)
	}
	want := []string{"integer", "date", "single_select", "text"}
	for index, kind := range want {
		if got := file.Analysis.Columns[index].UIType; got != kind {
			t.Errorf("column %d kind = %q, want %q", index, got, kind)
		}
	}
	if file.Analysis.Columns[0].Name != "编号" || file.Analysis.Columns[2].Examples[0] != "进行中" {
		t.Fatalf("decoded columns = %+v", file.Analysis.Columns)
	}
}

func TestImportChoiceColorExhaustionMatchesFieldCreation(t *testing.T) {
	want := []string{
		"red", "orange", "amber", "yellow", "lime", "green", "teal",
		"cyan", "blue", "indigo", "purple", "pink", "gray", "red", "orange",
	}
	for index, color := range want {
		if got := importChoiceColor(index); got != color {
			t.Errorf("color %d = %q, want %q", index, got, color)
		}
	}
}

func TestImportFileCSVHeaderEncodingAndDecimals(t *testing.T) {
	for _, test := range []struct {
		name, input, encoding string
		header                bool
	}{
		{"bom", "\xef\xbb\xbfAmount,When\n1.2345,2026-01-01 10:11:12\n2.2,2026-01-02 10:11:12\n", "utf-8-bom", true},
		{"no-header", "1.2345,2026-01-01 10:11:12\n2.2,2026-01-02 10:11:12\n", "utf-8", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := writeImportTestFile(t, "numbers.csv", []byte(test.input))
			file, err := AnalyzeImportFile(context.Background(), path, "numbers.csv", Actor{Locale: "en", Timezone: "UTC"})
			if err != nil {
				t.Fatal(err)
			}
			if file.Analysis.Encoding != test.encoding || file.Analysis.HasHeader != test.header {
				t.Fatalf("analysis = %+v", file.Analysis)
			}
			if got := file.Analysis.Columns[0]; got.UIType != "decimal" || !strings.Contains(string(got.Options), `"scale":4`) {
				t.Errorf("decimal = %+v", got)
			}
			if got := file.Analysis.Columns[1].UIType; got != "datetime" {
				t.Errorf("datetime = %q", got)
			}
		})
	}
}

func TestImportFileXLSXAndPackage(t *testing.T) {
	for _, withManifest := range []bool{false, true} {
		t.Run(fmt.Sprint(withManifest), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "sample.xlsx")
			book := excelize.NewFile()
			if err := book.SetSheetName("Sheet1", "Data"); err != nil {
				t.Fatal(err)
			}
			header := []any{"Name", "Count"}
			if withManifest {
				header = []any{"_id", "Name", "Count"}
				if _, err := book.NewSheet("_pandatable"); err != nil {
					t.Fatal(err)
				}
				manifest := tablePackageManifest{
					Format: "pandatable.table", FormatVersion: 1, RowIDColumn: true,
					Table: packageTable{Name: "Original"},
					Fields: []packageField{
						{Key: "f1", Name: "Name", UIType: "text", Options: json.RawMessage(`{}`)},
						{Key: "f2", Name: "Count", UIType: "integer", Options: json.RawMessage(`{}`)},
					},
				}
				raw, err := json.Marshal(manifest)
				if err != nil {
					t.Fatal(err)
				}
				if err := book.SetCellStr("_pandatable", "A1", string(raw)); err != nil {
					t.Fatal(err)
				}
			}
			if err := book.SetSheetRow("Data", "A1", &header); err != nil {
				t.Fatal(err)
			}
			values := []any{"Alpha", 2}
			if withManifest {
				values = []any{"1", "Alpha", 2}
			}
			if err := book.SetSheetRow("Data", "A2", &values); err != nil {
				t.Fatal(err)
			}
			if err := book.SaveAs(path); err != nil {
				t.Fatal(err)
			}
			if err := book.Close(); err != nil {
				t.Fatal(err)
			}
			file, err := AnalyzeImportFile(context.Background(), path, "sample.xlsx", Actor{Locale: "en"})
			if err != nil {
				t.Fatal(err)
			}
			if len(file.Analysis.Columns) != 2 || file.Analysis.SampledRows != 1 {
				t.Fatalf("analysis = %+v", file.Analysis)
			}
			if withManifest && (file.Analysis.Format != "package" || file.Analysis.TableName != "Original" ||
				file.Analysis.Columns[0].PackageKey != "f1") {
				t.Fatalf("package = %+v", file.Analysis)
			}
			if !withManifest && file.Analysis.Format != "xlsx" {
				t.Fatalf("spreadsheet = %+v", file.Analysis)
			}
		})
	}
}
