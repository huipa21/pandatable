package structure

import (
	"encoding/json"
	"testing"

	"pandatable/internal/acl"
)

func relationTable(t *testing.T, f *fixture, owner Actor, book string, name string) (TableSchema, Field) {
	t.Helper()
	table, err := f.service.CreateTable(f.ctx, owner, book, name)
	if err != nil {
		t.Fatal(err)
	}
	field, err := f.service.CreateField(f.ctx, owner, table.Table.ID, FieldInput{Name: "Label", UIType: "text"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.UpdateTable(f.ctx, owner, table.Table.ID, TablePatch{DisplayFieldID: &field.Field.ID}); err != nil {
		t.Fatal(err)
	}
	return field.Schema, *field.Field
}

func relationRow(t *testing.T, f *fixture, owner Actor, tableID, fieldID, value string) string {
	t.Helper()
	rows, err := f.service.CreateRows(f.ctx, owner, tableID, []RowInsert{{Fields: map[string]json.RawMessage{
		fieldID: json.RawMessage(`"` + value + `"`),
	}}})
	if err != nil || len(rows) != 1 {
		t.Fatalf("create relation row: rows=%+v err=%v", rows, err)
	}
	return rows[0].ID
}

func relationValues(t *testing.T, row RowRecord, fieldID string) []LinkValue {
	t.Helper()
	var values []LinkValue
	if err := json.Unmarshal(row.Fields[fieldID], &values); err != nil {
		t.Fatalf("decode relation links %q: %v", row.Fields[fieldID], err)
	}
	return values
}

func TestRelationOneToManyLinksAndDeletePolicies(t *testing.T) {
	for _, policy := range []string{"set_null", "cascade", "restrict"} {
		t.Run(policy, func(t *testing.T) {
			f := newFixture(t)
			owner := f.user("user", "active", "en")
			workspace := f.workspace(owner)
			book := f.book(owner, workspace.ID)
			parentSchema, parentLabel := relationTable(t, f, owner, book.ID, "Projects")
			childSchema, childLabel := relationTable(t, f, owner, book.ID, "Tasks")
			created, err := f.service.CreateRelation(f.ctx, owner, childSchema.Table.ID,
				RelationInput{Name: "Project", ReverseName: "Tasks", TargetTableID: parentSchema.Table.ID,
					Kind: "one_many", SourceSide: "child", OnDelete: policy})
			if err != nil {
				t.Fatal(err)
			}
			parentID := relationRow(t, f, owner, created.Relation.ParentTableID, parentLabel.ID, "Project A")
			childID := relationRow(t, f, owner, created.Relation.ChildTableID, childLabel.ID, "Task A")
			if _, err := f.service.ChangeLinks(f.ctx, owner, created.Relation.ChildTableID, childID,
				created.Relation.ChildFieldID, LinkChange{Add: []string{parentID}}); err != nil {
				t.Fatal(err)
			}
			child, err := f.service.GetRow(f.ctx, owner, created.Relation.ChildTableID, childID)
			if err != nil {
				t.Fatal(err)
			}
			links := relationValues(t, child, created.Relation.ChildFieldID)
			if len(links) != 1 || links[0].ID != parentID || links[0].Label != "Project A" {
				t.Fatalf("child link = %+v", links)
			}
			parent, err := f.service.GetRow(f.ctx, owner, created.Relation.ParentTableID, parentID)
			if err != nil {
				t.Fatal(err)
			}
			links = relationValues(t, parent, created.Relation.ParentFieldID)
			if len(links) != 1 || links[0].ID != childID || links[0].Label != "Task A" {
				t.Fatalf("reverse link = %+v", links)
			}

			_, deleteErr := f.service.DeleteRows(f.ctx, owner, created.Relation.ParentTableID, []string{parentID})
			switch policy {
			case "set_null":
				if deleteErr != nil {
					t.Fatal(deleteErr)
				}
				child, err = f.service.GetRow(f.ctx, owner, created.Relation.ChildTableID, childID)
				if err != nil || len(relationValues(t, child, created.Relation.ChildFieldID)) != 0 {
					t.Fatalf("set_null child link=%+v err=%v", child, err)
				}
			case "cascade":
				if deleteErr != nil {
					t.Fatal(deleteErr)
				}
				_, err = f.service.GetRow(f.ctx, owner, created.Relation.ChildTableID, childID)
				expectError(t, err, 404, "ERR_NOT_FOUND")
			case "restrict":
				expectError(t, deleteErr, 409, "ERR_RESTRICT_VIOLATION")
				if _, err := f.service.GetRow(f.ctx, owner, created.Relation.ParentTableID, parentID); err != nil {
					t.Fatalf("restricted parent was deleted: %v", err)
				}
			}
		})
	}
}

func TestRelationOneToOneReplacementAndManyToMany(t *testing.T) {
	t.Run("one-to-one replacement", func(t *testing.T) {
		f := newFixture(t)
		owner := f.user("user", "active", "en")
		workspace := f.workspace(owner)
		book := f.book(owner, workspace.ID)
		parent, parentLabel := relationTable(t, f, owner, book.ID, "People")
		child, childLabel := relationTable(t, f, owner, book.ID, "Profiles")
		created, err := f.service.CreateRelation(f.ctx, owner, child.Table.ID, RelationInput{
			Name: "Person", ReverseName: "Profile", TargetTableID: parent.Table.ID, Kind: "one_one",
			SourceSide: "child",
		})
		if err != nil {
			t.Fatal(err)
		}
		first := relationRow(t, f, owner, created.Relation.ChildTableID, childLabel.ID, "Profile A")
		second := relationRow(t, f, owner, created.Relation.ChildTableID, childLabel.ID, "Profile B")
		person := relationRow(t, f, owner, created.Relation.ParentTableID, parentLabel.ID, "Person")
		if _, err := f.service.ChangeLinks(f.ctx, owner, created.Relation.ChildTableID, first,
			created.Relation.ChildFieldID, LinkChange{Add: []string{person}}); err != nil {
			t.Fatal(err)
		}
		_, err = f.service.ChangeLinks(f.ctx, owner, created.Relation.ChildTableID, second,
			created.Relation.ChildFieldID, LinkChange{Add: []string{person}})
		expectError(t, err, 409, "ERR_RELATION_CONFLICT")
		if _, err := f.service.ChangeLinks(f.ctx, owner, created.Relation.ChildTableID, second,
			created.Relation.ChildFieldID, LinkChange{Add: []string{person}, Replace: true}); err != nil {
			t.Fatal(err)
		}
		firstRow, err := f.service.GetRow(f.ctx, owner, created.Relation.ChildTableID, first)
		if err != nil || len(relationValues(t, firstRow, created.Relation.ChildFieldID)) != 0 {
			t.Fatalf("old one-to-one link = %+v err=%v", firstRow, err)
		}
		secondRow, err := f.service.GetRow(f.ctx, owner, created.Relation.ChildTableID, second)
		if err != nil || len(relationValues(t, secondRow, created.Relation.ChildFieldID)) != 1 {
			t.Fatalf("replacement link = %+v err=%v", secondRow, err)
		}
	})

	t.Run("many-to-many reverse and delete", func(t *testing.T) {
		f := newFixture(t)
		owner := f.user("user", "active", "en")
		workspace := f.workspace(owner)
		book := f.book(owner, workspace.ID)
		tasks, taskLabel := relationTable(t, f, owner, book.ID, "Tasks")
		tags, tagLabel := relationTable(t, f, owner, book.ID, "Tags")
		created, err := f.service.CreateRelation(f.ctx, owner, tasks.Table.ID, RelationInput{
			Name: "Tags", ReverseName: "Tasks", TargetTableID: tags.Table.ID, Kind: "many_many", OnDelete: "cascade",
		})
		if err != nil {
			t.Fatal(err)
		}
		taskA := relationRow(t, f, owner, created.Relation.ParentTableID, taskLabel.ID, "Task A")
		taskB := relationRow(t, f, owner, created.Relation.ParentTableID, taskLabel.ID, "Task B")
		tag := relationRow(t, f, owner, created.Relation.ChildTableID, tagLabel.ID, "Tag")
		if _, err := f.service.ChangeLinks(f.ctx, owner, created.Relation.ParentTableID, taskA,
			created.Relation.ParentFieldID, LinkChange{Add: []string{tag}}); err != nil {
			t.Fatal(err)
		}
		if _, err := f.service.ChangeLinks(f.ctx, owner, created.Relation.ChildTableID, tag,
			created.Relation.ChildFieldID, LinkChange{Add: []string{taskB}}); err != nil {
			t.Fatal(err)
		}
		tagRow, err := f.service.GetRow(f.ctx, owner, created.Relation.ChildTableID, tag)
		if err != nil {
			t.Fatal(err)
		}
		links := relationValues(t, tagRow, created.Relation.ChildFieldID)
		if len(links) != 2 || links[0].Label != "Task A" || links[1].Label != "Task B" {
			t.Fatalf("many-to-many reverse links = %+v", links)
		}
		if _, err := f.service.DeleteRows(f.ctx, owner, created.Relation.ParentTableID, []string{taskA}); err != nil {
			t.Fatal(err)
		}
		_, err = f.service.GetRow(f.ctx, owner, created.Relation.ChildTableID, tag)
		expectError(t, err, 404, "ERR_NOT_FOUND")
	})
}

func TestRelationSameBookSelfAndACL(t *testing.T) {
	f := newFixture(t)
	owner, reader := f.user("user", "active", "en"), f.user("user", "active", "en")
	workspace := f.workspace(owner)
	book := f.book(owner, workspace.ID)
	table, label := relationTable(t, f, owner, book.ID, "Employees")
	otherBook := f.book(owner, workspace.ID)
	outside, _ := relationTable(t, f, owner, otherBook.ID, "Outside")
	_, err := f.service.CreateRelation(f.ctx, owner, table.Table.ID, RelationInput{
		Name: "Manager", ReverseName: "Reports", TargetTableID: outside.Table.ID, Kind: "one_many",
		SourceSide: "child",
	})
	expectError(t, err, 400, "ERR_VALIDATION")

	f.grant(acl.Table, table.Table.ID, "user", reader.ID, acl.Write)
	_, err = f.service.CreateRelation(f.ctx, reader, table.Table.ID, RelationInput{
		Name: "Manager", ReverseName: "Reports", TargetTableID: table.Table.ID, Kind: "one_many",
		SourceSide: "child",
	})
	expectError(t, err, 403, "ERR_PERMISSION_DENIED")

	created, err := f.service.CreateRelation(f.ctx, owner, table.Table.ID, RelationInput{
		Name: "Manager", ReverseName: "Reports", TargetTableID: table.Table.ID, Kind: "one_many",
		SourceSide: "child",
	})
	if err != nil {
		t.Fatal(err)
	}
	manager := relationRow(t, f, owner, table.Table.ID, label.ID, "Manager")
	report := relationRow(t, f, owner, table.Table.ID, label.ID, "Report")
	if _, err := f.service.ChangeLinks(f.ctx, owner, table.Table.ID, report, created.Relation.ChildFieldID,
		LinkChange{Add: []string{manager}}); err != nil {
		t.Fatal(err)
	}
	row, err := f.service.GetRow(f.ctx, owner, table.Table.ID, manager)
	if err != nil {
		t.Fatal(err)
	}
	links := relationValues(t, row, created.Relation.ParentFieldID)
	if len(links) != 1 || links[0].ID != report {
		t.Fatalf("self relation reverse links = %+v", links)
	}
}

func TestRelationFieldDeletionRemovesCounterpartAndPhysicalStorage(t *testing.T) {
	f := newFixture(t)
	owner := f.user("user", "active", "en")
	workspace := f.workspace(owner)
	book := f.book(owner, workspace.ID)
	parent, _ := relationTable(t, f, owner, book.ID, "Projects")
	child, _ := relationTable(t, f, owner, book.ID, "Tasks")
	created, err := f.service.CreateRelation(f.ctx, owner, child.Table.ID, RelationInput{
		Name: "Project", ReverseName: "Tasks", TargetTableID: parent.Table.ID,
		Kind: "one_many", SourceSide: "child",
	})
	if err != nil {
		t.Fatal(err)
	}

	var schemaName, tableName, fkColumn string
	if err := f.pool.QueryRow(f.ctx, `SELECT b.pg_schema,t.pg_name,r.fk_column
		FROM meta.relations r
		JOIN meta.tables t ON t.id=r.child_table_id
		JOIN meta.books b ON b.id=t.book_id
		WHERE r.id=$1::uuid`, created.Relation.ID).
		Scan(&schemaName, &tableName, &fkColumn); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.DeleteField(f.ctx, owner, created.Field.ID, created.Field.Version); err != nil {
		t.Fatal(err)
	}

	for _, tableID := range []string{parent.Table.ID, child.Table.ID} {
		schema, err := f.service.Schema(f.ctx, owner, tableID)
		if err != nil {
			t.Fatal(err)
		}
		if len(schema.Relations) != 0 {
			t.Fatalf("relation remains in table %s schema: %+v", tableID, schema.Relations)
		}
		for _, field := range schema.Fields {
			if field.ID == created.Relation.ParentFieldID ||
				field.ID == created.Relation.ChildFieldID {
				t.Fatalf("counterpart link field remains in table %s: %+v", tableID, field)
			}
		}
	}

	var remaining int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema=$1 AND table_name=$2 AND column_name=$3`,
		schemaName, tableName, fkColumn).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("relation FK column remains after delete: %s.%s.%s", schemaName, tableName, fkColumn)
	}
}

func TestRelationTestDBIsPostgres18(t *testing.T) {
	f := newFixture(t)
	var version int
	if err := f.pool.QueryRow(f.ctx, "SELECT current_setting('server_version_num')::int").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version < 180000 || version >= 190000 {
		t.Fatalf("relation tests require PostgreSQL 18, got server_version_num=%d", version)
	}
}
