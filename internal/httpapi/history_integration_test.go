package httpapi

import (
	"fmt"
	"testing"

	"pandatable/internal/acl"
	"pandatable/internal/structure"
)

func TestHistoryAPIReadPermissionsPaginationAndDeletedRows(t *testing.T) {
	f := newACLFixture(t)
	owner, reader, writer, outsider := f.user("user", false), f.user("user", false),
		f.user("user", false), f.user("admin", false)
	workspace := f.newWorkspace(owner, "History")
	book := f.newBook(owner, workspace.ID, "Book")
	table := f.newTable(owner, book.ID, "Rows")
	readGrant := f.grantOK(owner, acl.Table, table.Table.ID, "user", reader.id, acl.Read)
	f.grantOK(owner, acl.Table, table.Table.ID, "user", writer.id, acl.Write)
	row := decode[rowsResponse](f, f.api(owner, "POST", "/tables/"+table.Table.ID+"/rows",
		`{"rows":[{"fields":{}}]}`), 201).Rows[0]
	path := "/tables/" + table.Table.ID + "/rows/" + row.ID + "/history"
	f.exec(`INSERT INTO meta.row_history(table_id,row_id,user_id,action,changes)
		SELECT $1,$2::bigint,$3,'update','{}'::jsonb FROM generate_series(1,55)`,
		table.Table.ID, row.ID, writer.id)
	f.exec(`INSERT INTO meta.row_history(table_id,user_id,action,changes)
		VALUES ($1,$2,'type_change','{}')`, table.Table.ID, owner.id)

	first := decode[structure.HistoryListing](f, f.api(reader, "GET", path, ""), 200)
	if len(first.Items) != 50 || !first.HasMore || first.Items[0].AuthorID == nil ||
		*first.Items[0].AuthorID != writer.id || first.Items[0].AuthorName == "" {
		t.Fatalf("first history page = %+v", first)
	}
	second := decode[structure.HistoryListing](f, f.api(reader, "GET", path+"?offset=50", ""), 200)
	if len(second.Items) != 6 || second.HasMore {
		t.Fatalf("second history page = %+v", second)
	}
	seen := make(map[string]bool)
	for _, entry := range append(first.Items, second.Items...) {
		if seen[entry.ID] || entry.Action == "type_change" {
			t.Fatalf("duplicated or table-level history in row page: %+v", entry)
		}
		seen[entry.ID] = true
	}
	f.expectStatus(f.api(outsider, "GET", path, ""), 404, "ERR_NOT_FOUND")
	for _, query := range []string{"?offset=-1", "?offset=1000001", "?offset=x", "?offset=1&offset=2", "?limit=1"} {
		f.expectStatus(f.api(reader, "GET", path+query, ""), 400, "ERR_VALIDATION")
	}
	f.expectStatus(f.api(reader, "GET", "/tables/"+table.Table.ID+"/rows/01/history", ""), 404, "ERR_NOT_FOUND")
	f.expectStatus(f.api(reader, "PATCH", "/tables/"+table.Table.ID+"/rows/"+row.ID,
		fmt.Sprintf(`{"fields":{%q:"forbidden"}}`, *table.Table.DisplayFieldID)), 403, "ERR_PERMISSION_DENIED")

	f.exec("DELETE FROM meta.users WHERE id=$1", writer.id)
	afterDeletion := decode[structure.HistoryListing](f, f.api(reader, "GET", path, ""), 200)
	if afterDeletion.Items[0].AuthorID != nil || afterDeletion.Items[0].AuthorName != "" {
		t.Fatalf("deleted author was not anonymized: %+v", afterDeletion.Items[0])
	}
	f.expectStatus(f.api(owner, "DELETE", "/tables/"+table.Table.ID+"/rows",
		fmt.Sprintf(`{"ids":[%q]}`, row.ID)), 200, "")
	afterRowDelete := decode[structure.HistoryListing](f, f.api(reader, "GET", path, ""), 200)
	if len(afterRowDelete.Items) != 50 || afterRowDelete.Items[0].Action != "delete" {
		t.Fatalf("deleted row lost its history: %+v", afterRowDelete)
	}
	f.expectStatus(f.api(owner, "DELETE", "/grants/"+readGrant.Grant.ID, ""), 200, "")
	f.expectStatus(f.api(reader, "GET", path, ""), 404, "ERR_NOT_FOUND")
	f.exec("DELETE FROM meta.tables WHERE id=$1", table.Table.ID)
	if got := f.scan("SELECT count(*)::text FROM meta.row_history WHERE table_id=$1", table.Table.ID); got != "0" {
		t.Fatalf("table purge retained %s history rows", got)
	}
}
