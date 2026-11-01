package httpapi

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strconv"
	"testing"

	"pandatable/internal/acl"
)

func TestExportBatchesGroupingAndRevokedAccess(t *testing.T) {
	f := newACLFixture(t)
	owner, reader, stranger := f.user("user", false), f.user("user", false), f.user("user", false)
	book := f.newBook(owner, f.newWorkspace(owner, "Large export").ID, "Book")
	table := f.newTable(owner, book.ID, "Batches")
	name := relationLabelField(table.TableSchema)
	group := f.newField(owner, table.Table.ID, `{"name":"Group","uiType":"integer"}`)
	grant := f.grantOK(owner, acl.Table, table.Table.ID, "user", reader.id, acl.Read)

	values := make([]any, 205)
	groups := []int{10, 2, 1}
	for index := range values {
		values[index] = map[string]any{"fields": map[string]any{
			name.ID: fmt.Sprintf("Row-%03d", index), group.ID: groups[index%len(groups)],
		}}
	}
	body, err := json.Marshal(map[string]any{"rows": values})
	if err != nil {
		t.Fatal(err)
	}
	decode[rowsResponse](f, f.api(owner, "POST", "/tables/"+table.Table.ID+"/rows", string(body)), 201)
	view := table.Views[0].ID
	ticket := prepareExportForTest(f, reader, view, map[string]any{
		"format": "csv", "fieldIds": []string{name.ID, group.ID},
		"groupBy": []string{group.ID}, "sorts": []any{map[string]any{"fieldId": name.ID, "direction": "desc"}},
	})
	f.expectStatus(f.api(stranger, "GET", "/exports/"+ticket, ""), 404, "ERR_NOT_FOUND")
	response := f.api(reader, "GET", "/exports/"+ticket, "")
	f.expectStatus(response, 200, "")
	records, err := csv.NewReader(bytes.NewReader(response.Body.Bytes()[3:])).ReadAll()
	if err != nil || len(records) != 206 {
		t.Fatalf("batched CSV = %d records, %v", len(records), err)
	}
	seen := make(map[string]bool)
	previousGroup, previousName := 0, ""
	for _, record := range records[1:] {
		key, err := strconv.Atoi(record[1])
		if err != nil || key < previousGroup || (key == previousGroup && record[0] >= previousName) || seen[record[0]] {
			t.Fatalf("grouped export order or duplicate: %v after %d/%s", record, previousGroup, previousName)
		}
		seen[record[0]], previousGroup, previousName = true, key, record[0]
	}
	if len(seen) != len(values) {
		t.Fatalf("exported %d unique rows, want %d", len(seen), len(values))
	}

	ticket = prepareExportForTest(f, reader, view, map[string]any{"format": "xlsx"})
	f.expectStatus(f.api(owner, "DELETE", "/grants/"+grant.Grant.ID, ""), 200, "")
	f.expectStatus(f.api(reader, "GET", "/exports/"+ticket, ""), 404, "ERR_NOT_FOUND")
}
