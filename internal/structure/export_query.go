package structure

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/acl"
	"pandatable/internal/acl/aclsql"
	"pandatable/internal/codec"
	"pandatable/internal/structure/structuresql"
)

const (
	ExportCSV     = "csv"
	ExportPackage = "xlsx"
	exportBatch   = 100
)

type ExportRequest struct {
	Format   string          `json:"format"`
	FieldIDs *[]string       `json:"fieldIds,omitempty"`
	Filter   json.RawMessage `json:"filter,omitempty"`
	Sorts    *[]RowSort      `json:"sorts,omitempty"`
	GroupBy  *[]string       `json:"groupBy,omitempty"`
	Search   *string         `json:"search,omitempty"`
}

type ExportInfo struct {
	Name   string
	Format string
}

type exportPlan struct {
	tx       pgx.Tx
	actor    Actor
	table    rowTable
	fields   []rowField
	name     string
	format   string
	location *time.Location
	query    string
	args     []any
	manifest []byte
}

type exportRow struct {
	record RowRecord
	values map[string]any
}

// CheckExport validates the same access and query plan used by the download.
func (s *Service) CheckExport(ctx context.Context, actor Actor, viewID string, request ExportRequest, version string) error {
	return s.withExportPlan(ctx, actor, viewID, request, version, func(plan *exportPlan) error {
		if err := plan.openCursor(ctx); err != nil {
			return err
		}
		_, err := plan.tx.Exec(ctx, "CLOSE pt_export")
		return err
	})
}

// Export streams rows from a consistent view snapshot to the supplied writer.
// The callback is invoked only after the caller and export plan are validated.
func (s *Service) Export(ctx context.Context, actor Actor, viewID string, request ExportRequest, version string,
	output func(ExportInfo, func(io.Writer) error) error,
) error {
	return s.withExportPlan(ctx, actor, viewID, request, version, func(plan *exportPlan) error {
		if err := plan.openCursor(ctx); err != nil {
			return err
		}
		return output(ExportInfo{Name: plan.name, Format: plan.format}, func(w io.Writer) error {
			if plan.format == ExportCSV {
				return plan.writeCSV(ctx, w)
			}
			return plan.writePackage(ctx, w)
		})
	})
}

func (s *Service) withExportPlan(ctx context.Context, actor Actor, viewID string, request ExportRequest, version string,
	run func(*exportPlan) error,
) error {
	if request.Format != ExportCSV && request.Format != ExportPackage {
		return validation("Export format is invalid")
	}
	if request.Format == ExportPackage &&
		(request.FieldIDs != nil || len(request.Filter) != 0 || request.Sorts != nil ||
			request.GroupBy != nil || request.Search != nil) {
		return validation("Package export always includes the full table")
	}
	viewUUID, ok := acl.ParseUUID(viewID)
	if !ok {
		return acl.ErrNotFound()
	}
	user, ok := acl.ParseUUID(actor.ID)
	if !ok {
		return acl.ErrValidation()
	}
	location, err := codec.LoadLocation(actor.Timezone)
	if err != nil {
		return fmt.Errorf("load export timezone: %w", err)
	}
	view, err := s.queries.GetViewForQuery(ctx, viewUUID)
	if err != nil {
		return notFoundIfNoRows(err, "read export view")
	}
	if view.Scope == "personal" && (!view.OwnerID.Valid || view.OwnerID.String() != actor.ID) {
		return acl.ErrNotFound()
	}
	target := acl.Resource{Type: acl.Table, ID: view.TableID}
	access, err := s.resolver.Access(ctx, actor.ID, target)
	if err != nil {
		return err
	}
	if err := access.Check(acl.Read); err != nil {
		return err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return fmt.Errorf("begin export snapshot: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if _, err := tx.Exec(ctx, lockTimeout); err != nil {
		return rowDatabaseError(fmt.Errorf("set export lock timeout: %w", err))
	}
	q := structuresql.New(tx)
	locked, err := q.LockViewForQuery(ctx, viewUUID)
	if err != nil {
		return rowDatabaseError(notFoundIfNoRows(err, "lock export view"))
	}
	if locked.Scope == "personal" && (!locked.OwnerID.Valid || locked.OwnerID.String() != actor.ID) {
		return acl.ErrNotFound()
	}
	if err := s.authorizeRows(ctx, tx, user, acl.Resource{Type: acl.Table, ID: locked.TableID}, acl.Read); err != nil {
		return err
	}
	if request.Format == ExportCSV && locked.Type != "grid" {
		return validation("CSV export requires a grid view")
	}
	table, err := loadRowTable(ctx, q, tx, locked.TableID, locked.PgSchema, locked.PgName, locked.SchemaVersion)
	if err != nil {
		return err
	}
	metadata, err := q.GetTable(ctx, table.id)
	if err != nil {
		return notFoundIfNoRows(err, "read export table")
	}
	plan := &exportPlan{
		tx: tx, actor: actor, table: table, name: metadata.Name,
		format: request.Format, location: location,
	}
	var config rowViewConfig
	if err := json.Unmarshal(locked.Config, &config); err != nil {
		return fmt.Errorf("decode export view config: %w", err)
	}
	var stored struct {
		Filter json.RawMessage `json:"filter"`
	}
	if err := json.Unmarshal(locked.Config, &stored); err != nil {
		return fmt.Errorf("decode export view filter: %w", err)
	}
	if request.Format == ExportPackage {
		schema, err := loadSchema(ctx, q, tx, table.id, user, access.Level)
		if err != nil {
			return err
		}
		plan.fields = make([]rowField, 0, len(schema.Fields))
		for _, field := range schema.Fields {
			rowField, exists := table.byID[field.ID]
			if !exists {
				return errors.New("package field is missing from the row projection")
			}
			plan.fields = append(plan.fields, rowField)
		}
		plan.manifest, err = packageManifest(ctx, tx, actor, schema, table, version)
		if err != nil {
			return err
		}
		if len(plan.fields)+1 > maxSpreadsheetColumns {
			return validation("Table has too many columns for an XLSX package")
		}
	} else {
		if request.FieldIDs != nil {
			seen := make(map[string]bool, len(*request.FieldIDs))
			for _, id := range *request.FieldIDs {
				if _, exists := table.byID[id]; !exists || seen[id] {
					return validation("Export fields must be unique live fields")
				}
				seen[id] = true
			}
		}
		plan.fields = visibleRowFields(table, config.Fields, request.FieldIDs)
	}
	if err := plan.buildQuery(ctx, config, stored.Filter, request); err != nil {
		return err
	}
	if err := run(plan); err != nil {
		return rowDatabaseError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return rowDatabaseError(fmt.Errorf("commit export snapshot: %w", err))
	}
	return nil
}

func (plan *exportPlan) buildQuery(ctx context.Context, config rowViewConfig, storedFilter json.RawMessage, request ExportRequest) error {
	where := ""
	var args []any
	var groups []string
	var sorts []rowSortKey
	if request.Format == ExportCSV {
		rawFilter := storedFilter
		if request.Filter != nil {
			rawFilter = request.Filter
		}
		if len(rawFilter) > 0 && !bytes.Equal(bytes.TrimSpace(rawFilter), []byte("null")) {
			var filter FilterNode
			if err := json.Unmarshal(rawFilter, &filter); err != nil {
				return validation("Export filter is invalid")
			}
			var err error
			where, args, err = compileRowFilter(ctx, plan.tx, plan.actor, plan.table, &filter)
			if err != nil {
				return err
			}
		}
		if err := validateQuickSearch(request.Search); err != nil {
			return err
		}
		searchSQL, searchArgs, err := compileQuickSearch(ctx, plan.tx, plan.actor, plan.table, plan.fields, request.Search)
		if err != nil {
			return err
		}
		where, args = combineQueryConditions(where, args, searchSQL, searchArgs)
		groups = config.GroupBy
		if request.GroupBy != nil {
			groups = *request.GroupBy
		}
		if err := validateGroupConfig(ViewColumns{GroupBy: groups}, plan.table); err != nil {
			return err
		}
		sorts, _, err = configuredRowSorts(plan.table, config.Sort, nil, request.Sorts)
		if err != nil {
			return err
		}
	}
	query, _, err := sq.Select(plan.table.selectColumnListAlias(plan.fields, "source")...).
		From(plan.table.qualifiedName() + ` AS "source"`).
		PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return fmt.Errorf("build export query: %w", err)
	}
	if where != "" {
		query += " WHERE " + where
	}
	order := make([]string, 0, len(groups)+len(sorts)+1)
	for _, id := range groups {
		field := plan.table.byID[id]
		compiler := filterCompiler{ctx: ctx, tx: plan.tx, actor: plan.actor, table: plan.table}
		expression, _, err := compiler.fieldExpression(field)
		if err != nil {
			return err
		}
		expression = shiftSQLArguments(expression, len(args))
		order = append(order, "("+canonicalGroupKey(field, expression)+") ASC")
		args = append(args, compiler.args...)
	}
	for _, key := range sorts {
		compiler := filterCompiler{ctx: ctx, tx: plan.tx, actor: plan.actor, table: plan.table}
		expression, _, err := compiler.fieldExpression(key.field)
		if err != nil {
			return err
		}
		order = append(order, shiftSQLArguments(expression, len(args))+" "+key.direction+" NULLS LAST")
		args = append(args, compiler.args...)
	}
	order = append(order, `"source"."id" ASC`)
	plan.query = query + " ORDER BY " + strings.Join(order, ", ")
	plan.args = args
	return nil
}

func (plan *exportPlan) openCursor(ctx context.Context) error {
	args := append([]any{pgx.QueryExecModeExec}, plan.args...)
	if _, err := plan.tx.Exec(ctx, "DECLARE pt_export NO SCROLL CURSOR FOR "+plan.query, args...); err != nil {
		return fmt.Errorf("open export cursor: %w", err)
	}
	return nil
}

func (plan *exportPlan) eachBatch(ctx context.Context, emit func([]exportRow, map[string]string) error) error {
	physical := physicalRowFields(plan.fields)
	for {
		rows, err := queryUncachedRows(ctx, plan.tx, "FETCH FORWARD 100 FROM pt_export")
		if err != nil {
			return fmt.Errorf("fetch export rows: %w", err)
		}
		batch := make([]exportRow, 0, exportBatch)
		records := make([]RowRecord, 0, exportBatch)
		for rows.Next() {
			values, err := rows.Values()
			if err != nil {
				rows.Close()
				return fmt.Errorf("read export row: %w", err)
			}
			record, err := plan.table.encode(values, plan.fields)
			if err != nil {
				rows.Close()
				return err
			}
			physicalValues := make(map[string]any, len(physical))
			for index, field := range physical {
				physicalValues[field.id] = values[index+1]
			}
			records = append(records, record)
			batch = append(batch, exportRow{record: record, values: physicalValues})
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return fmt.Errorf("iterate export rows: %w", err)
		}
		if len(batch) == 0 {
			return nil
		}
		if err := enrichRowLinks(ctx, plan.tx, plan.table, plan.fields, records); err != nil {
			return err
		}
		if err := enrichRowComputed(ctx, plan.tx, plan.actor, plan.table, plan.fields, records); err != nil {
			return err
		}
		if err := enrichRowFormulas(ctx, plan.tx, plan.actor, plan.table, plan.fields, records); err != nil {
			return err
		}
		labels, err := exportUserLabels(ctx, plan.tx, plan.fields, records, plan.actor.Locale)
		if err != nil {
			return err
		}
		if err := emit(batch, labels); err != nil {
			return err
		}
	}
}

func exportUserLabels(ctx context.Context, tx pgx.Tx, fields []rowField, records []RowRecord, locale string) (map[string]string, error) {
	seen := make(map[string]bool)
	ids := make([]pgtype.UUID, 0)
	for _, field := range fields {
		switch field.kind {
		case codec.User, codec.MultiUser, codec.CreatedBy, codec.UpdatedBy:
		default:
			continue
		}
		for _, record := range records {
			values, err := exportUserIDs(record.Fields[field.id])
			if err != nil {
				return nil, err
			}
			for _, id := range values {
				if seen[id] {
					continue
				}
				parsed, ok := acl.ParseUUID(id)
				if !ok {
					return nil, errors.New("export user value is not a UUID")
				}
				seen[id] = true
				ids = append(ids, parsed)
			}
		}
	}
	labels := make(map[string]string, len(ids))
	if len(ids) == 0 {
		return labels, nil
	}
	users, err := aclsql.New(tx).ResolveTableUserLabels(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("resolve export user labels: %w", err)
	}
	for _, user := range users {
		name := user.Name
		if user.Deleted {
			name = "Deleted user"
			if locale == "zh-CN" {
				name = "已删除的用户"
			}
		}
		labels[user.ID.String()] = name
	}
	return labels, nil
}

func exportUserIDs(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		return []string{single}, nil
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err != nil {
		return nil, errors.New("export user value has an invalid shape")
	}
	return many, nil
}
