package structure

import (
	"encoding/json"
	"reflect"
	"testing"

	"pandatable/internal/acl"
)

func draftLinks(ids ...string) json.RawMessage {
	links := make([]LinkValue, len(ids))
	for index, id := range ids {
		links[index] = LinkValue{ID: id, Label: "untrusted client label"}
	}
	raw, _ := json.Marshal(links)
	return raw
}

func TestPatchRowMixedRelationDraft(t *testing.T) {
	for _, kind := range []string{"many_many", "one_many", "one_one"} {
		for _, side := range []string{"parent", "child"} {
			t.Run(kind+"/"+side, func(t *testing.T) {
				f := newFixture(t)
				owner := f.user("user", "active", "en")
				book := f.book(owner, f.workspace(owner).ID)
				source, sourceLabel := relationTable(t, f, owner, book.ID, "Source")
				target, targetLabel := relationTable(t, f, owner, book.ID, "Target")
				from, to, creationSide := source.Table.ID, target.Table.ID, side
				if kind == "many_many" && side == "child" {
					from, to, creationSide = to, from, "parent"
				}
				relation, err := f.service.CreateRelation(f.ctx, owner, from, RelationInput{
					Name: "Links", ReverseName: "Reverse", TargetTableID: to,
					Kind: kind, SourceSide: creationSide,
				})
				if err != nil {
					t.Fatal(err)
				}
				fieldID := relation.Relation.ParentFieldID
				if side == "child" {
					fieldID = relation.Relation.ChildFieldID
				}
				sourceID := relationRow(t, f, owner, source.Table.ID, sourceLabel.ID, "Before")
				first := relationRow(t, f, owner, target.Table.ID, targetLabel.ID, "First")
				second := relationRow(t, f, owner, target.Table.ID, targetLabel.ID, "Second")
				if _, err := f.service.ChangeLinks(f.ctx, owner, source.Table.ID, sourceID,
					fieldID, LinkChange{Add: []string{first}}); err != nil {
					t.Fatal(err)
				}
				patched, err := f.service.PatchRow(f.ctx, owner, source.Table.ID, sourceID, map[string]json.RawMessage{
					sourceLabel.ID: rawJSON(`"After"`), fieldID: draftLinks(second),
				})
				if err != nil {
					t.Fatal(err)
				}
				links := relationValues(t, patched, fieldID)
				if string(patched.Fields[sourceLabel.ID]) != `"After"` || len(links) != 1 ||
					links[0].ID != second || links[0].Label != "Second" {
					t.Fatalf("mixed patch = %+v, links = %+v", patched, links)
				}
				var previous []LinkValue
				if err := json.Unmarshal(patched.Previous[fieldID], &previous); err != nil {
					t.Fatal(err)
				}
				if string(patched.Previous[sourceLabel.ID]) != `"Before"` || len(previous) != 1 ||
					previous[0].ID != first || previous[0].Label != "First" || len(patched.LinkChanges) != 2 {
					t.Fatalf("undo values = %+v, deltas = %+v", patched.Previous, patched.LinkChanges)
				}
				reverseField := relation.Relation.ChildFieldID
				if side == "child" {
					reverseField = relation.Relation.ParentFieldID
				}
				reverse, err := f.service.GetRow(f.ctx, owner, target.Table.ID, second)
				if err != nil {
					t.Fatal(err)
				}
				reverseLinks := relationValues(t, reverse, reverseField)
				if len(reverseLinks) != 1 || reverseLinks[0].ID != sourceID || reverseLinks[0].Label != "After" {
					t.Fatalf("reverse patch = %+v", reverseLinks)
				}
				requireHistoryAction(t, historyFor(t, f, owner, source.Table.ID, sourceID), "update")
				linkHistory := requireHistoryAction(t, historyFor(t, f, owner, target.Table.ID, second), "link")
				var historyLinks []LinkValue
				if err := json.Unmarshal(linkHistory.Changes[reverseField].New, &historyLinks); err != nil {
					t.Fatal(err)
				}
				if len(historyLinks) != 1 || historyLinks[0].ID != sourceID || historyLinks[0].Label != "After" {
					t.Fatalf("authoritative reverse history = %+v", linkHistory)
				}

				// Labels are presentation, not undo guards; compare linked ID sets.
				undone, err := f.service.PatchRows(f.ctx, owner, source.Table.ID, []RowPatch{{
					ID: sourceID, Fields: patched.Previous, Expected: map[string]json.RawMessage{
						sourceLabel.ID: rawJSON(`"After"`), fieldID: draftLinks(second),
					},
				}})
				if err != nil || len(undone) != 1 || string(undone[0].Fields[sourceLabel.ID]) != `"Before"` {
					t.Fatalf("guarded mixed undo = %+v, %v", undone, err)
				}
				if links := relationValues(t, undone[0], fieldID); len(links) != 1 || links[0].ID != first {
					t.Fatalf("undone links = %+v", links)
				}
			})
		}
	}
}

func TestPatchRowRelationRollback(t *testing.T) {
	f := newFixture(t)
	owner := f.user("user", "active", "en")
	writer := f.user("user", "active", "en")
	book := f.book(owner, f.workspace(owner).ID)
	source, label := relationTable(t, f, owner, book.ID, "Source")
	target, targetLabel := relationTable(t, f, owner, book.ID, "Target")
	relation, err := f.service.CreateRelation(f.ctx, owner, source.Table.ID, RelationInput{
		Name: "Target", ReverseName: "Source", TargetTableID: target.Table.ID, Kind: "one_one", SourceSide: "child",
	})
	if err != nil {
		t.Fatal(err)
	}
	rowID := relationRow(t, f, owner, source.Table.ID, label.ID, "Before")
	targetID := relationRow(t, f, owner, target.Table.ID, targetLabel.ID, "Target")
	f.grant(acl.Table, source.Table.ID, "user", writer.ID, acl.Write)
	f.grant(acl.Table, target.Table.ID, "user", writer.ID, acl.Read)
	before, err := f.service.GetRow(f.ctx, owner, source.Table.ID, rowID)
	if err != nil {
		t.Fatal(err)
	}
	history := historyFor(t, f, owner, source.Table.ID, rowID)
	for _, test := range []struct {
		name   string
		actor  Actor
		value  json.RawMessage
		status int
		code   string
	}{
		{"missing target", owner, draftLinks("999999"), 404, "ERR_NOT_FOUND"},
		{"invalid target", owner, draftLinks("01"), 400, "ERR_VALIDATION"},
		{"invalid relation value", owner, rawJSON(`null`), 400, "ERR_VALIDATION"},
		{"excess cardinality", owner, draftLinks(targetID, "999999"), 400, "ERR_VALIDATION"},
		{"target read only", writer, draftLinks(targetID), 403, "ERR_PERMISSION_DENIED"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := f.service.PatchRow(f.ctx, test.actor, source.Table.ID, rowID, map[string]json.RawMessage{
				label.ID: rawJSON(`"Must roll back"`), relation.Field.ID: test.value,
			})
			expectError(t, err, test.status, test.code)
			after, err := f.service.GetRow(f.ctx, owner, source.Table.ID, rowID)
			if err != nil || !reflect.DeepEqual(before.Fields, after.Fields) {
				t.Fatalf("failed patch changed row: before=%+v after=%+v err=%v", before, after, err)
			}
			if actual := historyFor(t, f, owner, source.Table.ID, rowID); !reflect.DeepEqual(history, actual) {
				t.Fatalf("failed patch wrote history: %+v", actual)
			}
		})
	}
	t.Run("later bulk failure rolls back links and both histories", func(t *testing.T) {
		second := relationRow(t, f, owner, source.Table.ID, label.ID, "Second")
		targetBefore, err := f.service.GetRow(f.ctx, owner, target.Table.ID, targetID)
		if err != nil {
			t.Fatal(err)
		}
		targetHistory := historyFor(t, f, owner, target.Table.ID, targetID)
		_, err = f.service.PatchRows(f.ctx, owner, source.Table.ID, []RowPatch{
			{ID: rowID, Fields: map[string]json.RawMessage{
				label.ID: rawJSON(`"Must roll back"`), relation.Field.ID: draftLinks(targetID),
			}},
			{ID: second, Fields: map[string]json.RawMessage{relation.Field.ID: draftLinks("999999")}},
		})
		expectError(t, err, 404, "ERR_NOT_FOUND")
		sourceAfter, err := f.service.GetRow(f.ctx, owner, source.Table.ID, rowID)
		if err != nil || !reflect.DeepEqual(before.Fields, sourceAfter.Fields) {
			t.Fatalf("later failure changed earlier source: %+v, %v", sourceAfter, err)
		}
		targetAfter, err := f.service.GetRow(f.ctx, owner, target.Table.ID, targetID)
		if err != nil || !reflect.DeepEqual(targetBefore.Fields, targetAfter.Fields) {
			t.Fatalf("later failure changed reverse target: %+v, %v", targetAfter, err)
		}
		if actual := historyFor(t, f, owner, source.Table.ID, rowID); !reflect.DeepEqual(history, actual) {
			t.Fatalf("later failure retained earlier source history: %+v", actual)
		}
		if actual := historyFor(t, f, owner, target.Table.ID, targetID); !reflect.DeepEqual(targetHistory, actual) {
			t.Fatalf("later failure retained earlier target history: %+v", actual)
		}
	})
}

func TestPatchRowsRelationGuardsAndDisplacement(t *testing.T) {
	f := newFixture(t)
	owner := f.user("user", "active", "en")
	book := f.book(owner, f.workspace(owner).ID)
	parent, parentLabel := relationTable(t, f, owner, book.ID, "People")
	child, childLabel := relationTable(t, f, owner, book.ID, "Profiles")
	relation, err := f.service.CreateRelation(f.ctx, owner, child.Table.ID, RelationInput{
		Name: "Person", ReverseName: "Profile", TargetTableID: parent.Table.ID, Kind: "one_one", SourceSide: "child",
	})
	if err != nil {
		t.Fatal(err)
	}
	first := relationRow(t, f, owner, child.Table.ID, childLabel.ID, "First")
	second := relationRow(t, f, owner, child.Table.ID, childLabel.ID, "Second")
	personA := relationRow(t, f, owner, parent.Table.ID, parentLabel.ID, "A")
	personB := relationRow(t, f, owner, parent.Table.ID, parentLabel.ID, "B")
	for row, person := range map[string]string{first: personA, second: personB} {
		if _, err := f.service.ChangeLinks(f.ctx, owner, child.Table.ID, row, relation.Field.ID,
			LinkChange{Add: []string{person}}); err != nil {
			t.Fatal(err)
		}
	}
	patch := []RowPatch{
		{ID: first, Fields: map[string]json.RawMessage{relation.Field.ID: draftLinks(personB)},
			Expected: map[string]json.RawMessage{relation.Field.ID: draftLinks(personA)}},
		{ID: second, Fields: map[string]json.RawMessage{relation.Field.ID: draftLinks(personA)},
			Expected: map[string]json.RawMessage{relation.Field.ID: draftLinks(personB)}},
	}
	swapped, err := f.service.PatchRows(f.ctx, owner, child.Table.ID, patch)
	if err != nil {
		t.Fatal(err)
	}
	if len(swapped[0].LinkChanges) != 3 || len(swapped[1].LinkChanges) != 1 {
		t.Fatalf("missing displaced deltas: %+v / %+v", swapped[0].LinkChanges, swapped[1].LinkChanges)
	}
	for index, row := range swapped {
		if !reflect.DeepEqual(row.Previous[relation.Field.ID], mustLinkJSON(t, []LinkValue{{
			ID: []string{personA, personB}[index], Label: []string{"A", "B"}[index],
		}})) {
			t.Fatalf("previous must reflect pre-batch links: %+v", swapped)
		}
		patch[index].Fields = row.Previous
		patch[index].Expected = map[string]json.RawMessage{relation.Field.ID: row.Fields[relation.Field.ID]}
	}
	// All guards must be checked before either restoration displaces the other row.
	restored, err := f.service.PatchRows(f.ctx, owner, child.Table.ID, patch)
	if err != nil || len(restored) != 2 {
		t.Fatalf("guarded swap undo = %+v, %v", restored, err)
	}
	for index, row := range restored {
		if links := relationValues(t, row, relation.Field.ID); len(links) != 1 || links[0].ID != []string{personA, personB}[index] {
			t.Fatalf("restored links = %+v", links)
		}
	}
	patch[0].Fields[childLabel.ID] = rawJSON(`"Must roll back"`)
	patch[0].Expected[childLabel.ID] = rawJSON(`"First"`)
	patch[1].Expected[relation.Field.ID] = draftLinks()
	_, err = f.service.PatchRows(f.ctx, owner, child.Table.ID, patch)
	expectError(t, err, 409, "ERR_UNDO_CONFLICT")
	unchanged, err := f.service.GetRow(f.ctx, owner, child.Table.ID, first)
	if err != nil || string(unchanged.Fields[childLabel.ID]) != `"First"` {
		t.Fatalf("conflicting bulk undo changed an earlier row: %+v, %v", unchanged, err)
	}
}

func TestPatchRowRelationDisplacedUndo(t *testing.T) {
	for _, test := range []struct{ kind, side string }{
		{"one_one", "parent"}, {"one_one", "child"}, {"one_many", "parent"},
	} {
		t.Run(test.kind+"/"+test.side, func(t *testing.T) {
			f := newFixture(t)
			owner := f.user("user", "active", "en")
			book := f.book(owner, f.workspace(owner).ID)
			source, sourceLabel := relationTable(t, f, owner, book.ID, "Source")
			target, targetLabel := relationTable(t, f, owner, book.ID, "Target")
			relation, err := f.service.CreateRelation(f.ctx, owner, source.Table.ID, RelationInput{
				Name: "Links", ReverseName: "Reverse", TargetTableID: target.Table.ID,
				Kind: test.kind, SourceSide: test.side,
			})
			if err != nil {
				t.Fatal(err)
			}
			first := relationRow(t, f, owner, source.Table.ID, sourceLabel.ID, "First")
			second := relationRow(t, f, owner, source.Table.ID, sourceLabel.ID, "Second")
			targetA := relationRow(t, f, owner, target.Table.ID, targetLabel.ID, "A")
			targetB := relationRow(t, f, owner, target.Table.ID, targetLabel.ID, "B")
			for row, linked := range map[string]string{first: targetA, second: targetB} {
				if _, err := f.service.ChangeLinks(f.ctx, owner, source.Table.ID, row, relation.Field.ID,
					LinkChange{Add: []string{linked}}); err != nil {
					t.Fatal(err)
				}
			}
			patched, err := f.service.PatchRow(f.ctx, owner, source.Table.ID, first,
				map[string]json.RawMessage{relation.Field.ID: draftLinks(targetB)})
			if err != nil || len(patched.LinkChanges) != 3 {
				t.Fatalf("replacement deltas = %+v, %v", patched.LinkChanges, err)
			}
			displaced, err := f.service.GetRow(f.ctx, owner, source.Table.ID, second)
			if err != nil || len(relationValues(t, displaced, relation.Field.ID)) != 0 {
				t.Fatalf("displaced source = %+v, %v", displaced, err)
			}
			for index := len(patched.LinkChanges) - 1; index >= 0; index-- {
				delta := patched.LinkChanges[index]
				change := LinkChange{}
				if delta.Action == "link" {
					change.Remove = []string{delta.TargetID}
				} else {
					change.Add = []string{delta.TargetID}
				}
				if _, err := f.service.ChangeLinks(f.ctx, owner, delta.TableID, delta.RowID, delta.FieldID, change); err != nil {
					t.Fatal(err)
				}
			}
			for rowID, targetID := range map[string]string{first: targetA, second: targetB} {
				row, err := f.service.GetRow(f.ctx, owner, source.Table.ID, rowID)
				if err != nil {
					t.Fatal(err)
				}
				if links := relationValues(t, row, relation.Field.ID); len(links) != 1 || links[0].ID != targetID {
					t.Fatalf("delta undo did not restore displaced source %s: %+v", rowID, links)
				}
			}
		})
	}
}

func mustLinkJSON(t *testing.T, links []LinkValue) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(links)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
