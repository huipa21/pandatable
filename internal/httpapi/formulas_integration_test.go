package httpapi

import (
	"encoding/json"
	"testing"

	"pandatable/internal/acl"
)

func TestFormulaCheckAPIRequiresOwnerAndReportsPosition(t *testing.T) {
	f := newACLFixture(t)
	owner, reader := f.user("user", false), f.user("user", false)
	table := f.newTable(owner, f.newBook(owner, f.newWorkspace(owner, "Formulas").ID, "Book").ID, "Table")
	f.grantOK(owner, acl.Table, table.Table.ID, "user", reader.id, acl.Read)

	valid := f.api(owner, "POST", "/formula:check",
		`{"tableId":"`+table.Table.ID+`","expr":"CONCAT({Name},\"!\")"}`)
	if valid.Code != 200 {
		t.Fatalf("valid formula status = %d: %s", valid.Code, valid.Body.String())
	}
	var checked struct {
		Type  string `json:"type"`
		Error *struct {
			Position int `json:"position"`
		} `json:"error"`
	}
	if err := json.Unmarshal(valid.Body.Bytes(), &checked); err != nil {
		t.Fatal(err)
	}
	if checked.Type != "text" || checked.Error != nil {
		t.Fatalf("valid formula response = %+v", checked)
	}

	invalid := f.api(owner, "POST", "/formula:check",
		`{"tableId":"`+table.Table.ID+`","expr":"{Name} * 2"}`)
	if invalid.Code != 200 {
		t.Fatalf("invalid formula status = %d: %s", invalid.Code, invalid.Body.String())
	}
	if err := json.Unmarshal(invalid.Body.Bytes(), &checked); err != nil {
		t.Fatal(err)
	}
	if checked.Error == nil || checked.Error.Position == 0 {
		t.Fatalf("invalid formula response = %+v", checked)
	}
	f.expectStatus(f.api(reader, "POST", "/formula:check",
		`{"tableId":"`+table.Table.ID+`","expr":"{Name}"}`), 403, "ERR_PERMISSION_DENIED")
}
