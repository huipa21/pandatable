package structure

import (
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"

	"pandatable/internal/codec"
)

const (
	importSampleRows       = 1000
	maxImportColumns       = 1500
	maxImportRows          = 500000
	maxImportManifestBytes = 4 << 20
	maxImportUnzippedBytes = 2 << 30
	importXMLMemoryBytes   = 8 << 20
)

type ImportColumn struct {
	Index          int             `json:"index"`
	Name           string          `json:"name"`
	UIType         string          `json:"uiType"`
	Options        json.RawMessage `json:"options"`
	Examples       []string        `json:"examples,omitempty"`
	PackageKey     string          `json:"packageKey,omitempty"`
	LinkTarget     string          `json:"linkTarget,omitempty"`
	RelinkTargetID string          `json:"relinkTargetId,omitempty"`
	TargetFieldID  string          `json:"targetFieldId,omitempty"`
}

type ImportAnalysis struct {
	Format      string         `json:"format"`
	Encoding    string         `json:"encoding,omitempty"`
	Delimiter   string         `json:"delimiter,omitempty"`
	HasHeader   bool           `json:"hasHeader"`
	SampledRows int            `json:"sampledRows"`
	TableName   string         `json:"tableName,omitempty"`
	Columns     []ImportColumn `json:"columns"`
}

// ImportFile contains server-side analysis and the path to a private staged
// upload. The path and package manifest are never returned to the browser.
type ImportFile struct {
	Analysis ImportAnalysis
	path     string
	sheet    string
	manifest *tablePackageManifest
}

type importRows interface {
	Next() ([]string, error)
	Close() error
}

type csvImportRows struct {
	file     *os.File
	reader   *csv.Reader
	encoding string
}

func (rows *csvImportRows) Next() ([]string, error) {
	record, err := rows.reader.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, io.EOF
		}
		return nil, validation("The CSV file is malformed")
	}
	if len(record) > maxImportColumns {
		return nil, validation("The import contains too many columns")
	}
	for _, value := range record {
		if !utf8.ValidString(value) || rows.encoding == "gbk" && strings.ContainsRune(value, utf8.RuneError) {
			return nil, validation("The CSV file contains invalid text encoding")
		}
	}
	return record, nil
}

func (rows *csvImportRows) Close() error { return rows.file.Close() }

type spreadsheetImportRows struct {
	book *excelize.File
	rows *excelize.Rows
	raw  bool
}

func (rows *spreadsheetImportRows) Next() ([]string, error) {
	if !rows.rows.Next() {
		if err := rows.rows.Error(); err != nil {
			return nil, fmt.Errorf("read spreadsheet rows: %w", err)
		}
		return nil, io.EOF
	}
	record, err := rows.rows.Columns(excelize.Options{RawCellValue: rows.raw})
	if err != nil {
		return nil, fmt.Errorf("read spreadsheet cells: %w", err)
	}
	if len(record) > maxImportColumns+1 {
		return nil, validation("The import contains too many columns")
	}
	return record, nil
}

func (rows *spreadsheetImportRows) Close() error {
	rowErr := rows.rows.Close()
	bookErr := rows.book.Close()
	if rowErr != nil {
		return rowErr
	}
	return bookErr
}

func openSpreadsheet(path string) (*excelize.File, error) {
	book, err := excelize.OpenFile(path, excelize.Options{
		UnzipSizeLimit: maxImportUnzippedBytes, UnzipXMLSizeLimit: importXMLMemoryBytes,
	})
	if err != nil {
		return nil, validation("The XLSX file is invalid or exceeds the supported archive size")
	}
	return book, nil
}

func (file *ImportFile) openRows() (importRows, error) {
	if file.Analysis.Format == "csv" {
		input, err := os.Open(file.path)
		if err != nil {
			return nil, fmt.Errorf("open staged CSV: %w", err)
		}
		var source io.Reader = input
		if file.Analysis.Encoding == "utf-8-bom" {
			if _, err := input.Seek(3, io.SeekStart); err != nil {
				_ = input.Close()
				return nil, fmt.Errorf("skip CSV BOM: %w", err)
			}
		} else if file.Analysis.Encoding == "gbk" {
			source = transform.NewReader(input, simplifiedchinese.GBK.NewDecoder())
		}
		reader := csv.NewReader(source)
		reader.FieldsPerRecord = -1
		reader.Comma = []rune(file.Analysis.Delimiter)[0]
		return &csvImportRows{file: input, reader: reader, encoding: file.Analysis.Encoding}, nil
	}
	book, err := openSpreadsheet(file.path)
	if err != nil {
		return nil, err
	}
	rows, err := book.Rows(file.sheet)
	if err != nil {
		_ = book.Close()
		return nil, validation("The spreadsheet data sheet is invalid")
	}
	return &spreadsheetImportRows{book: book, rows: rows, raw: file.Analysis.Format == "package"}, nil
}

// AnalyzeImportFile inspects at most 1,000 data rows; the staged file remains
// on disk and is reopened for the import rather than retained in memory.
func AnalyzeImportFile(ctx context.Context, path, filename string, actor Actor) (*ImportFile, error) {
	input, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open staged import: %w", err)
	}
	var signature [4]byte
	n, readErr := io.ReadFull(input, signature[:])
	closeErr := input.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) && !errors.Is(readErr, io.ErrUnexpectedEOF) {
		return nil, fmt.Errorf("read import signature: %w", readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close staged import: %w", closeErr)
	}
	file := &ImportFile{path: path}
	if n == 4 && string(signature[:]) == "PK\x03\x04" || strings.HasSuffix(strings.ToLower(filename), ".xlsx") {
		if err := file.analyzeSpreadsheet(); err != nil {
			return nil, err
		}
	} else {
		encoding, err := detectCSVEncoding(ctx, path)
		if err != nil {
			return nil, err
		}
		file.Analysis = ImportAnalysis{Format: "csv", Encoding: encoding, Delimiter: ","}
		delimiter, err := detectCSVDelimiter(file)
		if err != nil {
			return nil, err
		}
		file.Analysis.Delimiter = string(delimiter)
	}
	if err := file.analyzeRows(ctx, actor); err != nil {
		return nil, err
	}
	return file, nil
}

func detectCSVEncoding(ctx context.Context, path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open CSV for encoding detection: %w", err)
	}
	defer file.Close()
	reader := bufio.NewReader(file)
	prefix, _ := reader.Peek(3)
	if string(prefix) == "\xef\xbb\xbf" {
		return "utf-8-bom", nil
	}
	for count := 0; ; count++ {
		if count%8192 == 0 {
			if err := ctx.Err(); err != nil {
				return "", err
			}
		}
		value, size, err := reader.ReadRune()
		if errors.Is(err, io.EOF) {
			return "utf-8", nil
		}
		if err != nil {
			return "", fmt.Errorf("detect CSV encoding: %w", err)
		}
		if value == utf8.RuneError && size == 1 {
			return "gbk", nil
		}
	}
}

func detectCSVDelimiter(file *ImportFile) (rune, error) {
	best, bestScore := ',', -1
	for _, delimiter := range []rune{',', '\t', ';', '|'} {
		file.Analysis.Delimiter = string(delimiter)
		rows, err := file.openRows()
		if err != nil {
			return 0, err
		}
		counts := make(map[int]int)
		for range 20 {
			record, readErr := rows.Next()
			if errors.Is(readErr, io.EOF) {
				break
			}
			if readErr != nil {
				break
			}
			counts[len(record)]++
		}
		if err := rows.Close(); err != nil {
			return 0, fmt.Errorf("close CSV delimiter sample: %w", err)
		}
		for width, count := range counts {
			if width > 1 {
				score := count*maxImportColumns + width
				if score > bestScore {
					best, bestScore = delimiter, score
				}
			}
		}
	}
	return best, nil
}

func (file *ImportFile) analyzeSpreadsheet() error {
	book, err := openSpreadsheet(file.path)
	if err != nil {
		return err
	}
	defer book.Close()
	sheets := book.GetSheetList()
	if len(sheets) == 0 {
		return validation("The spreadsheet has no sheets")
	}
	file.sheet = sheets[0]
	file.Analysis.Format = "xlsx"
	if index, err := book.GetSheetIndex("_pandatable"); err != nil {
		return validation("The spreadsheet sheet list is invalid")
	} else if index < 0 {
		return nil
	}
	rows, err := book.Rows("_pandatable")
	if err != nil {
		return validation("The package manifest is invalid")
	}
	var text strings.Builder
	for rows.Next() {
		cells, err := rows.Columns()
		if err != nil || len(cells) > 1 {
			_ = rows.Close()
			return validation("The package manifest is invalid")
		}
		if len(cells) == 1 {
			if text.Len()+len(cells[0]) > maxImportManifestBytes {
				_ = rows.Close()
				return validation("The package manifest is too large")
			}
			text.WriteString(cells[0])
		}
	}
	if err := rows.Error(); err != nil {
		_ = rows.Close()
		return validation("The package manifest is invalid")
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close package manifest: %w", err)
	}
	var manifest tablePackageManifest
	if err := json.Unmarshal([]byte(text.String()), &manifest); err != nil ||
		manifest.Format != "pandatable.table" || manifest.FormatVersion != 1 || !manifest.RowIDColumn ||
		len(manifest.Fields) == 0 || len(manifest.Fields) > maxImportColumns {
		return validation("The package manifest is invalid or unsupported")
	}
	if _, err := cleanName(manifest.Table.Name); err != nil {
		return validation("The package table name is invalid")
	}
	seen := make(map[string]bool, len(manifest.Fields))
	for _, field := range manifest.Fields {
		if field.Key == "" || seen[field.Key] || field.UIType == "" || !json.Valid(field.Options) {
			return validation("The package field metadata is invalid")
		}
		if _, err := cleanName(field.Name); err != nil {
			return validation("The package field name is invalid")
		}
		seen[field.Key] = true
	}
	if manifest.Table.DisplayField != nil && !seen[*manifest.Table.DisplayField] {
		return validation("The package display field is invalid")
	}
	if index, err := book.GetSheetIndex("Data"); err != nil || index < 0 {
		return validation("The package data sheet is missing")
	}
	file.manifest = &manifest
	file.sheet = "Data"
	file.Analysis.Format = "package"
	file.Analysis.TableName = manifest.Table.Name
	return nil
}

func (file *ImportFile) analyzeRows(ctx context.Context, actor Actor) error {
	rows, err := file.openRows()
	if err != nil {
		return err
	}
	defer rows.Close()
	first, err := rows.Next()
	if errors.Is(err, io.EOF) || err == nil && len(first) == 0 {
		return validation("The import file is empty")
	}
	if err != nil {
		return err
	}
	if file.manifest != nil {
		return file.analyzePackageRows(ctx, rows, first)
	}
	if len(first) > maxImportColumns {
		return validation("The import contains too many columns")
	}
	second, err := rows.Next()
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	file.Analysis.HasHeader = looksLikeHeader(first, second)
	columns := make([]ImportColumn, len(first))
	guesses := make([]*importGuess, len(first))
	used := make(map[string]bool)
	for _, field := range systemFields {
		used[strings.ToLower(field.label(labelsFor(actor.Locale)))] = true
	}
	for index := range first {
		name := ""
		if file.Analysis.HasHeader {
			name = first[index]
		}
		name = importColumnName(name, index, actor.Locale, used)
		columns[index] = ImportColumn{Index: index, Name: name}
		guesses[index] = newImportGuess()
	}
	location, err := importLocation(actor.Timezone)
	if err != nil {
		return err
	}
	consume := func(record []string) error {
		if len(record) > len(columns) {
			return validation("A row has more columns than the first row")
		}
		for index, value := range record {
			guesses[index].add(value, codec.Env{Location: location})
		}
		file.Analysis.SampledRows++
		return nil
	}
	if !file.Analysis.HasHeader {
		if err := consume(first); err != nil {
			return err
		}
	}
	if second != nil {
		if err := consume(second); err != nil {
			return err
		}
	}
	for file.Analysis.SampledRows < importSampleRows {
		if err := ctx.Err(); err != nil {
			return err
		}
		record, err := rows.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if err := consume(record); err != nil {
			return err
		}
	}
	for index, guess := range guesses {
		columns[index].UIType, columns[index].Options = guess.result()
		columns[index].Examples = guess.examples
	}
	file.Analysis.Columns = columns
	return nil
}

func (file *ImportFile) analyzePackageRows(ctx context.Context, rows importRows, header []string) error {
	if len(header) != len(file.manifest.Fields)+1 || header[0] != "_id" {
		return validation("The package data header does not match the manifest")
	}
	file.Analysis.HasHeader = true
	file.Analysis.Columns = make([]ImportColumn, len(file.manifest.Fields))
	for index, field := range file.manifest.Fields {
		if header[index+1] != field.Name {
			return validation("The package data header does not match the manifest")
		}
		column := ImportColumn{Index: index + 1, Name: field.Name, UIType: field.UIType,
			Options: field.Options, PackageKey: field.Key}
		if field.UIType == "link" {
			var options struct {
				TargetTable *string `json:"targetTable"`
			}
			if json.Unmarshal(field.Options, &options) != nil {
				return validation("The package link metadata is invalid")
			}
			if options.TargetTable != nil {
				column.LinkTarget = *options.TargetTable
			}
		}
		file.Analysis.Columns[index] = column
	}
	for file.Analysis.SampledRows < importSampleRows {
		if err := ctx.Err(); err != nil {
			return err
		}
		record, err := rows.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if len(record) > len(header) {
			return validation("A package row has more columns than its manifest")
		}
		file.Analysis.SampledRows++
	}
	return nil
}

func importLocation(zone string) (*time.Location, error) {
	if zone == "" {
		return time.UTC, nil
	}
	location, err := codec.LoadLocation(zone)
	if err != nil {
		return nil, validation("The import timezone is invalid")
	}
	return location, nil
}

func looksLikeHeader(first, second []string) bool {
	if len(second) == 0 {
		return true
	}
	seen := make(map[string]bool, len(first))
	for _, value := range first {
		name := strings.TrimSpace(value)
		if name == "" || seen[strings.ToLower(name)] {
			return false
		}
		seen[strings.ToLower(name)] = true
		for _, kind := range []codec.Kind{codec.Integer, codec.Decimal, codec.Date, codec.DateTime, codec.Checkbox} {
			field, err := codec.Bind(kind, json.RawMessage(`{}`))
			if err == nil {
				if _, err := field.ParsePaste(name, codec.Env{Location: time.UTC}); err == nil {
					return false
				}
			}
		}
	}
	return true
}

func importColumnName(value string, index int, locale string, used map[string]bool) string {
	name, err := cleanName(value)
	if err != nil {
		if locale == "zh-CN" {
			name = "列 " + strconv.Itoa(index+1)
		} else {
			name = "Column " + strconv.Itoa(index+1)
		}
	}
	base := []rune(name)
	for suffix := 2; used[strings.ToLower(name)]; suffix++ {
		addition := " " + strconv.Itoa(suffix)
		limit := maxNameRunes - utf8.RuneCountInString(addition)
		name = string(base[:min(len(base), limit)]) + addition
	}
	used[strings.ToLower(name)] = true
	return name
}

type importGuess struct {
	count       int
	possible    map[codec.Kind]bool
	fields      map[codec.Kind]codec.Field
	decimals    []codec.Field
	decimalOK   []bool
	choices     map[string]string
	choiceOrder []string
	choiceValid bool
	repeated    bool
	examples    []string
}

func newImportGuess() *importGuess {
	guess := &importGuess{
		possible: make(map[codec.Kind]bool), fields: make(map[codec.Kind]codec.Field),
		choices: make(map[string]string), decimals: make([]codec.Field, 39), decimalOK: make([]bool, 39),
		choiceValid: true,
	}
	for _, kind := range []codec.Kind{codec.Integer, codec.Date, codec.DateTime, codec.Checkbox} {
		options := json.RawMessage(`{}`)
		if kind == codec.Integer {
			options = json.RawMessage(`{"size":"big"}`)
		}
		field, err := codec.Bind(kind, options)
		if err == nil {
			guess.fields[kind], guess.possible[kind] = field, true
		}
	}
	for scale := range guess.decimals {
		options := json.RawMessage(fmt.Sprintf(`{"precision":38,"scale":%d}`, scale))
		field, err := codec.Bind(codec.Decimal, options)
		if err == nil {
			guess.decimals[scale], guess.decimalOK[scale] = field, true
		}
	}
	return guess
}

func (guess *importGuess) add(value string, env codec.Env) {
	if strings.TrimSpace(value) == "" {
		return
	}
	guess.count++
	if len(guess.examples) < 3 {
		end, count := len(value), 0
		for index := range value {
			if count == 120 {
				end = index
				break
			}
			count++
		}
		guess.examples = append(guess.examples, value[:end])
	}
	for kind, possible := range guess.possible {
		if possible {
			_, err := guess.fields[kind].ParsePaste(value, env)
			guess.possible[kind] = err == nil
		}
	}
	for scale, possible := range guess.decimalOK {
		if possible {
			_, err := guess.decimals[scale].ParsePaste(value, env)
			guess.decimalOK[scale] = err == nil
		}
	}
	label := strings.TrimSpace(value)
	if !guess.choiceValid || utf8.RuneCountInString(label) > 100 || strings.Contains(label, ",") {
		guess.choiceValid = false
		return
	}
	folded := strings.ToLower(label)
	if _, ok := guess.choices[folded]; ok {
		guess.repeated = true
	} else if len(guess.choices) <= 20 {
		guess.choices[folded] = label
		guess.choiceOrder = append(guess.choiceOrder, folded)
	}
}

func (guess *importGuess) result() (string, json.RawMessage) {
	if guess.count > 0 {
		if guess.possible[codec.Integer] {
			return string(codec.Integer), guess.fields[codec.Integer].Options()
		}
		for scale, possible := range guess.decimalOK {
			if possible {
				return string(codec.Decimal), guess.decimals[scale].Options()
			}
		}
		for _, kind := range []codec.Kind{codec.Date, codec.DateTime, codec.Checkbox} {
			if guess.possible[kind] {
				return string(kind), guess.fields[kind].Options()
			}
		}
		if guess.choiceValid && len(guess.choices) <= 20 && guess.repeated {
			choices := make([]codec.Choice, 0, len(guess.choiceOrder))
			for index, key := range guess.choiceOrder {
				choices = append(choices, codec.Choice{ID: "c" + strconv.Itoa(index+1),
					Label: guess.choices[key], Color: "gray"})
			}
			options, err := json.Marshal(struct {
				Choices []codec.Choice `json:"choices"`
			}{Choices: choices})
			if err == nil {
				if field, err := codec.Bind(codec.SingleSelect, options); err == nil {
					return string(codec.SingleSelect), field.Options()
				}
			}
		}
	}
	return string(codec.Text), json.RawMessage(`{}`)
}
