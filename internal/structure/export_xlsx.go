package structure

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"strconv"
	"time"
	"unicode/utf16"

	"github.com/xuri/excelize/v2"

	"pandatable/internal/codec"
)

const (
	maxSpreadsheetColumns   = excelize.MaxColumns
	maxSpreadsheetCellChars = excelize.TotalCellChars
	manifestChunkChars      = 32000
	maxZipEntryBytes        = int64(math.MaxUint32 - 1)
)

func (plan *exportPlan) writePackage(ctx context.Context, w io.Writer) (result error) {
	book := excelize.NewFile()
	defer func() {
		if err := book.Close(); err != nil && result == nil {
			result = fmt.Errorf("remove spreadsheet temporary files: %w", err)
		}
	}()
	if err := book.SetSheetName("Sheet1", "Data"); err != nil {
		return fmt.Errorf("name spreadsheet data sheet: %w", err)
	}
	book.SetActiveSheet(0)
	if _, err := book.NewSheet(XLSXMetaSheet); err != nil {
		return fmt.Errorf("create spreadsheet metadata sheet: %w", err)
	}
	if err := book.ProtectSheet(XLSXMetaSheet, &excelize.SheetProtectionOptions{
		SelectLockedCells: true, SelectUnlockedCells: true,
	}); err != nil {
		return fmt.Errorf("protect spreadsheet metadata: %w", err)
	}
	if _, err := book.NewSheet("_pandatable"); err != nil {
		return fmt.Errorf("create spreadsheet manifest sheet: %w", err)
	}
	if err := book.SetSheetVisible("_pandatable", false, true); err != nil {
		return fmt.Errorf("hide spreadsheet manifest: %w", err)
	}
	dateFormat := "yyyy-mm-dd"
	dateTimeFormat := "yyyy-mm-dd hh:mm:ss.000"
	timeFormat := "hh:mm:ss.000000"
	dateStyle, err := book.NewStyle(&excelize.Style{CustomNumFmt: &dateFormat})
	if err != nil {
		return fmt.Errorf("create spreadsheet date style: %w", err)
	}
	dateTimeStyle, err := book.NewStyle(&excelize.Style{CustomNumFmt: &dateTimeFormat})
	if err != nil {
		return fmt.Errorf("create spreadsheet datetime style: %w", err)
	}
	timeStyle, err := book.NewStyle(&excelize.Style{CustomNumFmt: &timeFormat})
	if err != nil {
		return fmt.Errorf("create spreadsheet time style: %w", err)
	}
	metadata, err := book.NewStreamWriter(XLSXMetaSheet)
	if err != nil {
		return fmt.Errorf("create spreadsheet metadata writer: %w", err)
	}
	if err := metadata.SetPanes(&excelize.Panes{
		Freeze: true, YSplit: XLSXMetaHeaderRow, TopLeftCell: "A6", ActivePane: "bottomLeft",
	}); err != nil {
		return fmt.Errorf("freeze spreadsheet metadata header: %w", err)
	}
	info := [][]any{
		{"format_version", plan.metadata.FormatVersion, "source_instance_id", plan.metadata.SourceInstanceID},
		{"table_id", plan.metadata.TableID, "table_name", plan.metadata.TableName},
		{"schema_version", plan.metadata.SchemaVersion, "exported_at", plan.metadata.ExportedAt.Format(time.RFC3339Nano)},
		{"datetime_timezone", plan.metadata.DatetimeTimezone},
	}
	for index, row := range info {
		if err := metadata.SetRow("A"+strconv.Itoa(index+1), row); err != nil {
			return fmt.Errorf("write spreadsheet metadata source: %w", err)
		}
	}
	headers := XLSXMetaColumns()
	headerValues := make([]any, len(headers))
	for index, header := range headers {
		headerValues[index] = header
	}
	if err := metadata.SetRow("A"+strconv.Itoa(XLSXMetaHeaderRow), headerValues); err != nil {
		return fmt.Errorf("write spreadsheet metadata header: %w", err)
	}
	for index, field := range plan.metadata.Fields {
		if err := metadata.SetRow("A"+strconv.Itoa(XLSXMetaFirstFieldRow+index), field.Cells()); err != nil {
			return fmt.Errorf("write spreadsheet metadata field: %w", err)
		}
	}
	if err := metadata.Flush(); err != nil {
		return fmt.Errorf("flush spreadsheet metadata: %w", err)
	}
	manifest, err := book.NewStreamWriter("_pandatable")
	if err != nil {
		return fmt.Errorf("create manifest writer: %w", err)
	}
	for index, chunk := range splitPackageManifest(plan.manifest) {
		cell, err := excelize.CoordinatesToCellName(1, index+1)
		if err != nil {
			return fmt.Errorf("address manifest cell: %w", err)
		}
		if err := manifest.SetRow(cell, []any{chunk}); err != nil {
			return fmt.Errorf("write manifest cell: %w", err)
		}
	}
	if err := manifest.Flush(); err != nil {
		return fmt.Errorf("flush spreadsheet manifest: %w", err)
	}
	data, err := book.NewStreamWriter("Data")
	if err != nil {
		return fmt.Errorf("create spreadsheet data writer: %w", err)
	}
	header := make([]any, len(plan.fields)+1)
	header[0] = "_id"
	for index, field := range plan.fields {
		if len(utf16.Encode([]rune(field.name))) > maxSpreadsheetCellChars {
			return validation("A field name is too long for an XLSX package")
		}
		header[index+1] = field.name
	}
	if err := data.SetRow("A1", header); err != nil {
		return fmt.Errorf("write spreadsheet header: %w", err)
	}
	rowNumber := 2
	if err := plan.eachBatch(ctx, func(rows []exportRow, labels map[string]string) error {
		for _, row := range rows {
			if rowNumber > excelize.TotalRows {
				return validation("Table has too many rows for an XLSX package")
			}
			values := make([]any, len(plan.fields)+1)
			values[0] = row.record.ID
			for index, field := range plan.fields {
				value, err := plan.spreadsheetValue(row, field, labels)
				if err != nil {
					return err
				}
				if date, ok := value.(spreadsheetDate); ok {
					if date.value.Year() < 1900 {
						if date.dateTime {
							value = date.value.Format(time.RFC3339Nano)
						} else {
							value = date.value.Format("2006-01-02")
						}
					} else if date.dateTime {
						value = excelize.Cell{StyleID: dateTimeStyle, Value: date.value}
					} else {
						value = excelize.Cell{StyleID: dateStyle, Value: date.value}
					}
				}
				if field.kind == codec.Time && value != nil {
					value = excelize.Cell{StyleID: timeStyle, Value: value}
				}
				values[index+1] = value
			}
			if err := data.SetRow("A"+strconv.Itoa(rowNumber), values); err != nil {
				return fmt.Errorf("write spreadsheet row: %w", err)
			}
			rowNumber++
		}
		return nil
	}); err != nil {
		return err
	}
	if err := data.Flush(); err != nil {
		return fmt.Errorf("flush spreadsheet rows: %w", err)
	}
	// Excelize's WriteTo otherwise buffers the entire ZIP; its public ZIP writer
	// hook lets the archive flow directly to the response instead.
	book.SetZipWriter(func(_ io.Writer) excelize.ZipWriter {
		return &streamingZipWriter{writer: zip.NewWriter(w)}
	})
	if err := book.Write(io.Discard); err != nil {
		return fmt.Errorf("write spreadsheet archive: %w", err)
	}
	return nil
}

func splitPackageManifest(raw []byte) []string {
	text := string(raw)
	chunks := make([]string, 0, len(text)/manifestChunkChars+1)
	start, units := 0, 0
	for index, value := range text {
		size := 1
		if value > 0xffff {
			size = 2
		}
		if units+size > manifestChunkChars {
			chunks = append(chunks, text[start:index])
			start, units = index, 0
		}
		units += size
	}
	return append(chunks, text[start:])
}

type streamingZipWriter struct {
	writer *zip.Writer
}

func (writer *streamingZipWriter) Create(name string) (io.Writer, error) {
	entry, err := writer.writer.Create(name)
	if err != nil {
		return nil, err
	}
	return &boundedZipEntry{writer: entry}, nil
}

func (writer *streamingZipWriter) AddFS(fs.FS) error {
	return errors.New("adding a filesystem to an export archive is unsupported")
}

func (writer *streamingZipWriter) Close() error {
	return writer.writer.Close()
}

// Excelize's ZIP64 header repair requires a complete in-memory ZIP. Reject an
// oversized individual entry rather than emit an invalid streaming archive.
type boundedZipEntry struct {
	writer io.Writer
	size   int64
}

func (entry *boundedZipEntry) Write(p []byte) (int, error) {
	if int64(len(p)) > maxZipEntryBytes-entry.size {
		return 0, validation("An XLSX sheet exceeds the supported ZIP entry size")
	}
	n, err := entry.writer.Write(p)
	entry.size += int64(n)
	return n, err
}
