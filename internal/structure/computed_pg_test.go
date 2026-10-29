package structure

import (
	"encoding/json"
	"reflect"
	"testing"

	"pandatable/internal/acl"
)

func computedFieldFor(t *testing.T, f *fixture, owner Actor, tableID, name, kind string, options computedOptions) Field {
	t.Helper()
	raw, err := json.Marshal(options)
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.service.CreateField(f.ctx, owner, tableID, FieldInput{
		Name: name, UIType: kind, Options: raw,
	})
	if err != nil {
		t.Fatalf("create %s field: %v", kind, err)
	}
	return *result.Field
}

func computedRows(t *testing.T, f *fixture, actor Actor, schema TableSchema) []RowRecord {
	t.Helper()
	if len(schema.Views) == 0 {
		t.Fatal("table has no view")
	}
	result, err := f.service.QueryRows(f.ctx, actor, schema.Views[0].ID, RowQueryRequest{})
	if err != nil {
		t.Fatal(err)
	}
	return result.Rows
}

func TestLookupComputedValueUpdatesWithRelatedRow(t *testing.T) {
	f := newFixture(t)
	owner := f.user("user", "active", "en")
	workspace := f.workspace(owner)
	book := f.book(owner, workspace.ID)
	projects, projectLabel := relationTable(t, f, owner, book.ID, "Projects")
	tasks, taskLabel := relationTable(t, f, owner, book.ID, "Tasks")
	projectOwner, err := f.service.CreateField(f.ctx, owner, projects.Table.ID, FieldInput{
		Name: "Owner", UIType: "text",
	})
	if err != nil {
		t.Fatal(err)
	}
	relation, err := f.service.CreateRelation(f.ctx, owner, tasks.Table.ID, RelationInput{
		Name: "Project", ReverseName: "Tasks", TargetTableID: projects.Table.ID,
		Kind: "one_many", SourceSide: "child",
	})
	if err != nil {
		t.Fatal(err)
	}
	project, err := f.service.CreateRows(f.ctx, owner, projects.Table.ID, []RowInsert{{
		Fields: map[string]json.RawMessage{
			projectLabel.ID:       json.RawMessage(`"Panda"`),
			projectOwner.Field.ID: json.RawMessage(`"Alice"`),
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	task, err := f.service.CreateRows(f.ctx, owner, tasks.Table.ID, []RowInsert{{
		Fields: map[string]json.RawMessage{taskLabel.ID: json.RawMessage(`"Build"`)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ChangeLinks(f.ctx, owner, tasks.Table.ID, task[0].ID,
		relation.Relation.ChildFieldID, LinkChange{Add: []string{project[0].ID}}); err != nil {
		t.Fatal(err)
	}
	lookup := computedFieldFor(t, f, owner, tasks.Table.ID, "Project owner", "lookup", computedOptions{
		RelationFieldID: relation.Relation.ChildFieldID, TargetFieldID: projectOwner.Field.ID,
	})
	rows := computedRows(t, f, owner, relation.Schema)
	var got []string
	if err := json.Unmarshal(rows[0].Fields[lookup.ID], &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"Alice"}) {
		t.Fatalf("lookup = %v, want [Alice]", got)
	}
	if _, err := f.service.PatchRow(f.ctx, owner, projects.Table.ID, project[0].ID,
		map[string]json.RawMessage{projectOwner.Field.ID: json.RawMessage(`"Bob"`)}); err != nil {
		t.Fatal(err)
	}
	rows = computedRows(t, f, owner, relation.Schema)
	if err := json.Unmarshal(rows[0].Fields[lookup.ID], &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"Bob"}) {
		t.Fatalf("updated lookup = %v, want [Bob]", got)
	}
}

func TestRollupComputedAggregates(t *testing.T) {
	f := newFixture(t)
	owner := f.user("user", "active", "en")
	workspace := f.workspace(owner)
	book := f.book(owner, workspace.ID)
	projects, _ := relationTable(t, f, owner, book.ID, "Projects")
	tasks, label := relationTable(t, f, owner, book.ID, "Tasks")
	hours, err := f.service.CreateField(f.ctx, owner, tasks.Table.ID, FieldInput{
		Name: "Hours", UIType: "integer",
	})
	if err != nil {
		t.Fatal(err)
	}
	day, err := f.service.CreateField(f.ctx, owner, tasks.Table.ID, FieldInput{
		Name: "Day", UIType: "date",
	})
	if err != nil {
		t.Fatal(err)
	}
	relation, err := f.service.CreateRelation(f.ctx, owner, projects.Table.ID, RelationInput{
		Name: "Tasks", ReverseName: "Project", TargetTableID: tasks.Table.ID,
		Kind: "one_many", SourceSide: "parent",
	})
	if err != nil {
		t.Fatal(err)
	}
	project, err := f.service.CreateRows(f.ctx, owner, projects.Table.ID, []RowInsert{{Fields: map[string]json.RawMessage{}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		label, hours, day string
	}{
		{"A", "5", `"2026-01-02"`},
		{"B", "7", `"2026-01-05"`},
	} {
		created, err := f.service.CreateRows(f.ctx, owner, tasks.Table.ID, []RowInsert{{
			Fields: map[string]json.RawMessage{
				label.ID:       json.RawMessage(`"` + row.label + `"`),
				hours.Field.ID: json.RawMessage(row.hours),
				day.Field.ID:   json.RawMessage(row.day),
			},
		}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.service.ChangeLinks(f.ctx, owner, projects.Table.ID, project[0].ID,
			relation.Relation.ParentFieldID, LinkChange{Add: []string{created[0].ID}}); err != nil {
			t.Fatal(err)
		}
	}
	specs := []struct {
		name, aggregate, target string
		want                    any
	}{
		{"Count", "count", hours.Field.ID, float64(2)},
		{"Sum", "sum", hours.Field.ID, float64(12)},
		{"Average", "avg", hours.Field.ID, float64(6)},
		{"Minimum", "min", hours.Field.ID, float64(5)},
		{"Maximum", "max", hours.Field.ID, float64(7)},
		{"Earliest", "earliest", day.Field.ID, "2026-01-02"},
		{"Latest", "latest", day.Field.ID, "2026-01-05"},
		{"Concatenate", "concat", label.ID, "A, B"},
	}
	createdFields := make([]Field, len(specs))
	for index, spec := range specs {
		createdFields[index] = computedFieldFor(t, f, owner, projects.Table.ID, spec.name, "rollup", computedOptions{
			RelationFieldID: relation.Relation.ParentFieldID, TargetFieldID: spec.target, Aggregate: spec.aggregate,
		})
	}
	rows := computedRows(t, f, owner, relation.Schema)
	for index, spec := range specs {
		var got any
		if err := json.Unmarshal(rows[0].Fields[createdFields[index].ID], &got); err != nil {
			t.Fatal(err)
		}
		if got != spec.want {
			t.Errorf("%s = %v, want %v", spec.name, got, spec.want)
		}
	}
}

func TestComputedOneToOneAndManyToMany(t *testing.T) {
	t.Run("one-to-one lookup", func(t *testing.T) {
		f := newFixture(t)
		owner := f.user("user", "active", "en")
		workspace := f.workspace(owner)
		book := f.book(owner, workspace.ID)
		people, personLabel := relationTable(t, f, owner, book.ID, "People")
		profiles, profileLabel := relationTable(t, f, owner, book.ID, "Profiles")
		email, err := f.service.CreateField(f.ctx, owner, people.Table.ID, FieldInput{
			Name: "Email", UIType: "email",
		})
		if err != nil {
			t.Fatal(err)
		}
		relation, err := f.service.CreateRelation(f.ctx, owner, profiles.Table.ID, RelationInput{
			Name: "Person", ReverseName: "Profile", TargetTableID: people.Table.ID,
			Kind: "one_one", SourceSide: "child",
		})
		if err != nil {
			t.Fatal(err)
		}
		personRows, err := f.service.CreateRows(f.ctx, owner, people.Table.ID, []RowInsert{{
			Fields: map[string]json.RawMessage{
				personLabel.ID: json.RawMessage(`"Panda"`),
				email.Field.ID: json.RawMessage(`"panda@example.test"`),
			},
		}})
		if err != nil {
			t.Fatal(err)
		}
		profile := relationRow(t, f, owner, profiles.Table.ID, profileLabel.ID, "Profile")
		if _, err := f.service.ChangeLinks(f.ctx, owner, profiles.Table.ID, profile,
			relation.Relation.ChildFieldID, LinkChange{Add: []string{personRows[0].ID}}); err != nil {
			t.Fatal(err)
		}
		lookup := computedFieldFor(t, f, owner, profiles.Table.ID, "Email", "lookup", computedOptions{
			RelationFieldID: relation.Relation.ChildFieldID, TargetFieldID: email.Field.ID,
		})
		rows := computedRows(t, f, owner, relation.Schema)
		var got []string
		if err := json.Unmarshal(rows[0].Fields[lookup.ID], &got); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, []string{"panda@example.test"}) {
			t.Fatalf("one-to-one lookup = %v, want [panda@example.test]", got)
		}
	})

	t.Run("many-to-many both directions", func(t *testing.T) {
		f := newFixture(t)
		owner := f.user("user", "active", "en")
		workspace := f.workspace(owner)
		book := f.book(owner, workspace.ID)
		tasks, taskLabel := relationTable(t, f, owner, book.ID, "Tasks")
		tags, tagLabel := relationTable(t, f, owner, book.ID, "Tags")
		weight, err := f.service.CreateField(f.ctx, owner, tags.Table.ID, FieldInput{
			Name: "Weight", UIType: "integer",
		})
		if err != nil {
			t.Fatal(err)
		}
		relation, err := f.service.CreateRelation(f.ctx, owner, tasks.Table.ID, RelationInput{
			Name: "Tags", ReverseName: "Tasks", TargetTableID: tags.Table.ID, Kind: "many_many",
		})
		if err != nil {
			t.Fatal(err)
		}
		task := relationRow(t, f, owner, tasks.Table.ID, taskLabel.ID, "Task")
		tag, err := f.service.CreateRows(f.ctx, owner, tags.Table.ID, []RowInsert{{
			Fields: map[string]json.RawMessage{
				tagLabel.ID:     json.RawMessage(`"Tag"`),
				weight.Field.ID: json.RawMessage(`3`),
			},
		}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.service.ChangeLinks(f.ctx, owner, tasks.Table.ID, task,
			relation.Relation.ParentFieldID, LinkChange{Add: []string{tag[0].ID}}); err != nil {
			t.Fatal(err)
		}
		taskLookup := computedFieldFor(t, f, owner, tasks.Table.ID, "Tag lookup", "lookup", computedOptions{
			RelationFieldID: relation.Relation.ParentFieldID, TargetFieldID: tagLabel.ID,
		})
		tagLookup := computedFieldFor(t, f, owner, tags.Table.ID, "Task lookup", "lookup", computedOptions{
			RelationFieldID: relation.Relation.ChildFieldID, TargetFieldID: taskLabel.ID,
		})
		rollup := computedFieldFor(t, f, owner, tasks.Table.ID, "Weight sum", "rollup", computedOptions{
			RelationFieldID: relation.Relation.ParentFieldID, TargetFieldID: weight.Field.ID, Aggregate: "sum",
		})
		taskRows := computedRows(t, f, owner, relation.Schema)
		tagRows := computedRows(t, f, owner, tags)
		var gotTags, gotTasks []string
		if err := json.Unmarshal(taskRows[0].Fields[taskLookup.ID], &gotTags); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(tagRows[0].Fields[tagLookup.ID], &gotTasks); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(gotTags, []string{"Tag"}) || !reflect.DeepEqual(gotTasks, []string{"Task"}) {
			t.Fatalf("many-to-many lookups = %v / %v", gotTags, gotTasks)
		}
		var gotWeight float64
		if err := json.Unmarshal(taskRows[0].Fields[rollup.ID], &gotWeight); err != nil {
			t.Fatal(err)
		}
		if gotWeight != 3 {
			t.Fatalf("many-to-many rollup = %v, want 3", gotWeight)
		}
	})
}

func TestComputedValuesMaskedWithoutTargetRead(t *testing.T) {
	f := newFixture(t)
	owner := f.user("user", "active", "en")
	workspace := f.workspace(owner)
	book := f.book(owner, workspace.ID)
	projects, projectLabel := relationTable(t, f, owner, book.ID, "Projects")
	tasks, taskLabel := relationTable(t, f, owner, book.ID, "Tasks")
	projectSecret, err := f.service.CreateField(f.ctx, owner, projects.Table.ID, FieldInput{
		Name: "Secret", UIType: "text",
	})
	if err != nil {
		t.Fatal(err)
	}
	relation, err := f.service.CreateRelation(f.ctx, owner, tasks.Table.ID, RelationInput{
		Name: "Project", ReverseName: "Tasks", TargetTableID: projects.Table.ID,
		Kind: "one_many", SourceSide: "child",
	})
	if err != nil {
		t.Fatal(err)
	}
	project, err := f.service.CreateRows(f.ctx, owner, projects.Table.ID, []RowInsert{{Fields: map[string]json.RawMessage{
		projectLabel.ID:        json.RawMessage(`"Visible label"`),
		projectSecret.Field.ID: json.RawMessage(`"Private"`),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	task, err := f.service.CreateRows(f.ctx, owner, tasks.Table.ID, []RowInsert{{Fields: map[string]json.RawMessage{
		taskLabel.ID: json.RawMessage(`"Task"`),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ChangeLinks(f.ctx, owner, tasks.Table.ID, task[0].ID,
		relation.Relation.ChildFieldID, LinkChange{Add: []string{project[0].ID}}); err != nil {
		t.Fatal(err)
	}
	lookup := computedFieldFor(t, f, owner, tasks.Table.ID, "Secret lookup", "lookup", computedOptions{
		RelationFieldID: relation.Relation.ChildFieldID, TargetFieldID: projectSecret.Field.ID,
	})
	reader := f.user("user", "active", "en")
	f.grant(acl.Table, tasks.Table.ID, "user", reader.ID, acl.Read)
	rows := computedRows(t, f, reader, relation.Schema)
	var got string
	if err := json.Unmarshal(rows[0].Fields[lookup.ID], &got); err != nil {
		t.Fatal(err)
	}
	if got != "🔒" {
		t.Fatalf("masked lookup = %q, want lock", got)
	}
	var links []LinkValue
	if err := json.Unmarshal(rows[0].Fields[relation.Relation.ChildFieldID], &links); err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 || links[0].Label != "Visible label" {
		t.Fatalf("link display values = %+v", links)
	}
}

func TestComputedDependenciesPreventDeletingReferencedFields(t *testing.T) {
	f := newFixture(t)
	owner := f.user("user", "active", "en")
	workspace := f.workspace(owner)
	book := f.book(owner, workspace.ID)
	projects, _ := relationTable(t, f, owner, book.ID, "Projects")
	tasks, _ := relationTable(t, f, owner, book.ID, "Tasks")
	projectOwner, err := f.service.CreateField(f.ctx, owner, projects.Table.ID, FieldInput{
		Name: "Owner", UIType: "text",
	})
	if err != nil {
		t.Fatal(err)
	}
	relation, err := f.service.CreateRelation(f.ctx, owner, tasks.Table.ID, RelationInput{
		Name: "Project", ReverseName: "Tasks", TargetTableID: projects.Table.ID,
		Kind: "one_many", SourceSide: "child",
	})
	if err != nil {
		t.Fatal(err)
	}
	computedFieldFor(t, f, owner, tasks.Table.ID, "Project owner", "lookup", computedOptions{
		RelationFieldID: relation.Relation.ChildFieldID, TargetFieldID: projectOwner.Field.ID,
	})
	_, err = f.service.DeleteField(f.ctx, owner, projectOwner.Field.ID, projectOwner.Field.Version)
	expectError(t, err, 409, "ERR_FIELD_IN_USE")
	_, err = f.service.DeleteField(f.ctx, owner, relation.Field.ID, relation.Field.Version)
	expectError(t, err, 409, "ERR_FIELD_IN_USE")

	rows := computedRows(t, f, owner, relation.Schema)
	if len(rows) != 0 {
		t.Fatalf("unexpected rows after rejected dependency deletion: %d", len(rows))
	}
}
