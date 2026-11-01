package structure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/acl"
	"pandatable/internal/codec"
	"pandatable/internal/ddl"
	"pandatable/internal/structure/structuresql"
)

func (s *Service) preparePackageLinks(ctx context.Context, actor Actor, target ImportTarget, bookID string, file *ImportFile) error {
	book, ok := acl.ParseUUID(bookID)
	if !ok {
		return acl.ErrNotFound()
	}
	user, ok := acl.ParseUUID(actor.ID)
	if !ok {
		return acl.ErrValidation()
	}
	for index := range file.Analysis.Columns {
		column := &file.Analysis.Columns[index]
		if column.UIType != "link" || column.LinkTarget == "" {
			continue
		}
		rows, err := s.pool.Query(ctx, `SELECT id FROM meta.tables
			WHERE book_id=$1 AND deleted_at IS NULL AND lower(name)=lower($2) ORDER BY id`,
			book, column.LinkTarget)
		if err != nil {
			return fmt.Errorf("find package relation target: %w", err)
		}
		var matches []pgtype.UUID
		for rows.Next() {
			var id pgtype.UUID
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return fmt.Errorf("read package relation target: %w", err)
			}
			matches = append(matches, id)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate package relation targets: %w", err)
		}
		if len(matches) != 1 {
			continue
		}
		resource := acl.Resource{Type: acl.Table, ID: matches[0]}
		access, err := acl.AccessUncached(ctx, s.pool, user, resource)
		if err != nil {
			return err
		}
		level := acl.Owner
		if target.Kind == "table" {
			level = acl.Write
		}
		if access.Visible && access.Check(level) == nil {
			column.RelinkTargetID = matches[0].String()
		}
	}
	return nil
}

func (s *Service) runPackageImport(ctx context.Context, actor Actor, target ImportTarget, file *ImportFile,
	request ImportRequest, mappings map[int]ImportMapping, progress func(ImportResult),
) (ImportResult, error) {
	if !request.HasHeader {
		return ImportResult{}, validation("A package always has a header")
	}
	rows, err := file.openRows()
	if err != nil {
		return ImportResult{}, err
	}
	defer rows.Close()
	if _, err := rows.Next(); err != nil {
		return ImportResult{}, validation("The package header is missing")
	}
	reader := &importBatchReader{rows: rows, number: 1, expectedWidth: len(file.Analysis.Columns) + 1}
	if target.Kind == "table" {
		return s.runAppendImport(ctx, actor, target.ID, file, mappings, reader, progress)
	}
	return s.runNewPackageImport(ctx, actor, target.ID, file, request, mappings, reader, progress)
}

func (s *Service) runNewPackageImport(ctx context.Context, actor Actor, bookID string, file *ImportFile,
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
	systemNames, err := packageSystemNames(file.manifest)
	if err != nil {
		return ImportResult{}, err
	}
	for _, mapping := range mappings {
		if mapping.Skip {
			return ImportResult{}, validation("New package tables must restore every field")
		}
	}
	result := ImportResult{}
	err = retryCollisions(func() error {
		result = ImportResult{}
		return s.run(ctx, actor, scope{target: target, affectsACL: true}, func(c *txn) error {
			tableID, physical, systemIDs, err := s.createImportTable(ctx, c, target.ID, name, systemNames)
			if err != nil {
				return err
			}
			keys := make(map[string]string, len(file.manifest.Fields))
			for index, system := range systemFields {
				for _, field := range file.manifest.Fields {
					if field.UIType == system.column {
						keys[field.Key] = systemIDs[index]
					}
				}
			}
			for _, field := range file.manifest.Fields {
				if codec.IsSystem(codec.Kind(field.UIType)) || field.UIType == "link" ||
					field.UIType == "formula" || isComputedKind(field.UIType) {
					continue
				}
				bound, err := bindUserKind(field.UIType, field.Options)
				if err != nil {
					return err
				}
				if field.PGType != "" && pgTypeName(bound.Column().Type) != field.PGType {
					return validation("The package field type does not match its options")
				}
				id, err := s.createImportField(ctx, c, tableID, physical, FieldInput{
					Name: field.Name, UIType: field.UIType, Options: field.Options,
					IsRequired: field.Required, IsUnique: field.Unique, Default: field.Default,
				})
				if err != nil {
					return err
				}
				keys[field.Key] = id.String()
			}
			for index, field := range file.manifest.Fields {
				if field.UIType != "link" {
					continue
				}
				column := file.Analysis.Columns[index]
				mapping := mappings[column.Index]
				if !mapping.Relink {
					id, err := s.createImportField(ctx, c, tableID, physical,
						FieldInput{Name: field.Name, UIType: string(codec.Text)})
					if err != nil {
						return err
					}
					keys[field.Key] = id.String()
					result.ConvertedLinks = append(result.ConvertedLinks, field.Name)
					continue
				}
				id, err := s.createPackageRelation(ctx, c, tableID, name, field, column)
				if err != nil {
					return err
				}
				keys[field.Key] = id.String()
			}
			table, err := loadImportRowTable(ctx, c, tableID, physical)
			if err != nil {
				return err
			}
			columns, links, err := packageImportColumns(file, table, keys)
			if err != nil {
				return err
			}
			result.TableID = tableID.String()
			for {
				batch, err := reader.next(ctx)
				if err != nil {
					return err
				}
				if len(batch) == 0 {
					break
				}
				if err := s.copyImportBatch(ctx, c.tx, actor, table, columns, links, batch, true, true, &result); err != nil {
					return err
				}
				result.Rows += len(batch)
				if progress != nil {
					progress(result)
				}
			}
			if err := resetPackageSequences(ctx, c.tx, table, columns); err != nil {
				return err
			}
			if err := s.createPackageComputedFields(ctx, c, tableID, physical, file, keys); err != nil {
				return err
			}
			if err := s.createPackageFormulas(ctx, c, tableID, physical, file, keys); err != nil {
				return err
			}
			if err := restorePackagePositions(ctx, c.tx, file.manifest, keys); err != nil {
				return err
			}
			if file.manifest.Table.DisplayField != nil {
				display, ok := acl.ParseUUID(keys[*file.manifest.Table.DisplayField])
				if !ok {
					return validation("The package display field is invalid")
				}
				if _, err := c.q.SetTableDisplayField(ctx, structuresql.SetTableDisplayFieldParams{
					ID: tableID, DisplayFieldID: display,
				}); err != nil {
					return notFoundIfNoRows(err, "set package display field")
				}
			}
			if err := c.q.BumpSchemaVersion(ctx, tableID); err != nil {
				return fmt.Errorf("bump package schema version: %w", err)
			}
			table, err = loadImportRowTable(ctx, c, tableID, physical)
			if err != nil {
				return err
			}
			viewID, err := restorePackageViews(ctx, c, table, file.manifest, keys)
			if err != nil {
				return err
			}
			result.ViewID = viewID
			return nil
		})
	})
	if err != nil {
		result.TableID, result.ViewID, result.Rows = "", "", 0
	}
	return result, rowDatabaseError(err)
}

func packageSystemNames(manifest *tablePackageManifest) (map[codec.Kind]string, error) {
	names := make(map[codec.Kind]string, len(systemFields))
	for _, field := range manifest.Fields {
		kind := codec.Kind(field.UIType)
		if !codec.IsSystem(kind) {
			continue
		}
		if _, exists := names[kind]; exists || field.Unique || !isJSONNull(field.Default) && len(field.Default) > 0 {
			return nil, validation("The package system fields are invalid")
		}
		valid := false
		for _, system := range systemFields {
			if system.column == field.UIType && system.required == field.Required {
				valid = true
			}
		}
		if !valid {
			return nil, validation("The package system fields are invalid")
		}
		names[kind] = field.Name
	}
	if len(names) != len(systemFields) {
		return nil, validation("The package system fields are incomplete")
	}
	return names, nil
}

func packageImportColumns(file *ImportFile, table rowTable, keys map[string]string) ([]importColumnPlan, []importLinkPlan, error) {
	columns := make([]importColumnPlan, 0, len(file.Analysis.Columns))
	links := make([]importLinkPlan, 0)
	for _, column := range file.Analysis.Columns {
		if column.UIType == "formula" || isComputedKind(column.UIType) {
			continue
		}
		field, ok := table.byID[keys[column.PackageKey]]
		if !ok {
			return nil, nil, errors.New("created package field is unavailable")
		}
		if field.relation != nil {
			links = append(links, importLinkPlan{index: column.Index, name: column.Name, field: field, source: column})
		} else {
			columns = append(columns, importColumnPlan{index: column.Index, name: column.Name, field: field, source: column})
		}
	}
	return columns, links, nil
}

func importBatchIDs(ctx context.Context, tx pgx.Tx, table rowTable, batch []importRecord, preserve bool) ([]int64, error) {
	if !preserve {
		return allocateImportIDs(ctx, tx, table, len(batch))
	}
	ids := make([]int64, len(batch))
	for index, record := range batch {
		if len(record.values) == 0 {
			return nil, validation("The package row id is missing")
		}
		id, err := canonicalRowID(record.values[0])
		if err != nil {
			return nil, validation("The package row id is invalid")
		}
		ids[index] = id
	}
	return ids, nil
}

// Excelize's date reader rounds serials to whole seconds. Decode the package
// serial directly so its available fractional-second precision is retained.
func parsePackageTemporal(kind codec.Kind, text string) (any, bool, error) {
	serial, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return nil, false, nil
	}
	if math.IsNaN(serial) || math.IsInf(serial, 0) || serial < 1 || serial >= 2958466 {
		return nil, true, validation("The package date serial is invalid")
	}
	days := int(math.Floor(serial))
	if days == 60 {
		return nil, true, validation("The package date is not a real calendar date")
	}
	fraction := serial - float64(days)
	if days > 60 {
		days--
	}
	micros := int64(math.Round(fraction * 86400 * 1_000_000))
	instant := time.Date(1899, 12, 31, 0, 0, 0, 0, time.UTC).AddDate(0, 0, days).
		Add(time.Duration(micros) * time.Microsecond)
	bindKind, value := codec.DateTime, instant.Format(time.RFC3339Nano)
	if kind == codec.Date {
		bindKind, value = codec.Date, instant.Format("2006-01-02")
	}
	field, err := codec.Bind(bindKind, json.RawMessage(`{}`))
	if err != nil {
		return nil, true, err
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, true, err
	}
	decoded, err := field.Decode(raw)
	return decoded, true, err
}

func resetPackageSequences(ctx context.Context, tx pgx.Tx, table rowTable, columns []importColumnPlan) error {
	sequenceColumns := []string{"id"}
	for _, column := range columns {
		if column.field.kind == codec.AutoNumber {
			sequenceColumns = append(sequenceColumns, column.field.column)
		}
	}
	for _, column := range sequenceColumns {
		name := pgx.Identifier{column}.Sanitize()
		query := "SELECT pg_catalog.setval(pg_catalog.pg_get_serial_sequence($1,$2)::regclass, " +
			"COALESCE(MAX(" + name + "),1), COUNT(*) > 0) FROM " + table.qualifiedName()
		if _, err := tx.Exec(ctx, query, table.qualifiedName(), column); err != nil {
			return fmt.Errorf("reset package sequence: %w", err)
		}
	}
	return nil
}

type importAudit struct {
	id        int64
	updatedAt pgtype.Timestamptz
	updatedBy pgtype.UUID
}

func readImportAudit(ctx context.Context, tx pgx.Tx, table rowTable, ids []int64) ([]importAudit, error) {
	rows, err := tx.Query(ctx, `SELECT "id", "updated_at", "updated_by" FROM `+table.qualifiedName()+
		` WHERE "id"=ANY($1::bigint[]) ORDER BY "id"`, ids)
	if err != nil {
		return nil, fmt.Errorf("read package row timestamps: %w", err)
	}
	defer rows.Close()
	audit := make([]importAudit, 0, len(ids))
	for rows.Next() {
		var item importAudit
		if err := rows.Scan(&item.id, &item.updatedAt, &item.updatedBy); err != nil {
			return nil, fmt.Errorf("read package row timestamp: %w", err)
		}
		audit = append(audit, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate package row timestamps: %w", err)
	}
	if len(audit) != len(ids) {
		return nil, errors.New("package row timestamp count mismatch")
	}
	return audit, nil
}

func restoreImportAudit(ctx context.Context, tx pgx.Tx, table rowTable, audit []importAudit) error {
	ids := make([]int64, len(audit))
	times := make([]pgtype.Timestamptz, len(audit))
	users := make([]pgtype.UUID, len(audit))
	for index, item := range audit {
		ids[index], times[index], users[index] = item.id, item.updatedAt, item.updatedBy
	}
	query := "UPDATE " + table.qualifiedName() + ` AS "target" SET "updated_at"="source"."stamp",
		"updated_by"="source"."author" FROM unnest($1::bigint[], $2::timestamptz[], $3::uuid[])
		AS "source"("id","stamp","author") WHERE "target"."id"="source"."id"`
	if _, err := tx.Exec(ctx, query, ids, times, users); err != nil {
		return fmt.Errorf("restore package row timestamps: %w", err)
	}
	return nil
}

func restorePackagePositions(ctx context.Context, tx pgx.Tx, manifest *tablePackageManifest, keys map[string]string) error {
	for index, field := range manifest.Fields {
		if codec.IsSystem(codec.Kind(field.UIType)) {
			continue
		}
		id := keys[field.Key]
		if id == "" {
			return validation("A package field was not restored")
		}
		if _, err := tx.Exec(ctx, "UPDATE meta.fields SET position=$2 WHERE id=$1",
			id, float64(index+1)*positionStep); err != nil {
			return fmt.Errorf("restore package field order: %w", err)
		}
	}
	return nil
}

func restorePackageViews(ctx context.Context, c *txn, table rowTable, manifest *tablePackageManifest,
	keys map[string]string,
) (string, error) {
	first := ""
	for index, source := range manifest.Views {
		name, err := cleanName(source.Name)
		if err != nil || source.Type != "grid" && source.Type != "form" {
			return "", validation("The package view metadata is invalid")
		}
		config, err := packageViewConfig(source.Config, keys)
		if err != nil {
			return "", validation("The package view refers to an unavailable field")
		}
		config.FilterSet = len(config.Filter) > 0
		if config.Form != nil {
			form, err := validateFormConfig(*config.Form, table)
			if err != nil {
				return "", err
			}
			config.Form = &form
		}
		raw, _, err := normalizeManagedViewConfig(ctx, c.tx, c.actor, table, []byte(`{}`), config)
		if err != nil {
			return "", err
		}
		if config.Form != nil {
			var stored map[string]json.RawMessage
			if err := json.Unmarshal(raw, &stored); err != nil {
				return "", fmt.Errorf("decode restored view: %w", err)
			}
			stored["form"], err = json.Marshal(config.Form)
			if err != nil {
				return "", fmt.Errorf("encode restored form: %w", err)
			}
			raw, err = json.Marshal(stored)
			if err != nil {
				return "", fmt.Errorf("encode restored view: %w", err)
			}
		}
		created, err := c.q.InsertManagedView(ctx, structuresql.InsertManagedViewParams{
			TableID: table.id, Name: name, Type: source.Type, Scope: "shared",
			Position: float64(index+1) * positionStep, Config: raw, CreatedBy: c.user,
		})
		if err != nil {
			return "", fmt.Errorf("restore package view: %w", err)
		}
		if first == "" {
			first = created.ID.String()
		}
	}
	return first, nil
}

func (s *Service) createPackageComputedFields(ctx context.Context, c *txn, tableID pgtype.UUID,
	physical ddl.TableName, file *ImportFile, keys map[string]string,
) error {
	for _, field := range file.manifest.Fields {
		if !isComputedKind(field.UIType) {
			continue
		}
		var options struct {
			RelationFieldID string `json:"relationFieldId"`
			TargetField     string `json:"targetField"`
			Aggregate       string `json:"aggregate"`
		}
		if err := json.Unmarshal(field.Options, &options); err != nil {
			return validation("The package computed field options are invalid")
		}
		relationID := keys[options.RelationFieldID]
		table, err := loadImportRowTable(ctx, c, tableID, physical)
		if err != nil {
			return err
		}
		relation, exists := table.byID[relationID]
		if !exists {
			return validation("The package computed relation is unavailable")
		}
		computed := computedOptions{RelationFieldID: relationID, Aggregate: options.Aggregate}
		if relation.relation == nil || options.TargetField == "" {
			var missing pgtype.UUID
			if err := c.tx.QueryRow(ctx, "SELECT uuidv7()").Scan(&missing); err != nil {
				return fmt.Errorf("allocate missing dependency marker: %w", err)
			}
			computed.TargetFieldID, computed.Error = missing.String(), "dependency_missing"
		} else {
			target := relation.relation.parentTable
			if relation.relationSide == "parent" {
				target = relation.relation.childTable
			}
			var targetID pgtype.UUID
			err := c.tx.QueryRow(ctx, `SELECT id FROM meta.fields
				WHERE table_id=$1 AND name=$2 AND deleted_at IS NULL`, target, options.TargetField).Scan(&targetID)
			if errors.Is(err, pgx.ErrNoRows) {
				return validation("The package computed target field is unavailable")
			}
			if err != nil {
				return fmt.Errorf("find package computed target: %w", err)
			}
			computed.TargetFieldID = targetID.String()
		}
		raw, err := json.Marshal(computed)
		if err != nil {
			return fmt.Errorf("encode package computed options: %w", err)
		}
		raw, err = cleanComputedOptions(field.UIType, raw)
		if err != nil {
			return err
		}
		if computed.Error == "" {
			if _, err := computedTarget(ctx, c.tx, table, field.UIType, raw, relation); err != nil {
				return err
			}
		}
		id, err := insertPackageComputedField(ctx, c, tableID, field, raw)
		if err != nil {
			return err
		}
		keys[field.Key] = id.String()
	}
	return nil
}

func (s *Service) createPackageFormulas(ctx context.Context, c *txn, tableID pgtype.UUID,
	physical ddl.TableName, file *ImportFile, keys map[string]string,
) error {
	pending := make([]packageField, 0)
	for _, field := range file.manifest.Fields {
		if field.UIType == "formula" {
			pending = append(pending, field)
		}
	}
	for len(pending) > 0 {
		remaining := make([]packageField, 0, len(pending))
		var firstError error
		for _, field := range pending {
			table, err := loadImportRowTable(ctx, c, tableID, physical)
			if err != nil {
				return err
			}
			options, _, err := cleanFormulaOptions(field.Options, table, "", c.tx, ctx)
			if err != nil {
				if firstError == nil {
					firstError = err
				}
				remaining = append(remaining, field)
				continue
			}
			id, err := insertPackageComputedField(ctx, c, tableID, field, options)
			if err != nil {
				return err
			}
			keys[field.Key] = id.String()
		}
		if len(remaining) == len(pending) {
			return firstError
		}
		pending = remaining
	}
	return nil
}

func insertPackageComputedField(ctx context.Context, c *txn, tableID pgtype.UUID,
	field packageField, options json.RawMessage,
) (pgtype.UUID, error) {
	if field.Required || field.Unique || len(field.Default) > 0 && !isJSONNull(field.Default) {
		return pgtype.UUID{}, validation("Package computed constraints are invalid")
	}
	last, err := c.q.MaxUserFieldPosition(ctx, tableID)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("read package field position: %w", err)
	}
	id, err := c.q.InsertUserField(ctx, structuresql.InsertUserFieldParams{TableID: tableID,
		Name: field.Name, UiType: field.UIType, Options: options, Position: last + positionStep})
	if err != nil {
		return pgtype.UUID{}, fieldNameConflict(fmt.Errorf("insert package computed field: %w", err))
	}
	return id, nil
}

func packageReverseName(tableName, fieldName string, existing []structuresql.ListTableFieldsRow) string {
	base := []rune(strings.TrimSpace(tableName + " " + fieldName))
	if len(base) > maxNameRunes-8 {
		base = base[:maxNameRunes-8]
	}
	used := make(map[string]bool, len(existing))
	for _, field := range existing {
		used[strings.ToLower(field.Name)] = true
	}
	name := string(base)
	for suffix := 2; used[strings.ToLower(name)]; suffix++ {
		name = fmt.Sprintf("%s %d", string(base), suffix)
	}
	return name
}
