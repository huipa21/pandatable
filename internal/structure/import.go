package structure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/acl"
	"pandatable/internal/apperr"
	"pandatable/internal/codec"
	"pandatable/internal/ddl"
	"pandatable/internal/events"
	"pandatable/internal/structure/structuresql"
)

const (
	importBatchRows  = 5000
	importBatchBytes = 16 << 20
	maxImportIssues  = 1000
)

type ImportTarget struct {
	Kind string
	ID   string
}

type ImportMapping struct {
	Index         int             `json:"index"`
	Name          string          `json:"name"`
	UIType        string          `json:"uiType"`
	Options       json.RawMessage `json:"options"`
	TargetFieldID string          `json:"targetFieldId"`
	Skip          bool            `json:"skip"`
	Relink        bool            `json:"relink"`
}

type ImportRequest struct {
	Name      string          `json:"name"`
	HasHeader bool            `json:"hasHeader"`
	Columns   []ImportMapping `json:"columns"`
}

type ImportIssue struct {
	Row    int    `json:"row"`
	Column string `json:"column"`
	Value  string `json:"value"`
}

type ImportResult struct {
	TableID          string        `json:"tableId,omitempty"`
	ViewID           string        `json:"viewId,omitempty"`
	Rows             int           `json:"rows"`
	InvalidCells     int           `json:"invalidCells"`
	ConvertedLinks   []string      `json:"convertedLinks,omitempty"`
	UnavailableViews []string      `json:"unavailableViews,omitempty"`
	Issues           []ImportIssue `json:"-"`
	issueColumns     []int
}

func (result *ImportResult) issue(row, columnIndex int, column, value string) {
	result.InvalidCells++
	issue := ImportIssue{Row: row, Column: column, Value: value}
	count := len(result.Issues)
	if count == 0 || result.Issues[count-1].Row < row ||
		result.Issues[count-1].Row == row && result.issueColumns[count-1] <= columnIndex {
		if count < maxImportIssues {
			result.Issues = append(result.Issues, issue)
			result.issueColumns = append(result.issueColumns, columnIndex)
		}
		return
	}
	// User and link validation can discover earlier cells after later parse errors.
	index := sort.Search(count, func(i int) bool {
		return result.Issues[i].Row > row ||
			result.Issues[i].Row == row && result.issueColumns[i] > columnIndex
	})
	if count < maxImportIssues {
		result.Issues = append(result.Issues, ImportIssue{})
		result.issueColumns = append(result.issueColumns, 0)
		count++
	}
	copy(result.Issues[index+1:], result.Issues[index:count-1])
	copy(result.issueColumns[index+1:], result.issueColumns[index:count-1])
	result.Issues[index], result.issueColumns[index] = issue, columnIndex
}

func importResource(target ImportTarget) (acl.Resource, acl.Level, error) {
	switch target.Kind {
	case "book":
		resource, err := resourceOf(acl.Book, target.ID)
		return resource, acl.Owner, err
	case "table":
		resource, err := resourceOf(acl.Table, target.ID)
		return resource, acl.Write, err
	default:
		return acl.Resource{}, acl.None, validation("Import target is invalid")
	}
}

// CheckImportAccess is repeated when a job starts and for each committed append
// batch; creating a table retains the book-owner requirement of CreateTable.
func (s *Service) CheckImportAccess(ctx context.Context, actor Actor, target ImportTarget) error {
	resource, level, err := importResource(target)
	if err != nil {
		return err
	}
	user, ok := acl.ParseUUID(actor.ID)
	if !ok {
		return acl.ErrValidation()
	}
	return s.authorizeRows(ctx, s.pool, user, resource, level)
}

// PrepareImport adds append mapping suggestions without exposing inaccessible
// table metadata. The caller has already staged and analyzed the upload.
func (s *Service) PrepareImport(ctx context.Context, actor Actor, target ImportTarget, file *ImportFile) error {
	if file == nil {
		return validation("Import analysis is missing")
	}
	if err := s.CheckImportAccess(ctx, actor, target); err != nil {
		return err
	}
	bookID := target.ID
	if target.Kind == "table" {
		schema, err := s.Schema(ctx, actor, target.ID)
		if err != nil {
			return err
		}
		bookID = schema.Table.BookID
		for index := range file.Analysis.Columns {
			column := &file.Analysis.Columns[index]
			for _, field := range schema.Fields {
				if !field.IsSystem && strings.EqualFold(field.Name, column.Name) &&
					field.UIType == column.UIType && !codec.IsSystem(codec.Kind(field.UIType)) &&
					field.UIType != "formula" && !isComputedKind(field.UIType) && field.UIType != "autonumber" {
					column.TargetFieldID = field.ID
					break
				}
			}
		}
	}
	if file.manifest != nil {
		return s.preparePackageLinks(ctx, actor, target, bookID, file)
	}
	return nil
}

type importColumnPlan struct {
	index  int
	name   string
	field  rowField
	source ImportColumn
}

type importRecord struct {
	number int
	values []string
}

type importUserCell struct {
	row, position, columnIndex int
	id                         pgtype.UUID
	column, value              string
}

type importBatchReader struct {
	rows          importRows
	number        int
	count         int
	expectedWidth int
	pending       *importRecord
	done          bool
}

func (reader *importBatchReader) next(ctx context.Context) ([]importRecord, error) {
	batch := make([]importRecord, 0, importBatchRows)
	size := 0
	if reader.pending != nil {
		batch = append(batch, *reader.pending)
		for _, value := range reader.pending.values {
			size += len(value)
		}
		reader.pending = nil
	}
	for len(batch) < importBatchRows && !reader.done {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		values, err := reader.rows.Next()
		if errors.Is(err, io.EOF) {
			reader.done = true
			break
		}
		if err != nil {
			return nil, err
		}
		if len(values) > reader.expectedWidth {
			return nil, validation("An import row has more columns than the analyzed file")
		}
		reader.number++
		reader.count++
		if reader.count > maxImportRows {
			return nil, apperr.New(400, "ERR_IMPORT_ROWS_LIMIT", "Import exceeds the row limit")
		}
		record := importRecord{number: reader.number, values: values}
		bytes := 0
		for _, value := range values {
			bytes += len(value)
		}
		if len(batch) > 0 && size+bytes > importBatchBytes {
			reader.pending = &record
			break
		}
		size += bytes
		batch = append(batch, record)
	}
	return batch, nil
}

func validateImportMappings(file *ImportFile, request ImportRequest) (map[int]ImportMapping, error) {
	if len(request.Columns) != len(file.Analysis.Columns) {
		return nil, validation("Every import column requires a mapping")
	}
	valid := make(map[int]bool, len(file.Analysis.Columns))
	for _, column := range file.Analysis.Columns {
		valid[column.Index] = true
	}
	mappings := make(map[int]ImportMapping, len(request.Columns))
	for _, mapping := range request.Columns {
		if !valid[mapping.Index] {
			return nil, validation("Import mapping names an unknown column")
		}
		if _, exists := mappings[mapping.Index]; exists {
			return nil, validation("Import columns cannot be mapped twice")
		}
		mappings[mapping.Index] = mapping
	}
	return mappings, nil
}

// RunImport creates a new table in one transaction or commits an append one
// batch at a time. Progress is reported after each successful COPY.
func (s *Service) RunImport(ctx context.Context, actor Actor, target ImportTarget, file *ImportFile,
	request ImportRequest, progress func(ImportResult),
) (ImportResult, error) {
	if file == nil {
		return ImportResult{}, validation("Import analysis is missing")
	}
	if err := s.CheckImportAccess(ctx, actor, target); err != nil {
		return ImportResult{}, err
	}
	mappings, err := validateImportMappings(file, request)
	if err != nil {
		return ImportResult{}, err
	}
	if file.manifest != nil {
		return s.runPackageImport(ctx, actor, target, file, request, mappings, progress)
	}
	rows, err := file.openRows()
	if err != nil {
		return ImportResult{}, err
	}
	defer rows.Close()
	reader := &importBatchReader{rows: rows, expectedWidth: len(file.Analysis.Columns)}
	if request.HasHeader {
		if _, err := rows.Next(); err != nil {
			return ImportResult{}, validation("The import header is missing")
		}
		reader.number = 1
	}
	if target.Kind == "book" {
		return s.runNewPlainImport(ctx, actor, target.ID, file, request, mappings, reader, progress)
	}
	return s.runAppendImport(ctx, actor, target.ID, file, mappings, reader, progress)
}

func (s *Service) runNewPlainImport(ctx context.Context, actor Actor, bookID string, file *ImportFile,
	request ImportRequest, mappings map[int]ImportMapping, reader *importBatchReader, progress func(ImportResult),
) (ImportResult, error) {
	name, err := cleanName(request.Name)
	if err != nil {
		return ImportResult{}, err
	}
	target, err := resourceOf(acl.Book, bookID)
	if err != nil {
		return ImportResult{}, err
	}
	result := ImportResult{}
	err = retryCollisions(func() error {
		result = ImportResult{}
		return s.run(ctx, actor, scope{target: target, affectsACL: true}, func(c *txn) error {
			tableID, physical, systemIDs, err := s.createImportTable(ctx, c, target.ID, name, nil)
			if err != nil {
				return err
			}
			var display pgtype.UUID
			fieldIDs := make(map[int]pgtype.UUID)
			for _, column := range file.Analysis.Columns {
				mapping := mappings[column.Index]
				if mapping.Skip {
					continue
				}
				fieldName, err := cleanName(mapping.Name)
				if err != nil {
					return err
				}
				if codec.IsSystem(codec.Kind(mapping.UIType)) || mapping.UIType == "link" ||
					mapping.UIType == "formula" || isComputedKind(mapping.UIType) || mapping.UIType == "autonumber" {
					return validation("Plain imports require writable field types")
				}
				fieldID, err := s.createImportField(ctx, c, tableID, physical, FieldInput{
					Name: fieldName, UIType: mapping.UIType, Options: mapping.Options,
				})
				if err != nil {
					return err
				}
				fieldIDs[column.Index] = fieldID
				if !display.Valid && displayEligible(mapping.UIType, false) {
					display = fieldID
				}
			}
			if !display.Valid {
				return validation("An imported table needs a displayable field")
			}
			viewID, err := createImportDefaultView(ctx, c, tableID, display, systemIDs)
			if err != nil {
				return err
			}
			table, err := loadImportRowTable(ctx, c, tableID, physical)
			if err != nil {
				return err
			}
			columns := make([]importColumnPlan, 0, len(fieldIDs))
			for _, column := range file.Analysis.Columns {
				if id, ok := fieldIDs[column.Index]; ok {
					columns = append(columns, importColumnPlan{index: column.Index, name: column.Name,
						field: table.byID[id.String()], source: column})
				}
			}
			result.TableID, result.ViewID = tableID.String(), viewID.String()
			for {
				batch, err := reader.next(ctx)
				if err != nil {
					return err
				}
				if len(batch) == 0 {
					break
				}
				if err := s.copyImportBatch(ctx, c.tx, actor, table, columns, nil, batch, false, false, &result); err != nil {
					return err
				}
				result.Rows += len(batch)
				if progress != nil {
					progress(result)
				}
			}
			return nil
		})
	})
	if err != nil {
		result.TableID, result.ViewID, result.Rows = "", "", 0
	}
	return result, rowDatabaseError(err)
}

func (s *Service) runAppendImport(ctx context.Context, actor Actor, tableID string, file *ImportFile,
	mappings map[int]ImportMapping, reader *importBatchReader, progress func(ImportResult),
) (ImportResult, error) {
	result := ImportResult{TableID: tableID}
	if err := s.withWritableTable(ctx, actor, tableID, func(_ pgx.Tx, table rowTable) error {
		_, _, err := appendImportColumns(file, mappings, table)
		return err
	}); err != nil {
		return result, err
	}
	for {
		batch, err := reader.next(ctx)
		if err != nil {
			return result, err
		}
		if len(batch) == 0 {
			return result, nil
		}
		err = s.withWritableTable(ctx, actor, tableID, func(tx pgx.Tx, table rowTable) error {
			columns, links, err := appendImportColumns(file, mappings, table)
			if err != nil {
				return err
			}
			if err := s.copyImportBatch(ctx, tx, actor, table, columns, links, batch, file.manifest != nil, false, &result); err != nil {
				return err
			}
			if err := events.Emit(ctx, tx, events.Event{Type: "row.bulk", TableID: tableID, By: actor.ID, ByName: actor.Name}); err != nil {
				return err
			}
			return emitRelatedRowInvalidations(ctx, tx, table.id)
		})
		if err != nil {
			return result, err
		}
		result.Rows += len(batch)
		if progress != nil {
			progress(result)
		}
	}
}

func appendImportColumns(file *ImportFile, mappings map[int]ImportMapping, table rowTable) ([]importColumnPlan, []importLinkPlan, error) {
	columns := make([]importColumnPlan, 0, len(mappings))
	links := make([]importLinkPlan, 0)
	used := make(map[string]bool)
	for _, column := range file.Analysis.Columns {
		mapping := mappings[column.Index]
		if mapping.Skip {
			continue
		}
		field, found := table.byID[mapping.TargetFieldID]
		if !found || field.system || used[field.id] {
			return nil, nil, validation("Append mappings must name distinct writable fields")
		}
		used[field.id] = true
		if field.relation != nil {
			if file.manifest == nil || column.UIType != "link" || !mapping.Relink {
				return nil, nil, validation("A relation mapping requires package re-linking")
			}
			links = append(links, importLinkPlan{index: column.Index, name: column.Name, field: field, source: column})
			continue
		}
		if field.codec == nil || field.codec.ReadOnly() {
			return nil, nil, validation("Append mappings must name distinct writable fields")
		}
		columns = append(columns, importColumnPlan{index: column.Index, name: column.Name, field: field, source: column})
	}
	if len(columns)+len(links) == 0 {
		return nil, nil, validation("At least one import column must be mapped")
	}
	return columns, links, nil
}

func (s *Service) createImportTable(ctx context.Context, c *txn, bookID pgtype.UUID, name string,
	systemNames map[codec.Kind]string,
) (pgtype.UUID, ddl.TableName, []string, error) {
	book, err := c.q.GetBook(ctx, bookID)
	if err != nil {
		return pgtype.UUID{}, ddl.TableName{}, nil, notFoundIfNoRows(err, "read import book")
	}
	physicalName, err := s.newName(ddl.Table)
	if err != nil {
		return pgtype.UUID{}, ddl.TableName{}, nil, err
	}
	last, err := c.q.MaxTablePosition(ctx, bookID)
	if err != nil {
		return pgtype.UUID{}, ddl.TableName{}, nil, fmt.Errorf("read import table position: %w", err)
	}
	tableID, err := c.q.InsertTable(ctx, structuresql.InsertTableParams{BookID: bookID, Name: name,
		PgName: physicalName, Position: last + positionStep, CreatedBy: c.user})
	if err != nil {
		return pgtype.UUID{}, ddl.TableName{}, nil, fmt.Errorf("insert import table: %w", err)
	}
	physical := ddl.TableName{Schema: book.PgSchema, Name: physicalName}
	if err := ddl.CreateTable(ctx, c.tx, physical); err != nil {
		return pgtype.UUID{}, ddl.TableName{}, nil, err
	}
	labels := labelsFor(c.actor.Locale)
	ids := make([]string, 0, len(systemFields))
	for index, field := range systemFields {
		fieldName := field.label(labels)
		if name, ok := systemNames[codec.Kind(field.column)]; ok {
			fieldName = name
		}
		id, err := c.q.InsertField(ctx, structuresql.InsertFieldParams{TableID: tableID, Name: fieldName,
			UiType: field.column, PgColumn: pgText(field.column), PgType: pgText(field.pgType),
			IsSystem: true, IsRequired: field.required, Position: systemFieldPosition + float64(index)})
		if err != nil {
			return pgtype.UUID{}, ddl.TableName{}, nil, fieldNameConflict(fmt.Errorf("insert import system field: %w", err))
		}
		ids = append(ids, id.String())
	}
	return tableID, physical, ids, nil
}

func (s *Service) createImportField(ctx context.Context, c *txn, tableID pgtype.UUID, physical ddl.TableName,
	input FieldInput,
) (pgtype.UUID, error) {
	name, err := cleanName(input.Name)
	if err != nil {
		return pgtype.UUID{}, err
	}
	field, err := bindUserKind(input.UIType, input.Options)
	if err != nil {
		return pgtype.UUID{}, err
	}
	if input.UIType == string(codec.Currency) {
		field, err = bindCreationCurrency(ctx, c.q, input.Options, field)
		if err != nil {
			return pgtype.UUID{}, err
		}
	}
	value, err := resolveDefault(field, input.Default)
	if err != nil {
		return pgtype.UUID{}, err
	}
	if err := checkConstraints(field, input.IsRequired, input.IsUnique, value.stored != nil); err != nil {
		return pgtype.UUID{}, err
	}
	if err := checkTimezone(ctx, c.q, field); err != nil {
		return pgtype.UUID{}, err
	}
	column := field.Column()
	physicalName, err := s.newName(ddl.Column)
	if err != nil {
		return pgtype.UUID{}, err
	}
	last, err := c.q.MaxUserFieldPosition(ctx, tableID)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("read import field position: %w", err)
	}
	id, err := c.q.InsertUserField(ctx, structuresql.InsertUserFieldParams{TableID: tableID,
		Name: name, UiType: input.UIType, PgColumn: pgText(physicalName), PgType: pgText(pgTypeName(column.Type)),
		Options: field.Options(), IsRequired: input.IsRequired, IsUnique: input.IsUnique,
		DefaultJson: value.stored, Position: last + positionStep})
	if err != nil {
		return pgtype.UUID{}, fieldNameConflict(fmt.Errorf("insert import field: %w", err))
	}
	if column.AutoNumber {
		err = ddl.AddAutoNumberColumn(ctx, c.tx, physical, physicalName)
	} else {
		spec := ddl.ColumnSpec{Name: physicalName, Type: column.Type, NotNull: column.NotNull, Default: column.Default}
		if column.HasRangeCheck {
			spec.RangeCheck = &ddl.IntRangeCheck{Min: column.CheckMin, Max: column.CheckMax}
		}
		if column.UserReference {
			spec.Reference = ddl.UserReference
		}
		err = ddl.AddColumn(ctx, c.tx, physical, spec)
	}
	if err != nil {
		return pgtype.UUID{}, err
	}
	if err := applyColumn(ctx, c, physicalColumn{table: physical, column: physicalName}, column.Type,
		columnState{}, columnState{required: input.IsRequired, unique: input.IsUnique, stored: value.stored},
		value, false, false); err != nil {
		return pgtype.UUID{}, err
	}
	return id, nil
}

func createImportDefaultView(ctx context.Context, c *txn, tableID, display pgtype.UUID, systemIDs []string) (pgtype.UUID, error) {
	if _, err := c.q.SetTableDisplayField(ctx, structuresql.SetTableDisplayFieldParams{ID: tableID, DisplayFieldID: display}); err != nil {
		return pgtype.UUID{}, notFoundIfNoRows(err, "set import display field")
	}
	config, err := defaultGridConfig(display.String(), systemIDs)
	if err != nil {
		return pgtype.UUID{}, err
	}
	viewID, err := c.q.InsertDefaultGridView(ctx, structuresql.InsertDefaultGridViewParams{
		TableID: tableID, Name: labelsFor(c.actor.Locale).grid, CreatedBy: c.user, Config: config,
	})
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("insert import default view: %w", err)
	}
	return viewID, nil
}

func loadImportRowTable(ctx context.Context, c *txn, tableID pgtype.UUID, physical ddl.TableName) (rowTable, error) {
	row, err := c.q.GetTable(ctx, tableID)
	if err != nil {
		return rowTable{}, notFoundIfNoRows(err, "read import table")
	}
	return loadRowTable(ctx, c.q, c.tx, tableID, physical.Schema, physical.Name, row.SchemaVersion)
}

func allocateImportIDs(ctx context.Context, tx pgx.Tx, table rowTable, count int) ([]int64, error) {
	rows, err := tx.Query(ctx, `SELECT pg_catalog.nextval(pg_catalog.pg_get_serial_sequence($1,'id'))
		FROM pg_catalog.generate_series(1,$2)`, table.qualifiedName(), count)
	if err != nil {
		return nil, fmt.Errorf("allocate import row ids: %w", err)
	}
	defer rows.Close()
	ids := make([]int64, 0, count)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("read import row id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate import row ids: %w", err)
	}
	if len(ids) != count {
		return nil, errors.New("import row id count mismatch")
	}
	return ids, nil
}

func (s *Service) copyImportBatch(ctx context.Context, tx pgx.Tx, actor Actor, table rowTable, columns []importColumnPlan,
	links []importLinkPlan, batch []importRecord, isPackage, preserveSystem bool, result *ImportResult,
) error {
	location, err := importLocation(actor.Timezone)
	if err != nil {
		return err
	}
	ids, err := importBatchIDs(ctx, tx, table, batch, preserveSystem)
	if err != nil {
		return err
	}
	user, ok := acl.ParseUUID(actor.ID)
	if !ok {
		return acl.ErrValidation()
	}
	names := make([]string, 0, len(columns)+3)
	names = append(names, "id")
	if !preserveSystem {
		names = append(names, "created_by", "updated_by")
	}
	for _, column := range columns {
		if preserveSystem && column.field.kind == codec.RowID {
			continue
		}
		names = append(names, column.field.column)
	}
	values := make([][]any, len(batch))
	var userCells []importUserCell
	for index, record := range batch {
		row := make([]any, 0, len(names))
		row = append(row, ids[index])
		if !preserveSystem {
			row = append(row, user, user)
		}
		for _, column := range columns {
			text := ""
			if column.index < len(record.values) {
				text = record.values[column.index]
			}
			if preserveSystem && column.field.kind == codec.RowID {
				if text != strconv.FormatInt(ids[index], 10) {
					return validation("The package row id does not match its system field")
				}
				continue
			}
			value, parseErr := parseImportCell(column, text, isPackage, codec.Env{Location: location})
			if parseErr != nil {
				result.issue(record.number, column.index, column.name, text)
				if column.field.codec.Column().NotNull {
					return apperr.New(400, "ERR_REQUIRED_FIELD", "An invalid value cannot be stored in a required field")
				}
				value = nil
			}
			if value == nil && column.field.codec.Column().NotNull {
				result.issue(record.number, column.index, column.name, text)
				return apperr.New(400, "ERR_REQUIRED_FIELD", "A required import value is missing")
			}
			if column.field.kind == codec.User && value != nil {
				id, ok := value.(pgtype.UUID)
				if !ok || !id.Valid {
					return errors.New("parsed import user has an invalid type")
				}
				userCells = append(userCells, importUserCell{row: index, position: len(row),
					columnIndex: column.index, id: id, column: column.name, value: text})
			}
			row = append(row, value)
		}
		values[index] = row
	}
	if err := validateImportUsers(ctx, tx, batch, values, userCells, result); err != nil {
		return err
	}
	copied, err := tx.CopyFrom(ctx, pgx.Identifier{table.schema, table.name}, names, pgx.CopyFromRows(values))
	if err != nil {
		return fmt.Errorf("copy import rows: %w", err)
	}
	if copied != int64(len(batch)) {
		return errors.New("import COPY row count mismatch")
	}
	var audit []importAudit
	if preserveSystem && len(links) > 0 {
		audit, err = readImportAudit(ctx, tx, table, ids)
		if err != nil {
			return err
		}
	}
	if err := s.applyImportLinks(ctx, tx, actor, table, links, batch, ids, result); err != nil {
		return err
	}
	if len(audit) > 0 {
		if err := restoreImportAudit(ctx, tx, table, audit); err != nil {
			return err
		}
	}
	query := "SELECT " + table.selectColumns(table.fields) + " FROM " + table.qualifiedName() +
		` WHERE "id" = ANY($1::bigint[]) ORDER BY "id"`
	selected, err := queryUncachedRows(ctx, tx, query, ids)
	if err != nil {
		return fmt.Errorf("read imported rows for history: %w", err)
	}
	records, err := scanRows(selected, table, table.fields)
	if err != nil {
		return err
	}
	if err := enrichRowLinks(ctx, tx, table, table.fields, records); err != nil {
		return err
	}
	history, err := snapshotHistories(table, records, actor, "insert")
	if err != nil {
		return err
	}
	return insertHistory(ctx, tx, history)
}

func validateImportUsers(ctx context.Context, tx pgx.Tx, batch []importRecord, values [][]any,
	cells []importUserCell, result *ImportResult,
) error {
	if len(cells) == 0 {
		return nil
	}
	wanted := make(map[pgtype.UUID]bool, len(cells))
	for _, cell := range cells {
		wanted[cell.id] = true
	}
	ids := make([]pgtype.UUID, 0, len(wanted))
	for id := range wanted {
		ids = append(ids, id)
	}
	rows, err := tx.Query(ctx, `SELECT id FROM meta.users WHERE id=ANY($1::uuid[]) FOR KEY SHARE`, ids)
	if err != nil {
		return fmt.Errorf("validate import user references: %w", err)
	}
	found := make(map[pgtype.UUID]bool, len(ids))
	for rows.Next() {
		var id pgtype.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return fmt.Errorf("read import user reference: %w", err)
		}
		found[id] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate import user references: %w", err)
	}
	for _, cell := range cells {
		if !found[cell.id] {
			values[cell.row][cell.position] = nil
			result.issue(batch[cell.row].number, cell.columnIndex, cell.column, cell.value)
		}
	}
	return nil
}

func parseImportCell(column importColumnPlan, text string, isPackage bool, env codec.Env) (any, error) {
	if isPackage && (column.field.kind == codec.Date || column.field.kind == codec.DateTime ||
		column.field.kind == codec.Time || column.field.kind == codec.CreatedAt || column.field.kind == codec.UpdatedAt) {
		if value, numeric, err := parsePackageTemporal(column.field.kind, text); numeric || err != nil {
			return value, err
		}
	}
	if isPackage && column.field.system {
		switch column.field.kind {
		case codec.CreatedAt, codec.UpdatedAt:
			field, err := codec.Bind(codec.DateTime, json.RawMessage(`{}`))
			if err != nil {
				return nil, err
			}
			return field.ParsePaste(text, codec.Env{Location: time.UTC})
		case codec.CreatedBy, codec.UpdatedBy:
			field, err := codec.Bind(codec.User, json.RawMessage(`{}`))
			if err != nil {
				return nil, err
			}
			return field.ParsePaste(text, env)
		}
	}
	if isPackage && column.field.kind == codec.AutoNumber {
		var options struct {
			Prefix string `json:"prefix"`
		}
		if err := json.Unmarshal(column.field.codec.Options(), &options); err != nil {
			return nil, err
		}
		if !strings.HasPrefix(text, options.Prefix) {
			return nil, validation("The package autonumber is invalid")
		}
		return strconv.ParseInt(strings.TrimPrefix(text, options.Prefix), 10, 64)
	}
	if isPackage && column.source.UIType == "link" && column.field.kind != "" {
		var labels []string
		if strings.TrimSpace(text) != "" && json.Unmarshal([]byte(text), &labels) != nil {
			return nil, validation("The package link cell is invalid")
		}
		return column.field.codec.ParsePaste(strings.Join(labels, ", "), env)
	}
	if isPackage && column.source.UIType == string(column.field.kind) {
		switch column.field.kind {
		case codec.Attachment:
			if text == "" {
				return nil, nil
			}
			return nil, validation("The package contains attachment names without attachment data")
		case codec.MultiSelect, codec.TextArray, codec.IntArray, codec.MultiUser:
			if strings.TrimSpace(text) == "" {
				return nil, nil
			}
			return column.field.codec.Decode(json.RawMessage(text))
		case codec.DateTime:
			field, err := codec.Bind(codec.DateTime, json.RawMessage(`{}`))
			if err != nil {
				return nil, err
			}
			return field.ParsePaste(text, codec.Env{Location: time.UTC})
		}
	}
	return column.field.codec.ParsePaste(text, env)
}
