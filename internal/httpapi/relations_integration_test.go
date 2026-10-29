package httpapi

import (
	"fmt"
	"testing"

	"pandatable/internal/acl"
	"pandatable/internal/structure"
)

type relationBody struct {
	structure.RelationResult
	EventID string `json:"eventId"`
}

func relationLabelField(table structure.TableSchema) structure.Field {
	for _, field := range table.Fields {
		if !field.IsSystem && field.UIType == "text" {
			return field
		}
	}
	return structure.Field{}
}

func TestRelationHTTPAPICreateLinkAndEndpointACL(t *testing.T) {
	f := newACLFixture(t)
	owner, writer := f.user("user", false), f.user("user", false)
	workspace := f.newWorkspace(owner, "Relations API")
	book := f.newBook(owner, workspace.ID, "Book")
	projects := f.newTable(owner, book.ID, "Projects")
	tasks := f.newTable(owner, book.ID, "Tasks")
	projectLabel := relationLabelField(projects.TableSchema)
	taskLabel := relationLabelField(tasks.TableSchema)
	if projectLabel.ID == "" || taskLabel.ID == "" {
		t.Fatal("new tables are missing their text label fields")
	}
	f.grantOK(owner, acl.Table, tasks.Table.ID, "user", writer.id, acl.Owner)
	f.grantOK(owner, acl.Table, projects.Table.ID, "user", writer.id, acl.Read)
	f.expectProbe(writer, acl.Table, projects.Table.ID, acl.Write, 403)

	f.expectStatus(f.api(writer, "POST", "/tables/"+tasks.Table.ID+"/relations",
		fmt.Sprintf(`{"name":"Project","reverseName":"Tasks","targetTableId":%q,"kind":"one_many","sourceSide":"child"}`,
			projects.Table.ID)), 403, "ERR_PERMISSION_DENIED")

	created := decode[relationBody](f, f.api(owner, "POST", "/tables/"+tasks.Table.ID+"/relations",
		fmt.Sprintf(`{"name":"Project","reverseName":"Tasks","targetTableId":%q,"kind":"one_many","sourceSide":"child"}`,
			projects.Table.ID)), 201)
	if created.EventID == "" || created.Relation.Kind != "one_many" ||
		created.Field == nil || created.Field.UIType != "link" {
		t.Fatalf("relation response = %+v", created)
	}
	if len(created.Schema.Relations) != 1 {
		t.Fatalf("created schema relations = %+v", created.Schema.Relations)
	}

	projectRows := decode[rowsResponse](f, f.api(owner, "POST", "/tables/"+projects.Table.ID+"/rows",
		fmt.Sprintf(`{"rows":[{"fields":{%q:"Project A"}}]}`, projectLabel.ID)), 201)
	taskRows := decode[rowsResponse](f, f.api(owner, "POST", "/tables/"+tasks.Table.ID+"/rows",
		fmt.Sprintf(`{"rows":[{"fields":{%q:"Task A"}}]}`, taskLabel.ID)), 201)
	taskID, projectID := taskRows.Rows[0].ID, projectRows.Rows[0].ID

	f.expectStatus(f.api(writer, "POST", "/tables/"+tasks.Table.ID+"/rows/"+taskID+
		"/links/"+created.Relation.ChildFieldID,
		fmt.Sprintf(`{"add":[%q]}`, projectID)), 403, "ERR_PERMISSION_DENIED")

	f.grantOK(owner, acl.Table, projects.Table.ID, "user", writer.id, acl.Write)
	changed := decode[rowResponse](f, f.api(writer, "POST", "/tables/"+tasks.Table.ID+"/rows/"+taskID+
		"/links/"+created.Relation.ChildFieldID, fmt.Sprintf(`{"add":[%q]}`, projectID)), 200)
	if changed.EventID == "" || changed.Row.ID != taskID {
		t.Fatalf("link response = %+v", changed)
	}
	if got := string(changed.Row.Fields[created.Relation.ChildFieldID]); got !=
		fmt.Sprintf(`[{"id":%q,"label":"Project A"}]`, projectID) {
		t.Fatalf("linked row value = %s", got)
	}

	reverse := decode[structure.RowRecord](f, f.api(owner, "GET",
		"/tables/"+projects.Table.ID+"/rows/"+projectID, ""), 200)
	if got := string(reverse.Fields[created.Relation.ParentFieldID]); got !=
		fmt.Sprintf(`[{"id":%q,"label":"Task A"}]`, taskID) {
		t.Fatalf("reverse row value = %s", got)
	}

	f.expectStatus(f.api(owner, "POST", "/tables/"+tasks.Table.ID+"/rows/"+taskID+
		"/links/not-a-field", fmt.Sprintf(`{"add":[%q]}`, projectID)), 400, "ERR_VALIDATION")
}
