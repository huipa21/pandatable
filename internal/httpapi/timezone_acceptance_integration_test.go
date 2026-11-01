package httpapi

import (
	"encoding/json"
	"sort"
	"testing"
	"time"

	"pandatable/internal/acl"
	"pandatable/internal/structure"
)

func TestTodayFilterUsesEachUsersTimezoneAT117(t *testing.T) {
	f := newACLFixture(t)
	alice, bob := f.user("user", false), f.user("user", false)
	workspace := f.newWorkspace(alice, "Timezone acceptance")
	book := f.newBook(alice, workspace.ID, "Book")
	table := f.newTable(alice, book.ID, "Dates")
	f.grantOK(alice, acl.Table, table.Table.ID, "user", bob.id, acl.Read)
	date := f.newField(alice, table.Table.ID, `{"name":"Date","uiType":"date"}`)

	users := []struct {
		user     *aclUser
		timezone string
		location *time.Location
	}{
		{user: alice, timezone: "Pacific/Kiritimati"},
		{user: bob, timezone: "Pacific/Pago_Pago"},
	}
	for index := range users {
		location, err := time.LoadLocation(users[index].timezone)
		if err != nil {
			t.Fatal(err)
		}
		users[index].location = location
		body, err := json.Marshal(map[string]string{"timezone": users[index].timezone})
		if err != nil {
			t.Fatal(err)
		}
		f.expectStatus(f.api(users[index].user, "PATCH", "/auth/me", string(body)), 200, "")
	}

	now := time.Now()
	if now.In(users[0].location).Format("2006-01-02") == now.In(users[1].location).Format("2006-01-02") {
		t.Fatal("the test timezones must be on different calendar dates")
	}
	// Seed adjoining days so a query crossing midnight still has an unambiguous result.
	days := map[string]bool{}
	for _, user := range users {
		for offset := -1; offset <= 1; offset++ {
			days[now.In(user.location).AddDate(0, 0, offset).Format("2006-01-02")] = true
		}
	}
	orderedDays := make([]string, 0, len(days))
	for day := range days {
		orderedDays = append(orderedDays, day)
	}
	sort.Strings(orderedDays)
	rows := make([]map[string]any, 0, len(orderedDays))
	for _, day := range orderedDays {
		rows = append(rows, map[string]any{"fields": map[string]string{date.ID: day}})
	}

	body, err := json.Marshal(map[string]any{"rows": rows})
	if err != nil {
		t.Fatal(err)
	}
	created := decode[rowsResponse](f, f.api(alice, "POST", "/tables/"+table.Table.ID+"/rows", string(body)), 201)
	if len(created.Rows) != len(orderedDays) {
		t.Fatalf("created rows = %+v", created.Rows)
	}
	rowByDay := make(map[string]string, len(orderedDays))
	for index, day := range orderedDays {
		rowByDay[day] = created.Rows[index].ID
	}
	body, err = json.Marshal(map[string]any{
		"filter": map[string]any{
			"field": date.ID, "cmp": "is", "value": map[string]string{"rel": "today"},
		},
		"includeCount": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	matchedRows := make(map[string]bool)
	for _, user := range users {
		before := time.Now().In(user.location).Format("2006-01-02")
		result := decode[structure.RowQueryResult](f, f.api(user.user, "POST", "/views/"+table.Views[0].ID+"/query", string(body)), 200)
		after := time.Now().In(user.location).Format("2006-01-02")
		beforeID, beforeSeeded := rowByDay[before]
		afterID, afterSeeded := rowByDay[after]
		if !beforeSeeded || !afterSeeded || result.Total == nil || *result.Total != 1 || len(result.Rows) != 1 ||
			(result.Rows[0].ID != beforeID && result.Rows[0].ID != afterID) {
			t.Fatalf("today filter for %s = %+v", user.timezone, result)
		}
		matchedRows[result.Rows[0].ID] = true
	}
	if len(matchedRows) != len(users) {
		t.Fatal("users in different calendar dates matched the same row")
	}
}
