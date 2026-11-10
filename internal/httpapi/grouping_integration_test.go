package httpapi

import (
	"encoding/json"
	"fmt"
	"testing"

	"pandatable/internal/acl"
	"pandatable/internal/structure"
)

func TestGroupedRowsHTTPAPIUsesFilteredBucketsAndGroupPaths(t *testing.T) {
	f := newACLFixture(t)
	owner, reader, stranger := f.user("user", false), f.user("user", false), f.user("user", false)
	workspace := f.newWorkspace(owner, "Grouping")
	book := f.newBook(owner, workspace.ID, "Book")
	table := f.newTable(owner, book.ID, "Tasks")
	f.grantOK(owner, acl.Table, table.Table.ID, "user", reader.id, acl.Read)
	project := f.newField(owner, table.Table.ID, `{"name":"Project","uiType":"text"}`)
	status := f.newField(owner, table.Table.ID, `{"name":"Status","uiType":"text"}`)
	assignee := f.newField(owner, table.Table.ID, `{"name":"Owner","uiType":"text"}`)
	hours := f.newField(owner, table.Table.ID, `{"name":"Hours","uiType":"integer"}`)

	type row struct {
		project, status string
		owner           *string
		hours           int
	}
	alice, bob := "Alice", "Bob"
	data := []row{
		{"North", "Open", &alice, 4},
		{"North", "Open", &alice, 5},
		{"North", "Open", nil, 3},
		{"North", "Blocked", &bob, 7},
		{"South", "Open", &bob, 2},
		{"South", "Open", &bob, 3},
	}
	inserts := make([]map[string]any, len(data))
	for index, item := range data {
		inserts[index] = map[string]any{"fields": map[string]any{
			project.ID: item.project, status.ID: item.status,
			assignee.ID: item.owner, hours.ID: item.hours,
		}}
	}
	body, err := json.Marshal(map[string]any{"rows": inserts})
	if err != nil {
		t.Fatal(err)
	}
	f.expectStatus(f.api(owner, "POST", "/tables/"+table.Table.ID+"/rows", string(body)), 201, "")
	config := fmt.Sprintf(
		`{"config":{"fields":[],"frozen":0,"groupBy":[%q,%q,%q],"aggregates":[{"fieldId":%q,"aggregate":"sum"},{"fieldId":%q,"aggregate":"count"},{"fieldId":%q,"aggregate":"count_empty"},{"fieldId":%q,"aggregate":"count_filled"}]}}`,
		project.ID, status.ID, assignee.ID, hours.ID, assignee.ID, assignee.ID, assignee.ID,
	)
	updated := decode[viewResponse](f, f.fieldCall(owner, "PATCH", "/views/"+table.Views[0].ID, config, `"1"`), 200)
	if updated.View.Config.GroupBy == nil || len(updated.View.Config.GroupBy) != 3 ||
		len(updated.View.Config.Aggregates) != 4 {
		t.Fatalf("group config was not retained: %+v", updated.View.Config)
	}

	filter := fmt.Sprintf(`{"field":%q,"cmp":"is","value":"Open"}`, status.ID)
	root := decode[structure.RowGroupResult](f, f.api(owner, "POST",
		"/views/"+table.Views[0].ID+"/groups", `{"path":[],"filter":`+filter+`}`), 200)
	if root.SchemaVersion != table.SchemaVersion+4 || len(root.Groups) != 2 {
		t.Fatalf("filtered root groups = %+v", root)
	}
	north := groupedBucket(t, root.Groups, `"North"`)
	if north.Count != 3 || string(north.Aggregates[0].Value) != "12" ||
		string(north.Aggregates[1].Value) != "3" ||
		string(north.Aggregates[2].Value) != "1" ||
		string(north.Aggregates[3].Value) != "2" {
		t.Fatalf("filtered North group = %+v", north)
	}
	northStatuses := decode[structure.RowGroupResult](f, f.api(owner, "POST",
		"/views/"+table.Views[0].ID+"/groups", fmt.Sprintf(`{"path":["North"],"filter":%s}`, filter)), 200)
	if len(northStatuses.Groups) != 1 || string(northStatuses.Groups[0].Key) != `"Open"` ||
		northStatuses.Groups[0].Count != 3 ||
		string(northStatuses.Groups[0].Aggregates[0].Value) != "12" {
		t.Fatalf("filtered nested groups = %+v", northStatuses)
	}
	owners := decode[structure.RowGroupResult](f, f.api(owner, "POST",
		"/views/"+table.Views[0].ID+"/groups", fmt.Sprintf(`{"path":["North","Open"],"filter":%s}`, filter)), 200)
	if len(owners.Groups) != 2 {
		t.Fatalf("owner groups = %+v", owners)
	}
	aliceGroup := groupedBucket(t, owners.Groups, `"Alice"`)
	emptyGroup := groupedBucket(t, owners.Groups, `null`)
	if aliceGroup.Count != 2 || string(aliceGroup.Aggregates[0].Value) != "9" ||
		emptyGroup.Count != 1 || string(emptyGroup.Aggregates[0].Value) != "3" {
		t.Fatalf("owner buckets = %+v", owners.Groups)
	}
	rows := decode[structure.RowQueryResult](f, f.api(owner, "POST",
		"/views/"+table.Views[0].ID+"/query",
		fmt.Sprintf(`{"filter":%s,"groupPath":["North","Open",null]}`, filter)), 200)
	if len(rows.Rows) != 1 || rows.Total == nil || *rows.Total != 1 ||
		string(rows.Rows[0].Fields[hours.ID]) != "3" {
		t.Fatalf("expanded empty-owner rows = %+v", rows)
	}
	f.expectStatus(f.api(owner, "POST", "/views/"+table.Views[0].ID+"/groups",
		`{"path":["North","Open","Alice","extra"]}`), 400, "ERR_VALIDATION")
	f.expectStatus(f.api(reader, "POST", "/views/"+table.Views[0].ID+"/groups",
		`{"path":[]}`), 200, "")
	localGroups := decode[structure.RowGroupResult](f, f.api(reader, "POST",
		"/views/"+table.Views[0].ID+"/groups",
		fmt.Sprintf(`{"path":[],"groupBy":[%q]}`, status.ID)), 200)
	if len(localGroups.Groups) != 2 || groupedBucket(t, localGroups.Groups, `"Open"`).Count != 5 ||
		groupedBucket(t, localGroups.Groups, `"Blocked"`).Count != 1 {
		t.Fatalf("reader's transient groups = %+v", localGroups)
	}
	localRows := decode[structure.RowQueryResult](f, f.api(reader, "POST",
		"/views/"+table.Views[0].ID+"/query",
		fmt.Sprintf(`{"groupPath":["Blocked"],"groupBy":[%q]}`, status.ID)), 200)
	if len(localRows.Rows) != 1 || string(localRows.Rows[0].Fields[hours.ID]) != "7" {
		t.Fatalf("reader's transient group rows = %+v", localRows)
	}
	f.expectStatus(f.api(reader, "POST", "/views/"+table.Views[0].ID+"/groups",
		fmt.Sprintf(`{"path":[],"groupBy":[%q,%q]}`, status.ID, status.ID)), 400, "ERR_VALIDATION")
	savedGroups := decode[structure.RowGroupResult](f, f.api(reader, "POST",
		"/views/"+table.Views[0].ID+"/groups", `{"path":[]}`), 200)
	if len(savedGroups.Groups) != 2 || groupedBucket(t, savedGroups.Groups, `"North"`).Count != 4 {
		t.Fatalf("saved grouping changed by reader = %+v", savedGroups)
	}
	f.expectStatus(f.api(stranger, "POST", "/views/"+table.Views[0].ID+"/groups",
		`{"path":[]}`), 404, "ERR_NOT_FOUND")

	multi := f.newField(owner, table.Table.ID, `{"name":"Labels","uiType":"multi_select","options":{"choices":[{"id":"a","label":"A"},{"id":"b","label":"B"}]}}`)
	multiRows := fmt.Sprintf(`{"rows":[{"fields":{%q:["B","A"]}},{"fields":{%q:["A","B"]}},{"fields":{%q:[]}}]}`,
		multi.ID, multi.ID, multi.ID)
	f.expectStatus(f.api(owner, "POST", "/tables/"+table.Table.ID+"/rows", multiRows), 201, "")
	viewConfig, err := json.Marshal(map[string]any{
		"fields": []any{}, "frozen": 0, "groupBy": []string{multi.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	f.exec("UPDATE meta.views SET config=$1::jsonb WHERE id=$2", viewConfig, table.Views[0].ID)
	multiGroups := decode[structure.RowGroupResult](f, f.api(owner, "POST",
		"/views/"+table.Views[0].ID+"/groups", `{"path":[]}`), 200)
	if len(multiGroups.Groups) != 2 ||
		string(groupedBucket(t, multiGroups.Groups, `["A","B"]`).Key) != `["A","B"]` ||
		groupedBucket(t, multiGroups.Groups, `["A","B"]`).Count != 2 {
		t.Fatalf("multi-select buckets = %+v", multiGroups.Groups)
	}
}

func groupedBucket(t *testing.T, groups []structure.RowGroupBucket, key string) structure.RowGroupBucket {
	t.Helper()
	for _, group := range groups {
		if string(group.Key) == key {
			return group
		}
	}
	t.Fatalf("group key %s not found in %+v", key, groups)
	return structure.RowGroupBucket{}
}
