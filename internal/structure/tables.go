package structure

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/acl"
	"pandatable/internal/codec"
	"pandatable/internal/ddl"
	"pandatable/internal/structure/structuresql"
)

type TablePatch struct {
	Name           *string
	Index          *int
	DisplayFieldID *string
}

func parentBook(path acl.Path) acl.Resource {
	return acl.Resource{Type: acl.Book, ID: path.Book}
}

// CreateTable creates the physical table with its five system columns and the
// generated Name column, all system-field metadata, the Name display field and
// the default shared Grid view atomically. Only an owner of the live book may do so.
func (s *Service) CreateTable(ctx context.Context, actor Actor, bookID, name string) (TableSchema, error) {
	target, err := resourceOf(acl.Book, bookID)
	if err != nil {
		return TableSchema{}, err
	}
	cleaned, err := cleanName(name)
	if err != nil {
		return TableSchema{}, err
	}
	labels := labelsFor(actor.Locale)
	var created TableSchema
	err = retryCollisions(func() error {
		return s.run(ctx, actor, scope{target: target, affectsACL: true}, func(c *txn) error {
			book, err := c.q.GetBook(ctx, target.ID)
			if err != nil {
				return notFoundIfNoRows(err, "read book")
			}
			tableName, err := s.newName(ddl.Table)
			if err != nil {
				return err
			}
			displayColumn, err := s.newName(ddl.Column)
			if err != nil {
				return err
			}
			last, err := c.q.MaxTablePosition(ctx, target.ID)
			if err != nil {
				return fmt.Errorf("read table position: %w", err)
			}
			tableID, err := c.q.InsertTable(ctx, structuresql.InsertTableParams{BookID: target.ID, Name: cleaned,
				PgName: tableName, Position: last + positionStep, CreatedBy: c.user})
			if err != nil {
				return fmt.Errorf("insert table: %w", err)
			}
			physical := ddl.TableName{Schema: book.PgSchema, Name: tableName}
			if err := ddl.CreateTable(ctx, c.tx, physical); err != nil {
				return err
			}
			if err := ddl.AddColumn(ctx, c.tx, physical, ddl.ColumnSpec{Name: displayColumn, Type: ddl.DataType{Kind: ddl.Text}}); err != nil {
				return err
			}
			display, err := c.q.InsertField(ctx, structuresql.InsertFieldParams{TableID: tableID, Name: labels.name,
				UiType: "text", PgColumn: pgText(displayColumn), PgType: pgText(string(ddl.Text)), Position: nameFieldPosition})
			if err != nil {
				return fmt.Errorf("insert display field: %w", err)
			}
			systemIDs := make([]string, 0, len(systemFields))
			for index, field := range systemFields {
				id, err := c.q.InsertField(ctx, structuresql.InsertFieldParams{TableID: tableID, Name: field.label(labels),
					UiType: field.column, PgColumn: pgText(field.column), PgType: pgText(field.pgType), IsSystem: true,
					IsRequired: field.required, Position: systemFieldPosition + float64(index)})
				if err != nil {
					return fmt.Errorf("insert system field: %w", err)
				}
				systemIDs = append(systemIDs, id.String())
			}
			if _, err := c.q.SetTableDisplayField(ctx, structuresql.SetTableDisplayFieldParams{ID: tableID, DisplayFieldID: display}); err != nil {
				return notFoundIfNoRows(err, "set display field")
			}
			config, err := defaultGridConfig(display.String(), systemIDs)
			if err != nil {
				return err
			}
			if _, err := c.q.InsertDefaultGridView(ctx, structuresql.InsertDefaultGridViewParams{TableID: tableID,
				Name: labels.grid, CreatedBy: c.user, Config: config}); err != nil {
				return fmt.Errorf("insert default grid view: %w", err)
			}
			created, err = loadSchema(ctx, c.q, c.tx, tableID, c.user, acl.Owner)
			return err
		})
	})
	if err != nil {
		return TableSchema{}, err
	}
	return created, nil
}

func pgText(value string) pgtype.Text { return pgtype.Text{String: value, Valid: true} }

// UpdateTable renames (metadata only) and/or moves a table among the live
// tables of its book (book owner), and/or sets the display field (table owner,
// D-123/D-129).
func (s *Service) UpdateTable(ctx context.Context, actor Actor, id string, patch TablePatch) (Table, error) {
	target, err := resourceOf(acl.Table, id)
	if err != nil {
		return Table{}, err
	}
	if patch.Name == nil && patch.Index == nil && patch.DisplayFieldID == nil {
		return Table{}, validation("At least one table field is required")
	}
	var name string
	if patch.Name != nil {
		if name, err = cleanName(*patch.Name); err != nil {
			return Table{}, err
		}
	}
	if patch.Index != nil && *patch.Index < 0 {
		return Table{}, validation("Index is outside the sibling range")
	}
	sc := scope{target: target, owner: parentBook}
	if patch.Name == nil && patch.Index == nil {
		sc.owner = nil
	}
	var updated Table
	err = s.run(ctx, actor, sc, func(c *txn) error {
		if patch.DisplayFieldID != nil {
			if err := setDisplayField(ctx, c, target.ID, *patch.DisplayFieldID); err != nil {
				return err
			}
		}
		if patch.Name != nil {
			if _, err := c.q.RenameTable(ctx, structuresql.RenameTableParams{ID: target.ID, Name: name}); err != nil {
				return notFoundIfNoRows(err, "rename table")
			}
		}
		if patch.Index != nil {
			rows, err := c.q.ListLiveTableOrder(ctx, c.path.Book)
			if err != nil {
				return fmt.Errorf("list table order: %w", err)
			}
			siblings := make([]sibling, len(rows))
			for index, row := range rows {
				siblings[index] = sibling{ID: row.ID, Position: row.Position}
			}
			if err := reorder(siblings, target.ID, *patch.Index, func(position float64) error {
				_, err := c.q.SetTablePosition(ctx, structuresql.SetTablePositionParams{ID: target.ID, Position: position})
				return notFoundIfNoRows(err, "move table")
			}, func(order []pgtype.UUID) error {
				return c.q.RenumberTables(ctx, order)
			}); err != nil {
				return err
			}
		}
		row, err := c.q.GetTable(ctx, target.ID)
		if err != nil {
			return notFoundIfNoRows(err, "read table")
		}
		updated = tableDTO(row.ID, row.BookID, row.DisplayFieldID, row.Name, row.Position, row.SchemaVersion, row.CreatedAt)
		return nil
	})
	return updated, err
}

// DeleteTable soft-deletes a table; the physical table, rows, fields, views and
// grants stay for the later Trash restore/purge service.
func (s *Service) DeleteTable(ctx context.Context, actor Actor, id string) error {
	target, err := resourceOf(acl.Table, id)
	if err != nil {
		return err
	}
	return s.run(ctx, actor, scope{target: target, owner: parentBook, affectsACL: true}, func(c *txn) error {
		if _, err := c.q.TrashTable(ctx, structuresql.TrashTableParams{ID: target.ID, DeletedBy: c.user}); err != nil {
			return notFoundIfNoRows(err, "trash table")
		}
		return nil
	})
}

// Schema returns the table's live fields and the caller's visible views from one
// repeatable-read snapshot. Read access uses the cached resolver (D-079).
func (s *Service) Schema(ctx context.Context, actor Actor, id string) (TableSchema, error) {
	target, err := resourceOf(acl.Table, id)
	if err != nil {
		return TableSchema{}, err
	}
	access, err := s.resolver.Access(ctx, actor.ID, target)
	if err != nil {
		return TableSchema{}, err
	}
	if err := access.Check(acl.Read); err != nil {
		return TableSchema{}, err
	}
	user, ok := acl.ParseUUID(actor.ID)
	if !ok {
		return TableSchema{}, acl.ErrValidation()
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return TableSchema{}, fmt.Errorf("begin schema snapshot: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	return loadSchema(ctx, s.queries.WithTx(tx), tx, target.ID, user, access.Level)
}

func loadSchema(ctx context.Context, q *structuresql.Queries, tx pgx.Tx, tableID, user pgtype.UUID, level acl.Level) (TableSchema, error) {
	table, err := q.GetTable(ctx, tableID)
	if err != nil {
		return TableSchema{}, notFoundIfNoRows(err, "read table")
	}
	fields, err := q.ListTableFields(ctx, tableID)
	if err != nil {
		return TableSchema{}, fmt.Errorf("list table fields: %w", err)
	}
	views, err := q.ListTableViews(ctx, structuresql.ListTableViewsParams{TableID: tableID, UserID: user})
	if err != nil {
		return TableSchema{}, fmt.Errorf("list table views: %w", err)
	}
	relations, err := relationRows(ctx, tx, tableID)
	if err != nil {
		return TableSchema{}, err
	}
	schema := TableSchema{
		Table:  tableDTO(table.ID, table.BookID, table.DisplayFieldID, table.Name, table.Position, table.SchemaVersion, table.CreatedAt),
		Fields: make([]Field, len(fields)), Relations: make([]Relation, len(relations)), Views: make([]View, len(views)),
		SchemaVersion: table.SchemaVersion, MyLevel: level.String(),
	}
	for index, row := range fields {
		options, err := fieldOptionsForResponse(row.Options)
		if err != nil {
			return TableSchema{}, err
		}
		schema.Fields[index] = Field{ID: row.ID.String(), Name: row.Name, Description: row.Description, UIType: row.UiType,
			Options: json.RawMessage(options), Filterable: true, FilterOps: []codec.Op{},
			IsSystem: row.IsSystem, IsRequired: row.IsRequired, IsUnique: row.IsUnique,
			Default: json.RawMessage("null"), Position: row.Position, Version: row.Version}
		if row.DefaultJson != nil {
			var compact bytes.Buffer
			if err := json.Compact(&compact, row.DefaultJson); err != nil {
				return TableSchema{}, fmt.Errorf("read field default: %w", err)
			}
			schema.Fields[index].Default = json.RawMessage(compact.Bytes())
		}
	}
	rowMetadata, err := q.GetRowTable(ctx, tableID)
	if err != nil {
		return TableSchema{}, fmt.Errorf("read filterability table metadata: %w", err)
	}
	filterTable, err := loadRowTable(ctx, q, tx, tableID, rowMetadata.PgSchema, rowMetadata.PgName, rowMetadata.SchemaVersion)
	if err != nil {
		return TableSchema{}, err
	}
	filterCompiler := filterCompiler{ctx: ctx, tx: tx, actor: Actor{ID: user.String()}, table: filterTable}
	for index := range schema.Fields {
		metadata := filterTable.byID[schema.Fields[index].ID]
		readable, err := filterFieldReadable(ctx, tx, user, filterTable, metadata, map[string]bool{})
		if err != nil {
			return TableSchema{}, err
		}
		schema.Fields[index].Filterable = readable
		if readable {
			ops, err := filterCompiler.fieldOps(metadata)
			if err != nil {
				return TableSchema{}, err
			}
			schema.Fields[index].FilterOps = ops
			schema.Fields[index].Filterable = len(ops) > 0
		}
	}
	for index, row := range views {
		config, err := readViewColumns(row.Config, schema.Fields)
		if err != nil {
			return TableSchema{}, err
		}
		schema.Views[index] = View{ID: row.ID.String(), Name: row.Name, Type: row.Type, Scope: row.Scope,
			Locked: row.Locked, Position: row.Position, Version: row.Version, Config: config}
	}
	for index, relation := range relations {
		schema.Relations[index] = relationDTO(relation)
	}
	return schema, nil
}
