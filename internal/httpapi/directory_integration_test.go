package httpapi

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"pandatable/internal/acl"
)

func TestShareDirectoryOwnerPrivacyAndBounds(t *testing.T) {
	f := newACLFixture(t)
	owner, bookOwner, outsider, forced := f.user("user", false), f.user("user", false), f.user("admin", false), f.user("user", true)
	x := f.site()
	f.seedOwner(x.w, owner)
	f.grantOK(owner, acl.Book, x.b1, "user", bookOwner.id, acl.Owner)
	base := "/directory/users?resource=book:" + x.b1 + "&q="
	response := f.api(bookOwner, "GET", base+url.QueryEscape(bookOwner.login), "")
	f.expectStatus(response, 200, "")
	var listing struct {
		Items   []map[string]any `json:"items"`
		HasMore bool             `json:"hasMore"`
	}

	if err := json.Unmarshal(response.Body.Bytes(), &listing); err != nil {
		t.Fatal(err)
	}
	if len(listing.Items) != 1 || len(listing.Items[0]) != 3 || listing.Items[0]["id"] != bookOwner.id ||
		listing.Items[0]["username"] != bookOwner.login || listing.HasMore {
		t.Fatalf("minimal identity contract: %s", response.Body)
	}
	for _, forbidden := range []string{"email", "role", "status", "password", "member", "avatar", "total"} {
		if strings.Contains(response.Body.String(), `"`+forbidden+`"`) {
			t.Fatalf("directory disclosed %s", forbidden)
		}
	}
	f.expectStatus(f.api(nil, "GET", base+"ab", ""), 401, "ERR_UNAUTHENTICATED")
	f.expectStatus(f.api(forced, "GET", base+"ab", ""), 403, "ERR_PASSWORD_CHANGE_REQUIRED")
	f.expectStatus(f.api(outsider, "GET", base+"ab", ""), 404, "ERR_NOT_FOUND")
	f.expectStatus(f.api(bookOwner, "GET", "/directory/users?resource=workspace:"+x.w+"&q=ab", ""), 403, "ERR_PERMISSION_DENIED")
	f.expectStatus(f.api(bookOwner, "GET", "/directory/groups?resource=book:"+x.b2+"&q=ab", ""), 404, "ERR_NOT_FOUND")
	for _, path := range []string{
		"/directory/users", base, base + "a", base + "ab&q=cd", base + "ab&offset=0",
		base + "ab&resource=book:" + x.b1, "/directory/users?resource=bad&q=ab",
		base + url.QueryEscape(strings.Repeat("a", 101)), base + url.QueryEscape(strings.Repeat("界", 85)),
		base + url.QueryEscape("ab\tcd"), base + "%ZZ",
	} {
		f.expectStatus(f.api(owner, "GET", path, ""), 400, "ERR_VALIDATION")
	}
	f.expectStatus(f.api(owner, "GET", base+url.QueryEscape("  "+strings.ToUpper(bookOwner.login)+"  "), ""), 200, "")
	f.expectStatus(f.api(owner, "GET", base+url.QueryEscape(strings.Repeat("界", 84)), ""), 200, "")

	prefix := owner.login
	for i := range 22 {
		id := f.scan(`INSERT INTO meta.users (username,display_name,password_hash,avatar_color,status,role)
			VALUES ($1,$2,$3,'blue',$4,'user') RETURNING id::text`,
			fmt.Sprintf("%s-%02d", prefix, i), "Picker only", f.phc,
			map[bool]string{true: "disabled", false: "active"}[i == 21])
		f.users = append(f.users, id)
	}
	response = f.api(owner, "GET", base+prefix, "")
	f.expectStatus(response, 200, "")
	if err := json.Unmarshal(response.Body.Bytes(), &listing); err != nil {
		t.Fatal(err)
	}
	if len(listing.Items) != 20 || !listing.HasMore || listing.Items[0]["username"] != prefix ||
		listing.Items[19]["username"] != prefix+"-18" {
		t.Fatalf("bounded ordered search: %s", response.Body)
	}
	disabled := f.api(owner, "GET", base+prefix+"-21", "")
	f.expectStatus(disabled, 200, "")
	if disabled.Body.String() != "{\"items\":[],\"hasMore\":false}\n" {
		t.Fatalf("disabled user exposed: %s", disabled.Body)
	}
	group := f.scan("INSERT INTO meta.groups(name) VALUES ($1) RETURNING id::text", prefix+" %_ 团队")
	f.groups = append(f.groups, group)
	groupPath := "/directory/groups?resource=table:" + x.t1 + "&q="
	response = f.api(bookOwner, "GET", groupPath+url.QueryEscape("%_"), "")
	f.expectStatus(response, 200, "")
	if err := json.Unmarshal(response.Body.Bytes(), &listing); err != nil {
		t.Fatal(err)
	}
	if len(listing.Items) != 1 || len(listing.Items[0]) != 3 || listing.Items[0]["id"] != group || listing.Items[0]["username"] != nil {
		t.Fatalf("literal minimal group search: %s", response.Body)
	}
	for _, q := range []string{"%_", "' OR true --"} {
		f.expectStatus(f.api(owner, "GET", base+url.QueryEscape(q), ""), 200, "")
	}
	for i := range 22 {
		id := f.scan("INSERT INTO meta.groups(name) VALUES ($1) RETURNING id::text", fmt.Sprintf("%s-%02d", prefix, i))
		f.groups = append(f.groups, id)
	}
	response = f.api(owner, "GET", groupPath+prefix, "")
	f.expectStatus(response, 200, "")
	if err := json.Unmarshal(response.Body.Bytes(), &listing); err != nil {
		t.Fatal(err)
	}
	if len(listing.Items) != 20 || !listing.HasMore {
		t.Fatalf("bounded group search: %s", response.Body)
	}
	if again := f.api(owner, "GET", groupPath+prefix, ""); again.Body.String() != response.Body.String() {
		t.Fatal("group search order is not deterministic")
	}
	f.exec("UPDATE meta.users SET status='disabled' WHERE id=$1", outsider.id)
	f.expectStatus(f.api(outsider, "GET", base+"ab", ""), 401, "ERR_UNAUTHENTICATED")

	// Prime the resolver, then revoke via the real grant API: directory must not
	// trust the cache or retain the owner's former scope.
	f.expectProbe(bookOwner, acl.Book, x.b1, acl.Owner, 200)
	grant := f.grantOK(owner, acl.Book, x.b1, "user", bookOwner.id, acl.Read)
	f.expectStatus(f.api(bookOwner, "GET", base+"ab", ""), 403, "ERR_PERMISSION_DENIED")
	f.expectStatus(f.api(owner, "DELETE", "/grants/"+grant.Grant.ID, ""), 200, "")
	f.expectStatus(f.api(bookOwner, "GET", base+"ab", ""), 404, "ERR_NOT_FOUND")
	f.exec("UPDATE meta.workspaces SET deleted_at=now() WHERE id=$1", x.w)
	f.expectStatus(f.api(owner, "GET", base+"ab", ""), 404, "ERR_NOT_FOUND")
	f.expectStatus(f.api(owner, "GET", groupPath+"ab", ""), 404, "ERR_NOT_FOUND")
}

func TestTableUserDirectoryAndLabelsRespectTableAccess(t *testing.T) {
	f := newACLFixture(t)
	owner, writer, reader, outsider := f.user("user", false), f.user("user", false), f.user("user", false), f.user("user", false)
	x := f.site()
	f.seedOwner(x.w, owner)
	f.grantOK(owner, acl.Table, x.t1, "user", writer.id, acl.Write)
	f.grantOK(owner, acl.Table, x.t1, "user", reader.id, acl.Read)
	disabled := f.user("user", false)
	f.exec("UPDATE meta.users SET status='disabled' WHERE id=$1", disabled.id)

	path := "/tables/" + x.t1 + "/directory/users?q=" + url.QueryEscape(writer.login)
	response := f.api(writer, "GET", path, "")
	f.expectStatus(response, 200, "")
	var listing struct {
		Items   []map[string]any `json:"items"`
		HasMore bool             `json:"hasMore"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &listing); err != nil {
		t.Fatal(err)
	}
	if len(listing.Items) != 1 || len(listing.Items[0]) != 3 ||
		listing.Items[0]["id"] != writer.id || listing.Items[0]["username"] != writer.login {
		t.Fatalf("minimal table picker response: %s", response.Body)
	}
	f.expectStatus(f.api(reader, "GET", path, ""), 403, "ERR_PERMISSION_DENIED")
	f.expectStatus(f.api(outsider, "GET", path, ""), 404, "ERR_NOT_FOUND")
	f.expectStatus(f.api(writer, "GET", "/tables/"+x.t1+"/directory/users?q=x", ""), 400, "ERR_VALIDATION")
	f.expectStatus(f.api(writer, "GET", "/directory/users?resource=table:"+x.t1+"&q="+url.QueryEscape(writer.login), ""),
		403, "ERR_PERMISSION_DENIED")

	unknown := "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"
	body, err := json.Marshal(map[string][]string{"ids": {writer.id, disabled.id, unknown, writer.id}})
	if err != nil {
		t.Fatal(err)
	}
	labelsResponse := f.api(reader, "POST", "/tables/"+x.t1+"/directory/user-labels", string(body))
	f.expectStatus(labelsResponse, 200, "")
	var labels struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(labelsResponse.Body.Bytes(), &labels); err != nil {
		t.Fatal(err)
	}
	if len(labels.Items) != 2 || len(labels.Items[0]) != 2 || len(labels.Items[1]) != 2 {
		t.Fatalf("submitted-id label response: %s", labelsResponse.Body)
	}
	f.expectStatus(f.api(outsider, "POST", "/tables/"+x.t1+"/directory/user-labels", string(body)), 404, "ERR_NOT_FOUND")
	f.expectStatus(f.api(reader, "POST", "/tables/"+x.t1+"/directory/user-labels",
		`{"ids":["0190A1B2-C3D4-7E5F-8A9B-0C1D2E3F4A5B"]}`), 400, "ERR_VALIDATION")
}
