package structure

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"pandatable/internal/acl"
)

func historyFor(t *testing.T, f *fixture, actor Actor, tableID, rowID string) []RowHistory {
	t.Helper()
	page, err := f.service.ListRowHistory(f.ctx, actor, tableID, rowID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if page.HasMore {
		t.Fatal("unexpected additional history page")
	}
	return page.Items
}

func requireHistoryAction(t *testing.T, entries []RowHistory, action string) RowHistory {
	t.Helper()
	for _, entry := range entries {
		if entry.Action == action {
			return entry
		}
	}
	t.Fatalf("missing %s history: %+v", action, entries)
	return RowHistory{}
}

func historyLinks(t *testing.T, raw json.RawMessage) []LinkValue {
	t.Helper()
	var links []LinkValue
	if err := json.Unmarshal(raw, &links); err != nil {
		t.Fatalf("decode history links %s: %v", raw, err)
	}
	return links
}

func rejectHistoryInserts(t *testing.T, f *fixture, actor Actor) {
	t.Helper()
	functionName := "reject_history_" + strings.ReplaceAll(actor.ID, "-", "")[:12]
	function := pgx.Identifier{"meta", functionName}.Sanitize()
	trigger := pgx.Identifier{functionName}.Sanitize()
	f.exec("CREATE FUNCTION " + function + `() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'injected history failure'; END $$`)
	f.exec("CREATE TRIGGER " + trigger + " BEFORE INSERT ON meta.row_history FOR EACH ROW EXECUTE FUNCTION " + function + "()")
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := f.pool.Exec(ctx, "DROP TRIGGER "+trigger+" ON meta.row_history"); err != nil {
			t.Error(err)
		}
		if _, err := f.pool.Exec(ctx, "DROP FUNCTION "+function+"()"); err != nil {
			t.Error(err)
		}
	})
}

func TestHistoryRowSnapshotsChangesAndRollback(t *testing.T) {
	f := newFixture(t)
	owner := f.user("user", "active", "en")
	ft := f.fieldTable(owner)
	title := ft.create(FieldInput{Name: "Title", UIType: "text", Default: rawJSON(`"Default"`)})
	number := ft.create(FieldInput{Name: "Number", UIType: "integer"})
	data := ft.create(FieldInput{Name: "Data", UIType: "json"})
	formula := ft.create(FieldInput{Name: "Formula", UIType: "formula", Options: rawJSON(`{"expr":"1+1"}`)})

	rows, err := f.service.CreateRows(f.ctx, owner, ft.id(), []RowInsert{
		{Fields: map[string]json.RawMessage{number.ID: rawJSON(`1`), data.ID: rawJSON(`{"a":1,"b":2}`)}},
		{Fields: map[string]json.RawMessage{title.ID: rawJSON(`"Second"`), number.ID: rawJSON(`2`)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	first, second := rows[0], rows[1]
	insert := requireHistoryAction(t, historyFor(t, f, owner, ft.id(), first.ID), "insert")
	if string(insert.Changes[title.ID].New) != `"Default"` ||
		string(insert.Changes[number.ID].New) != `1` {
		t.Fatalf("insert does not contain persisted defaults: %+v", insert.Changes)
	}
	if _, ok := insert.Changes[formula.ID]; ok {
		t.Fatalf("formula captured in history: %+v", insert.Changes)
	}
	for _, field := range ft.schema.Fields {
		if field.IsSystem {
			if _, ok := insert.Changes[field.ID]; ok {
				t.Fatalf("system field captured in history: %s", field.ID)
			}
		}
	}
	for _, change := range insert.Changes {
		if len(change.Old) != 0 || len(change.New) == 0 {
			t.Fatalf("insert has an unexpected old or missing new value: %+v", change)
		}
	}

	_, err = f.service.PatchRow(f.ctx, owner, ft.id(), first.ID, map[string]json.RawMessage{
		title.ID: rawJSON(`"Edited"`), number.ID: rawJSON(`1`), data.ID: rawJSON(`{"b":2,"a":1}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	entries := historyFor(t, f, owner, ft.id(), first.ID)
	update := requireHistoryAction(t, entries, "update")
	if len(entries) != 2 || len(update.Changes) != 1 ||
		string(update.Changes[title.ID].Old) != `"Default"` || string(update.Changes[title.ID].New) != `"Edited"` {
		t.Fatalf("update did not capture only the changed value: %+v", entries)
	}
	if _, err := f.service.PatchRow(f.ctx, owner, ft.id(), first.ID,
		map[string]json.RawMessage{title.ID: rawJSON(`"Edited"`)}); err != nil {
		t.Fatal(err)
	}
	if got := len(historyFor(t, f, owner, ft.id(), first.ID)); got != 2 {
		t.Fatalf("no-op patch created history: %d", got)
	}

	_, err = f.service.PatchRows(f.ctx, owner, ft.id(), []RowPatch{
		{ID: second.ID, Fields: map[string]json.RawMessage{title.ID: rawJSON(`"Pasted 2"`)}},
		{ID: first.ID, Fields: map[string]json.RawMessage{title.ID: rawJSON(`"Pasted 1"`)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(historyFor(t, f, owner, ft.id(), first.ID)); got != 3 {
		t.Fatalf("first bulk row has %d history entries", got)
	}
	if got := len(historyFor(t, f, owner, ft.id(), second.ID)); got != 2 {
		t.Fatalf("second bulk row has %d history entries", got)
	}
	_, err = f.service.PatchRows(f.ctx, owner, ft.id(), []RowPatch{
		{ID: first.ID, Fields: map[string]json.RawMessage{title.ID: rawJSON(`"Must roll back"`)}},
		{ID: second.ID, Fields: map[string]json.RawMessage{number.ID: rawJSON(`"invalid"`)}},
	})
	expectError(t, err, 400, "ERR_VALIDATION")
	firstAfter, err := f.service.GetRow(f.ctx, owner, ft.id(), first.ID)
	if err != nil || string(firstAfter.Fields[title.ID]) != `"Pasted 1"` ||
		len(historyFor(t, f, owner, ft.id(), first.ID)) != 3 {
		t.Fatalf("failed bulk update did not roll back: %+v, %v", firstAfter, err)
	}

	rejectHistoryInserts(t, f, owner)
	_, err = f.service.PatchRow(f.ctx, owner, ft.id(), first.ID,
		map[string]json.RawMessage{title.ID: rawJSON(`"Rejected"`)})
	if err == nil {
		t.Fatal("history insertion failure did not fail the mutation")
	}
	firstAfter, err = f.service.GetRow(f.ctx, owner, ft.id(), first.ID)
	if err != nil || string(firstAfter.Fields[title.ID]) != `"Pasted 1"` ||
		len(historyFor(t, f, owner, ft.id(), first.ID)) != 3 {
		t.Fatalf("failed history insert did not roll back the mutation: %+v, %v", firstAfter, err)
	}
}

func TestHistoryLinksBulkReplacementAndDelete(t *testing.T) {
	t.Run("many-to-many bulk and unlink", func(t *testing.T) {
		f := newFixture(t)
		owner := f.user("user", "active", "en")
		book := f.book(owner, f.workspace(owner).ID)
		source, sourceLabel := relationTable(t, f, owner, book.ID, "Tasks")
		target, targetLabel := relationTable(t, f, owner, book.ID, "Tags")
		created, err := f.service.CreateRelation(f.ctx, owner, source.Table.ID, RelationInput{
			Name: "Tags", ReverseName: "Tasks", TargetTableID: target.Table.ID, Kind: "many_many",
		})
		if err != nil {
			t.Fatal(err)
		}
		sourceID := relationRow(t, f, owner, source.Table.ID, sourceLabel.ID, "Task")
		firstID := relationRow(t, f, owner, target.Table.ID, targetLabel.ID, "First")
		secondID := relationRow(t, f, owner, target.Table.ID, targetLabel.ID, "Second")
		change := LinkChange{Add: []string{firstID, secondID}}
		if _, err := f.service.ChangeLinks(f.ctx, owner, source.Table.ID, sourceID, created.Relation.ParentFieldID, change); err != nil {
			t.Fatal(err)
		}
		sourceHistory := historyFor(t, f, owner, source.Table.ID, sourceID)
		linked := requireHistoryAction(t, sourceHistory, "link")
		links := historyLinks(t, linked.Changes[created.Relation.ParentFieldID].New)
		if len(sourceHistory) != 2 || len(links) != 2 ||
			links[0] != (LinkValue{ID: firstID, Label: "First"}) ||
			links[1] != (LinkValue{ID: secondID, Label: "Second"}) {
			t.Fatalf("bulk source link history = %+v, %+v", sourceHistory, links)
		}
		reverse := requireHistoryAction(t, historyFor(t, f, owner, target.Table.ID, firstID), "link")
		links = historyLinks(t, reverse.Changes[created.Relation.ChildFieldID].New)
		if len(links) != 1 || links[0] != (LinkValue{ID: sourceID, Label: "Task"}) {
			t.Fatalf("reverse link history = %+v", links)
		}
		if _, err := f.service.ChangeLinks(f.ctx, owner, source.Table.ID, sourceID, created.Relation.ParentFieldID, change); err != nil {
			t.Fatal(err)
		}
		if got := len(historyFor(t, f, owner, source.Table.ID, sourceID)); got != 2 {
			t.Fatalf("idempotent link created history: %d", got)
		}
		if _, err := f.service.ChangeLinks(f.ctx, owner, source.Table.ID, sourceID, created.Relation.ParentFieldID,
			LinkChange{Remove: []string{firstID}}); err != nil {
			t.Fatal(err)
		}
		unlinked := requireHistoryAction(t, historyFor(t, f, owner, source.Table.ID, sourceID), "unlink")
		links = historyLinks(t, unlinked.Changes[created.Relation.ParentFieldID].Old)
		if len(links) != 1 || links[0].ID != firstID || links[0].Label != "First" {
			t.Fatalf("unlink history = %+v", links)
		}
		requireHistoryAction(t, historyFor(t, f, owner, target.Table.ID, firstID), "unlink")
	})

	t.Run("self link", func(t *testing.T) {
		f := newFixture(t)
		owner := f.user("user", "active", "en")
		book := f.book(owner, f.workspace(owner).ID)
		table, label := relationTable(t, f, owner, book.ID, "People")
		created, err := f.service.CreateRelation(f.ctx, owner, table.Table.ID, RelationInput{
			Name: "Manager", ReverseName: "Reports", TargetTableID: table.Table.ID,
			Kind: "one_many", SourceSide: "child",
		})
		if err != nil {
			t.Fatal(err)
		}
		id := relationRow(t, f, owner, table.Table.ID, label.ID, "Self")
		if _, err := f.service.ChangeLinks(f.ctx, owner, table.Table.ID, id, created.Relation.ChildFieldID,
			LinkChange{Add: []string{id}}); err != nil {
			t.Fatal(err)
		}
		entries := historyFor(t, f, owner, table.Table.ID, id)
		linked := requireHistoryAction(t, entries, "link")
		if len(entries) != 2 || len(linked.Changes) != 2 ||
			len(historyLinks(t, linked.Changes[created.Relation.ParentFieldID].New)) != 1 ||
			len(historyLinks(t, linked.Changes[created.Relation.ChildFieldID].New)) != 1 {
			t.Fatalf("self link did not record both fields once: %+v", entries)
		}
	})

	t.Run("history failure rolls back a link", func(t *testing.T) {
		f := newFixture(t)
		owner := f.user("user", "active", "en")
		book := f.book(owner, f.workspace(owner).ID)
		source, sourceLabel := relationTable(t, f, owner, book.ID, "Tasks")
		target, targetLabel := relationTable(t, f, owner, book.ID, "Tags")
		created, err := f.service.CreateRelation(f.ctx, owner, source.Table.ID, RelationInput{
			Name: "Tags", ReverseName: "Tasks", TargetTableID: target.Table.ID, Kind: "many_many",
		})
		if err != nil {
			t.Fatal(err)
		}
		sourceID := relationRow(t, f, owner, source.Table.ID, sourceLabel.ID, "Task")
		targetID := relationRow(t, f, owner, target.Table.ID, targetLabel.ID, "Tag")
		rejectHistoryInserts(t, f, owner)
		_, err = f.service.ChangeLinks(f.ctx, owner, source.Table.ID, sourceID, created.Relation.ParentFieldID,
			LinkChange{Add: []string{targetID}})
		if err == nil {
			t.Fatal("history insertion failure did not fail link mutation")
		}
		row, err := f.service.GetRow(f.ctx, owner, source.Table.ID, sourceID)
		if err != nil || len(relationValues(t, row, created.Relation.ParentFieldID)) != 0 ||
			len(historyFor(t, f, owner, source.Table.ID, sourceID)) != 1 {
			t.Fatalf("failed history insert did not roll back link: %+v, %v", row, err)
		}
	})

	t.Run("one-to-one replacement", func(t *testing.T) {
		f := newFixture(t)
		owner := f.user("user", "active", "en")
		book := f.book(owner, f.workspace(owner).ID)
		parents, parentLabel := relationTable(t, f, owner, book.ID, "People")
		children, childLabel := relationTable(t, f, owner, book.ID, "Profiles")
		created, err := f.service.CreateRelation(f.ctx, owner, children.Table.ID, RelationInput{
			Name: "Person", ReverseName: "Profile", TargetTableID: parents.Table.ID,
			Kind: "one_one", SourceSide: "child",
		})
		if err != nil {
			t.Fatal(err)
		}
		parent := relationRow(t, f, owner, parents.Table.ID, parentLabel.ID, "Person")
		first := relationRow(t, f, owner, children.Table.ID, childLabel.ID, "Old")
		second := relationRow(t, f, owner, children.Table.ID, childLabel.ID, "New")
		if _, err := f.service.ChangeLinks(f.ctx, owner, children.Table.ID, first, created.Relation.ChildFieldID,
			LinkChange{Add: []string{parent}}); err != nil {
			t.Fatal(err)
		}
		if _, err := f.service.ChangeLinks(f.ctx, owner, children.Table.ID, second, created.Relation.ChildFieldID,
			LinkChange{Add: []string{parent}, Replace: true}); err != nil {
			t.Fatal(err)
		}
		old := requireHistoryAction(t, historyFor(t, f, owner, children.Table.ID, first), "unlink")
		if links := historyLinks(t, old.Changes[created.Relation.ChildFieldID].Old); len(links) != 1 || links[0].ID != parent {
			t.Fatalf("displaced row missing unlink: %+v", links)
		}
		parentHistory := historyFor(t, f, owner, parents.Table.ID, parent)
		unlink := requireHistoryAction(t, parentHistory, "unlink")
		if links := historyLinks(t, unlink.Changes[created.Relation.ParentFieldID].Old); len(links) != 1 || links[0].ID != first {
			t.Fatalf("parent missing displaced row: %+v", links)
		}
		requireHistoryAction(t, historyFor(t, f, owner, children.Table.ID, second), "link")
	})

	for _, policy := range []string{"set_null", "cascade", "restrict"} {
		t.Run("delete "+policy, func(t *testing.T) {
			f := newFixture(t)
			owner := f.user("user", "active", "en")
			book := f.book(owner, f.workspace(owner).ID)
			parents, parentLabel := relationTable(t, f, owner, book.ID, "Parents")
			children, childLabel := relationTable(t, f, owner, book.ID, "Children")
			created, err := f.service.CreateRelation(f.ctx, owner, children.Table.ID, RelationInput{
				Name: "Parent", ReverseName: "Children", TargetTableID: parents.Table.ID,
				Kind: "one_many", SourceSide: "child", OnDelete: policy,
			})
			if err != nil {
				t.Fatal(err)
			}
			parent := relationRow(t, f, owner, parents.Table.ID, parentLabel.ID, "Parent")
			child := relationRow(t, f, owner, children.Table.ID, childLabel.ID, "Child")
			if _, err := f.service.ChangeLinks(f.ctx, owner, children.Table.ID, child, created.Relation.ChildFieldID,
				LinkChange{Add: []string{parent}}); err != nil {
				t.Fatal(err)
			}
			_, err = f.service.DeleteRows(f.ctx, owner, parents.Table.ID, []string{parent})
			if policy == "restrict" {
				expectError(t, err, 409, "ERR_RESTRICT_VIOLATION")
				if got := len(historyFor(t, f, owner, parents.Table.ID, parent)); got != 2 {
					t.Fatalf("restricted delete recorded history: %d", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			parentHistory := historyFor(t, f, owner, parents.Table.ID, parent)
			deleted := requireHistoryAction(t, parentHistory, "delete")
			if string(deleted.Changes[parentLabel.ID].Old) != `"Parent"` ||
				len(historyLinks(t, deleted.Changes[created.Relation.ParentFieldID].Old)) != 1 {
				t.Fatalf("delete snapshot is incomplete: %+v", deleted.Changes)
			}
			childHistory := historyFor(t, f, owner, children.Table.ID, child)
			requireHistoryAction(t, childHistory, "unlink")
			if policy == "cascade" {
				requireHistoryAction(t, childHistory, "delete")
			}
		})
	}
}

func TestHistoryRetentionCutoffDefaultAndSettings(t *testing.T) {
	f := newFixture(t)
	owner := f.user("user", "active", "en")
	table := f.table(owner, f.book(owner, f.workspace(owner).ID).ID)
	var previous []byte
	err := f.pool.QueryRow(f.ctx, "SELECT value FROM meta.settings WHERE key='history_retention_days'").Scan(&previous)
	hadPrevious := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if hadPrevious {
			f.exec(`INSERT INTO meta.settings(key,value) VALUES ('history_retention_days',$1::jsonb)
				ON CONFLICT (key) DO UPDATE SET value=excluded.value`, previous)
		} else {
			f.exec("DELETE FROM meta.settings WHERE key='history_retention_days'")
		}
	})
	f.exec("DELETE FROM meta.settings WHERE key='history_retention_days'")
	now := time.Now().UTC().Truncate(time.Microsecond)
	cutoff := now.Add(-90 * 24 * time.Hour)
	for _, timestamp := range []time.Time{cutoff.Add(-time.Microsecond), cutoff, cutoff.Add(time.Microsecond)} {
		f.exec(`INSERT INTO meta.row_history(table_id,row_id,user_id,action,changes,created_at)
			VALUES ($1,1,$2,'update','{}',$3)`, table.Table.ID, owner.ID, timestamp)
	}
	count, err := f.service.purgeRowHistory(f.ctx, now)
	if err != nil || count != 1 {
		t.Fatalf("default retention removed %d rows: %v", count, err)
	}
	if got := scanOne[int](f, "SELECT count(*) FROM meta.row_history WHERE table_id=$1", table.Table.ID); got != 2 {
		t.Fatalf("exact cutoff should be retained, got %d rows", got)
	}
	f.exec(`INSERT INTO meta.settings(key,value) VALUES ('history_retention_days','1'::jsonb)`)
	count, err = f.service.purgeRowHistory(f.ctx, now)
	if err != nil || count != 2 {
		t.Fatalf("updated retention removed %d rows: %v", count, err)
	}
	f.exec(`UPDATE meta.settings SET value='null'::jsonb WHERE key='history_retention_days'`)
	if _, err := f.service.purgeRowHistory(f.ctx, now); err == nil {
		t.Fatal("invalid retention setting was silently accepted")
	}
	f.exec(`UPDATE meta.settings SET value='0'::jsonb WHERE key='history_retention_days'`)
	if _, err := f.service.purgeRowHistory(f.ctx, now); err == nil {
		t.Fatal("out-of-range retention setting was silently accepted")
	}
}

func TestHistoryRetentionWorkerRunsImmediatelyAndStops(t *testing.T) {
	f := newFixture(t)
	owner := f.user("user", "active", "en")
	table := f.table(owner, f.book(owner, f.workspace(owner).ID).ID)
	f.exec(`INSERT INTO meta.row_history(table_id,row_id,user_id,action,changes,created_at)
		VALUES ($1,1,$2,'update','{}',now() - interval '3651 days')`, table.Table.ID, owner.ID)
	ctx, cancel := context.WithCancel(f.ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		f.service.RunHistoryRetentionWorker(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)))
	}()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("history retention worker did not stop")
		}
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if scanOne[int](f, "SELECT count(*) FROM meta.row_history WHERE table_id=$1", table.Table.ID) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("history retention worker did not run at startup")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestHistoryReadsRequireTableRead(t *testing.T) {
	f := newFixture(t)
	owner, reader, outsider := f.user("user", "active", "en"), f.user("user", "active", "en"),
		f.user("admin", "active", "en")
	table := f.table(owner, f.book(owner, f.workspace(owner).ID).ID)
	rows, err := f.service.CreateRows(f.ctx, owner, table.Table.ID, []RowInsert{{Fields: map[string]json.RawMessage{}}})
	if err != nil {
		t.Fatal(err)
	}
	rowID := rows[0].ID
	f.grant(acl.Table, table.Table.ID, "user", reader.ID, acl.Read)
	entries := historyFor(t, f, reader, table.Table.ID, rowID)
	if len(entries) != 1 || entries[0].AuthorID == nil || *entries[0].AuthorID != owner.ID || entries[0].AuthorName == "" {
		t.Fatalf("reader history = %+v", entries)
	}
	_, err = f.service.ListRowHistory(f.ctx, outsider, table.Table.ID, rowID, 0)
	expectError(t, err, 404, "ERR_NOT_FOUND")
	_, err = f.service.ListRowHistory(f.ctx, reader, table.Table.ID, rowID, -1)
	expectError(t, err, 400, "ERR_VALIDATION")
	_, err = f.service.ListRowHistory(f.ctx, reader, table.Table.ID, "01", 0)
	expectError(t, err, 404, "ERR_NOT_FOUND")
	f.exec("DELETE FROM meta.grants WHERE resource_type='table' AND resource_id=$1 AND principal_id=$2", table.Table.ID, reader.ID)
	f.resolver.Invalidate()
	_, err = f.service.ListRowHistory(f.ctx, reader, table.Table.ID, rowID, 0)
	expectError(t, err, 404, "ERR_NOT_FOUND")
}
