package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"pandatable/internal/acl"
	"pandatable/internal/admin"
	"pandatable/internal/attachments"
	"pandatable/internal/auth"
	"pandatable/internal/db"
	"pandatable/internal/structure"
)

type aclUser struct {
	id     string
	login  string
	cookie *http.Cookie
}

type aclFixture struct {
	t                 *testing.T
	pool              *db.Pool
	ctx               context.Context
	handler           http.Handler
	probes            http.Handler
	resolver          *acl.Resolver
	authService       *auth.Service
	aclService        *acl.Service
	attachmentService *attachments.Service
	phc               string
	logins            int
	users             []string
	spaces            []string
	groups            []string
}

const aclPassword = "Fixture-password-123"

func newACLFixture(t *testing.T) *aclFixture {
	t.Helper()
	pool := authDatabase.Open(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	authService, err := auth.New(pool)
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := acl.NewResolver(pool)
	if err != nil {
		t.Fatal(err)
	}
	aclService, err := acl.NewService(pool, resolver)
	if err != nil {
		t.Fatal(err)
	}
	f := &aclFixture{t: t, pool: pool, ctx: ctx, resolver: resolver, authService: authService, aclService: aclService}
	if f.phc, err = auth.NewPasswords().Hash(ctx, aclPassword); err != nil {
		t.Fatal(err)
	}
	structureService, err := structure.New(pool, resolver)
	if err != nil {
		t.Fatal(err)
	}
	adminService, err := admin.NewService(pool, authService, resolver)
	if err != nil {
		t.Fatal(err)
	}
	attachmentService, err := attachments.New(pool, resolver, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f.attachmentService = attachmentService
	options := AuthOptions{Service: authService, ACL: aclService, Structure: structureService, Admin: adminService,
		Attachments: attachmentService, BaseURL: "https://table.example"}
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	if f.handler, err = New(logger, testAssets(), options); err != nil {
		t.Fatal(err)
	}
	api, err := newAuthHTTP(logger, options)
	if err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	router.Use(api.protect)
	ok := func(w http.ResponseWriter, r *http.Request) {
		user, _ := UserFromContext(r.Context())
		writeJSON(w, http.StatusOK, map[string]string{"user": user.ID})
	}
	for _, kind := range []acl.ResourceType{acl.Workspace, acl.Book, acl.Table} {
		for _, level := range []acl.Level{acl.Read, acl.Write, acl.Owner} {
			router.With(api.require(level, kind, "id")).Get(fmt.Sprintf("/%ss/{id}/%s", kind, level), ok)
		}
	}
	f.probes = router
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		for _, sql := range []struct {
			query string
			ids   []string
		}{{"DELETE FROM meta.grants WHERE resource_id = ANY($1::uuid[]) OR principal_id = ANY($1::uuid[])", append(append(append([]string{}, f.spaces...), f.users...), f.groups...)},
			{"DELETE FROM meta.workspaces WHERE id = ANY($1::uuid[])", f.spaces},
			{"DELETE FROM meta.groups WHERE id = ANY($1::uuid[])", f.groups},
			{"DELETE FROM meta.users WHERE id = ANY($1::uuid[])", f.users}} {
			if _, err := pool.Exec(cleanup, sql.query, sql.ids); err != nil {
				t.Error(err)
			}
		}
	})
	return f
}

func (f *aclFixture) scan(sql string, args ...any) string {
	f.t.Helper()
	var id string
	if err := f.pool.QueryRow(f.ctx, sql, args...).Scan(&id); err != nil {
		f.t.Fatalf("%s: %v", sql, err)
	}
	return id
}

func (f *aclFixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(f.ctx, sql, args...); err != nil {
		f.t.Fatalf("%s: %v", sql, err)
	}
}

func (f *aclFixture) user(role string, mustChange bool) *aclUser {
	f.t.Helper()
	name := strings.ToLower(rand.Text())[:12]
	id := f.scan(`INSERT INTO meta.users (username,email,display_name,password_hash,role,must_change_password,avatar_color)
		VALUES ($1,$2,$1,$3,$4,$5,'blue') RETURNING id::text`, name, name+"@acl.test", f.phc, role, mustChange)
	f.users = append(f.users, id)
	user := &aclUser{id: id, login: name}
	f.logins++
	body, _ := json.Marshal(map[string]string{"login": name, "password": aclPassword})
	request := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(body))
	request.RemoteAddr = fmt.Sprintf("192.0.2.%d:4000", f.logins)
	f.decorate(request, nil)
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, request)
	if response.Code != 200 || len(response.Result().Cookies()) != 1 {
		f.t.Fatalf("login failed: %d %s", response.Code, response.Body)
	}
	user.cookie = response.Result().Cookies()[0]
	return user
}

func (f *aclFixture) decorate(request *http.Request, user *aclUser) {
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://table.example")
	request.Header.Set("X-PT-CSRF", "1")
	if user != nil {
		request.AddCookie(user.cookie)
	}
}

func (f *aclFixture) call(handler http.Handler, user *aclUser, method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.RemoteAddr = "192.0.2.250:5000"
	f.decorate(request, user)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func (f *aclFixture) api(user *aclUser, method, path, body string) *httptest.ResponseRecorder {
	return f.call(f.handler, user, method, "/api/v1"+path, body)
}

func (f *aclFixture) probe(user *aclUser, kind acl.ResourceType, id string, level acl.Level) int {
	return f.call(f.probes, user, "GET", fmt.Sprintf("/%ss/%s/%s", kind, id, level), "").Code
}

func (f *aclFixture) expectProbe(user *aclUser, kind acl.ResourceType, id string, level acl.Level, want int) {
	f.t.Helper()
	if got := f.probe(user, kind, id, level); got != want {
		f.t.Fatalf("%s %s requiring %s: HTTP %d, want %d", kind, id, level, got, want)
	}
}

func (f *aclFixture) put(caller *aclUser, kind acl.ResourceType, id, principalType, principal string, level acl.Level) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"resourceType": string(kind), "resourceId": id,
		"principalType": principalType, "principalId": principal, "level": level.String()})
	return f.api(caller, "PUT", "/grants", string(body))
}

func (f *aclFixture) expectStatus(response *httptest.ResponseRecorder, status int, code string) {
	f.t.Helper()
	if response.Code != status {
		f.t.Fatalf("HTTP %d, want %d: %s", response.Code, status, response.Body)
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		f.t.Fatal("grant responses must be no-store")
	}
	if code == "" {
		return
	}
	var body struct {
		Error struct{ Code string } `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.Error.Code != code {
		f.t.Fatalf("error code %q (%v), want %q: %s", body.Error.Code, err, code, response.Body)
	}
}

type grantBody struct {
	Grant struct {
		ID             string  `json:"id"`
		Level          string  `json:"level"`
		InheritedLevel *string `json:"inheritedLevel"`
	} `json:"grant"`
	Created bool   `json:"created"`
	EventID string `json:"eventId"`
}

func (f *aclFixture) grantOK(caller *aclUser, kind acl.ResourceType, id, principalType, principal string, level acl.Level) grantBody {
	f.t.Helper()
	response := f.put(caller, kind, id, principalType, principal, level)
	if response.Code != 200 && response.Code != 201 {
		f.t.Fatalf("grant %s: HTTP %d %s", level, response.Code, response.Body)
	}
	var body grantBody
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.EventID == "" || body.Grant.ID == "" {
		f.t.Fatalf("grant response: %v %s", err, response.Body)
	}
	return body
}

type site struct{ w, b1, b2, t1, t2, t3 string }

func (f *aclFixture) site() site {
	w := f.scan("INSERT INTO meta.workspaces (name) VALUES ('HTTP ACL') RETURNING id::text")
	f.spaces = append(f.spaces, w)
	book := func() string {
		return f.scan("INSERT INTO meta.books (workspace_id,name,pg_schema,position) VALUES ($1,'B',$2,1) RETURNING id::text",
			w, "b_"+strings.ToLower(rand.Text())[:10])
	}
	table := func(book string) string {
		return f.scan("INSERT INTO meta.tables (book_id,name,pg_name,position) VALUES ($1,'T',$2,1) RETURNING id::text",
			book, "t_"+strings.ToLower(rand.Text())[:10])
	}
	b1, b2 := book(), book()
	return site{w: w, b1: b1, b2: b2, t1: table(b1), t2: table(b1), t3: table(b2)}
}

func (f *aclFixture) seedOwner(workspace string, user *aclUser) {
	f.exec(`INSERT INTO meta.grants (resource_type,resource_id,principal_type,principal_id,level)
		VALUES ('workspace',$1,'user',$2,'owner')`, workspace, user.id)
	f.resolver.Invalidate()
}

func TestGrantAPIAcceptanceAT204ToAT212(t *testing.T) {
	f := newACLFixture(t)
	alice, bob, carol, dave := f.user("user", false), f.user("user", false), f.user("user", false), f.user("user", false)
	x := f.site()
	f.seedOwner(x.w, alice)

	t.Run("AT-204 workspace read", func(t *testing.T) {
		created := f.put(alice, acl.Workspace, x.w, "user", bob.id, acl.Read)
		f.expectStatus(created, 201, "")
		for _, check := range []struct {
			kind acl.ResourceType
			id   string
		}{{acl.Workspace, x.w}, {acl.Book, x.b1}, {acl.Table, x.t1}} {
			f.expectProbe(bob, check.kind, check.id, acl.Read, 200)
			f.expectProbe(bob, check.kind, check.id, acl.Write, 403)
			f.expectProbe(bob, check.kind, check.id, acl.Owner, 403)
		}
		f.expectStatus(f.put(bob, acl.Table, x.t1, "user", carol.id, acl.Read), 403, "ERR_PERMISSION_DENIED")
		f.expectStatus(f.api(bob, "GET", "/grants?resource=table:"+x.t1, ""), 403, "ERR_PERMISSION_DENIED")
	})

	t.Run("AT-205 write level", func(t *testing.T) {
		f.grantOK(alice, acl.Book, x.b1, "user", bob.id, acl.Write)
		f.expectProbe(bob, acl.Table, x.t1, acl.Write, 200)
		f.expectProbe(bob, acl.Table, x.t3, acl.Write, 403)
		f.expectProbe(bob, acl.Table, x.t1, acl.Owner, 403)
		f.expectProbe(bob, acl.Book, x.b1, acl.Owner, 403)
	})

	t.Run("AT-206 raise-only inheritance", func(t *testing.T) {
		erin := f.user("user", false)
		f.grantOK(alice, acl.Workspace, x.w, "user", erin.id, acl.Read)
		f.grantOK(alice, acl.Table, x.t1, "user", erin.id, acl.Write)
		f.expectProbe(erin, acl.Table, x.t1, acl.Write, 200)
		f.expectProbe(erin, acl.Table, x.t2, acl.Write, 403)
		f.expectProbe(erin, acl.Table, x.t2, acl.Read, 200)

		frank := f.user("user", false)
		f.grantOK(alice, acl.Workspace, x.w, "user", frank.id, acl.Write)
		warning := f.grantOK(alice, acl.Table, x.t1, "user", frank.id, acl.Read)
		if warning.Grant.InheritedLevel == nil || *warning.Grant.InheritedLevel != "write" {
			t.Fatalf("inherited-level warning missing: %+v", warning.Grant)
		}
		f.expectProbe(frank, acl.Table, x.t1, acl.Write, 200)
	})

	t.Run("AT-207 group grants", func(t *testing.T) {
		group := f.scan("INSERT INTO meta.groups (name) VALUES ($1) RETURNING id::text", "team-"+strings.ToLower(rand.Text())[:8])
		f.groups = append(f.groups, group)
		f.exec("INSERT INTO meta.group_members (group_id,user_id) VALUES ($1,$2)", group, carol.id)
		f.expectProbe(carol, acl.Table, x.t3, acl.Read, 404)
		f.grantOK(alice, acl.Book, x.b2, "group", group, acl.Write)
		f.expectProbe(carol, acl.Table, x.t3, acl.Write, 200)
		// The membership service (T1.6) must call Invalidate; AT-207's 5 s bound
		// is also enforced by the cache TTL (see acl tests).
		f.exec("DELETE FROM meta.group_members WHERE group_id=$1 AND user_id=$2", group, carol.id)
		f.resolver.Invalidate()
		f.expectProbe(carol, acl.Table, x.t3, acl.Read, 404)
	})

	t.Run("AT-208 table-only grant API visibility", func(t *testing.T) {
		f.grantOK(alice, acl.Table, x.t2, "user", carol.id, acl.Read)
		f.expectProbe(carol, acl.Table, x.t2, acl.Read, 200)
		f.expectProbe(carol, acl.Table, x.t1, acl.Read, 404)
		f.expectProbe(carol, acl.Book, x.b2, acl.Read, 404)
		f.expectProbe(carol, acl.Workspace, x.w, acl.Read, 403)
		f.expectProbe(carol, acl.Book, x.b1, acl.Read, 403)
		f.expectStatus(f.api(carol, "GET", "/grants?resource=workspace:"+x.w, ""), 403, "ERR_PERMISSION_DENIED")
		f.expectStatus(f.api(carol, "GET", "/grants?resource=book:"+x.b1, ""), 403, "ERR_PERMISSION_DENIED")
	})

	t.Run("AT-209 scope owner manages sub-scope", func(t *testing.T) {
		f.grantOK(alice, acl.Book, x.b1, "user", dave.id, acl.Owner)
		f.expectStatus(f.put(dave, acl.Book, x.b1, "user", carol.id, acl.Write), 201, "")
		f.expectStatus(f.put(dave, acl.Table, x.t1, "user", carol.id, acl.Write), 201, "")
		f.expectStatus(f.put(dave, acl.Workspace, x.w, "user", carol.id, acl.Write), 403, "ERR_PERMISSION_DENIED")
		f.expectStatus(f.put(dave, acl.Book, x.b2, "user", carol.id, acl.Write), 404, "ERR_NOT_FOUND")
		f.expectStatus(f.put(dave, acl.Table, x.t3, "user", carol.id, acl.Write), 404, "ERR_NOT_FOUND")
		f.expectStatus(f.api(dave, "GET", "/grants?resource=workspace:"+x.w, ""), 403, "ERR_PERMISSION_DENIED")
	})

	t.Run("AT-210 owner can grant owner", func(t *testing.T) {
		f.grantOK(alice, acl.Workspace, x.w, "user", bob.id, acl.Owner)
		f.expectStatus(f.put(bob, acl.Workspace, x.w, "user", carol.id, acl.Owner), 201, "")
		f.expectStatus(f.put(bob, acl.Table, x.t3, "user", dave.id, acl.Read), 201, "")
		f.expectProbe(carol, acl.Workspace, x.w, acl.Owner, 200)
	})

	t.Run("AT-211 last owner protected", func(t *testing.T) {
		y := f.site()
		solo, other := f.user("user", false), f.user("user", false)
		f.seedOwner(y.w, solo)
		f.expectStatus(f.put(solo, acl.Workspace, y.w, "user", solo.id, acl.Write), 409, "ERR_LAST_OWNER")
		f.expectStatus(f.put(solo, acl.Workspace, y.w, "user", solo.id, acl.Read), 409, "ERR_LAST_OWNER")
		own := f.scan("SELECT id::text FROM meta.grants WHERE resource_id=$1 AND principal_id=$2", y.w, solo.id)
		f.expectStatus(f.api(solo, "DELETE", "/grants/"+own, ""), 409, "ERR_LAST_OWNER")
		f.expectProbe(solo, acl.Workspace, y.w, acl.Owner, 200)
		f.grantOK(solo, acl.Workspace, y.w, "user", other.id, acl.Owner)
		f.expectStatus(f.put(solo, acl.Workspace, y.w, "user", solo.id, acl.Write), 200, "")
		f.expectProbe(solo, acl.Workspace, y.w, acl.Owner, 403)
	})

	t.Run("AT-212 hidden resources are 404", func(t *testing.T) {
		stranger := f.user("user", false)
		admin := f.user("admin", false)
		for _, user := range []*aclUser{stranger, admin} {
			f.expectProbe(user, acl.Table, x.t1, acl.Read, 404)
			f.expectProbe(user, acl.Book, x.b1, acl.Read, 404)
			f.expectProbe(user, acl.Workspace, x.w, acl.Read, 404)
			f.expectStatus(f.api(user, "GET", "/grants?resource=table:"+x.t1, ""), 404, "ERR_NOT_FOUND")
			f.expectStatus(f.put(user, acl.Workspace, x.w, "user", user.id, acl.Owner), 404, "ERR_NOT_FOUND")
		}
		f.expectProbe(stranger, acl.Table, "not-a-uuid", acl.Read, 404)
		f.expectProbe(stranger, acl.Table, "018f0000-0000-7000-8000-0000000000fb", acl.Read, 404)
	})
}

func TestGrantAPIListingAndDeleteContract(t *testing.T) {
	f := newACLFixture(t)
	alice, bob := f.user("user", false), f.user("user", false)
	x := f.site()
	f.seedOwner(x.w, alice)
	f.grantOK(alice, acl.Workspace, x.w, "user", bob.id, acl.Write)
	direct := f.grantOK(alice, acl.Table, x.t1, "user", bob.id, acl.Read)

	response := f.api(alice, "GET", "/grants?resource=table:"+x.t1, "")
	f.expectStatus(response, 200, "")
	var listing struct {
		Resource       struct{ Type, ID string }
		EffectiveLevel string `json:"effectiveLevel"`
		Items          []struct {
			ID             string  `json:"id"`
			PrincipalType  string  `json:"principalType"`
			Level          string  `json:"level"`
			InheritedLevel *string `json:"inheritedLevel"`
			Principal      struct {
				Name, Username string
				Status         string
			} `json:"principal"`
		} `json:"items"`
		Inherited []struct {
			ResourceType string `json:"resourceType"`
			Level        string `json:"level"`
		} `json:"inherited"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &listing); err != nil {
		t.Fatal(err)
	}
	if listing.EffectiveLevel != "owner" || len(listing.Items) != 1 || listing.Items[0].ID != direct.Grant.ID ||
		listing.Items[0].InheritedLevel == nil || *listing.Items[0].InheritedLevel != "write" ||
		listing.Items[0].Principal.Username != bob.login || listing.Items[0].Principal.Status != "active" ||
		len(listing.Inherited) != 2 || listing.Inherited[0].ResourceType != "workspace" {
		t.Fatalf("unexpected listing: %s", response.Body)
	}
	for _, secret := range []string{"password", "hash", "token"} {
		if strings.Contains(strings.ToLower(response.Body.String()), secret) {
			t.Fatalf("listing leaks %q", secret)
		}
	}

	f.expectStatus(f.api(alice, "DELETE", "/grants/"+direct.Grant.ID, ""), 200, "")
	// Deleting the lower direct grant never reduces the inherited write level.
	f.expectProbe(bob, acl.Table, x.t1, acl.Write, 200)
	f.expectProbe(bob, acl.Table, x.t1, acl.Owner, 403)
	f.expectStatus(f.api(alice, "DELETE", "/grants/"+direct.Grant.ID, ""), 404, "ERR_NOT_FOUND")
	f.expectStatus(f.api(alice, "DELETE", "/grants/not-a-uuid", ""), 400, "ERR_VALIDATION")
}

func TestGrantAPIProtocolAndAuthRules(t *testing.T) {
	f := newACLFixture(t)
	alice, bob, forced, owner2 := f.user("user", false), f.user("user", false), f.user("user", true), f.user("user", false)
	x := f.site()
	f.seedOwner(x.w, alice)
	f.seedOwner(x.w, forced)
	f.seedOwner(x.w, owner2)
	valid := fmt.Sprintf(`{"resourceType":"workspace","resourceId":%q,"principalType":"user","principalId":%q,"level":"read"}`, x.w, bob.id)

	// Authentication, forced password change and CSRF/Origin.
	f.expectStatus(f.api(nil, "GET", "/grants?resource=workspace:"+x.w, ""), 401, "ERR_UNAUTHENTICATED")
	f.expectStatus(f.api(nil, "PUT", "/grants", valid), 401, "ERR_UNAUTHENTICATED")
	f.expectStatus(f.api(forced, "GET", "/grants?resource=workspace:"+x.w, ""), 403, "ERR_PASSWORD_CHANGE_REQUIRED")
	f.expectStatus(f.api(forced, "PUT", "/grants", valid), 403, "ERR_PASSWORD_CHANGE_REQUIRED")
	f.expectProbe(forced, acl.Workspace, x.w, acl.Read, 403)
	for _, mutate := range []func(*http.Request){
		func(r *http.Request) { r.Header.Del("X-PT-CSRF") },
		func(r *http.Request) { r.Header.Set("X-PT-CSRF", "0") },
		func(r *http.Request) { r.Header.Del("Origin") },
		func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") },
		func(r *http.Request) { r.Header.Add("Origin", "https://table.example") },
	} {
		request := httptest.NewRequest("PUT", "/api/v1/grants", strings.NewReader(valid))
		request.RemoteAddr = "192.0.2.251:1"
		f.decorate(request, alice)
		mutate(request)
		response := httptest.NewRecorder()
		f.handler.ServeHTTP(response, request)
		f.expectStatus(response, 403, "ERR_CSRF")
	}
	if f.scan("SELECT count(*)::text FROM meta.grants WHERE resource_id=$1 AND principal_id=$2", x.w, bob.id) != "0" {
		t.Fatal("rejected unsafe requests must not mutate")
	}
	deleteCSRF := httptest.NewRequest("DELETE", "/api/v1/grants/"+x.w, nil)
	deleteCSRF.AddCookie(alice.cookie)
	deleteCSRF.Header.Set("Origin", "https://table.example")
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, deleteCSRF)
	f.expectStatus(response, 403, "ERR_CSRF")

	// Request body rules.
	f.expectStatus(f.api(alice, "PUT", "/grants", `{"resourceType":"workspace"`), 400, "ERR_VALIDATION")
	f.expectStatus(f.api(alice, "PUT", "/grants", valid[:len(valid)-1]+`,"extra":1}`), 400, "ERR_VALIDATION")
	f.expectStatus(f.api(alice, "PUT", "/grants", valid+valid), 400, "ERR_VALIDATION")
	f.expectStatus(f.api(alice, "PUT", "/grants", `{}`), 400, "ERR_VALIDATION")
	f.expectStatus(f.api(alice, "PUT", "/grants", `null`), 400, "ERR_VALIDATION")
	f.expectStatus(f.api(alice, "PUT", "/grants", `{"level":7}`), 400, "ERR_VALIDATION")
	f.expectStatus(f.api(alice, "PUT", "/grants", strings.Repeat(" ", 5000)+valid), 400, "ERR_VALIDATION")
	text := httptest.NewRequest("PUT", "/api/v1/grants", strings.NewReader(valid))
	text.AddCookie(alice.cookie)
	text.Header.Set("Origin", "https://table.example")
	text.Header.Set("X-PT-CSRF", "1")
	text.Header.Set("Content-Type", "text/plain")
	response = httptest.NewRecorder()
	f.handler.ServeHTTP(response, text)
	f.expectStatus(response, 415, "ERR_CONTENT_TYPE")

	// Query rules and method handling.
	for _, query := range []string{"", "?resource=", "?resource=workspace", "?resource=workspace:" + x.w + "&resource=workspace:" + x.w,
		"?resource=view:" + x.w, "?resource=workspace:xyz", "?other=1"} {
		f.expectStatus(f.api(alice, "GET", "/grants"+query, ""), 400, "ERR_VALIDATION")
	}
	post := f.api(alice, "POST", "/grants", valid)
	f.expectStatus(post, 405, "ERR_METHOD_NOT_ALLOWED")
	if post.Header().Get("Allow") != "GET, PUT" {
		t.Fatalf("Allow=%q", post.Header().Get("Allow"))
	}
	patch := f.api(alice, "PATCH", "/grants/"+x.w, "{}")
	f.expectStatus(patch, 405, "ERR_METHOD_NOT_ALLOWED")
	if patch.Header().Get("Allow") != "DELETE" {
		t.Fatalf("Allow=%q", patch.Header().Get("Allow"))
	}

	// Success shape and disabled-user denial.
	created := f.api(alice, "PUT", "/grants", valid)
	f.expectStatus(created, 201, "")
	for _, key := range []string{`"eventId"`, `"created":true`, `"resourceType":"workspace"`, `"principalId"`, `"grantedBy"`, `"createdAt"`} {
		if !strings.Contains(created.Body.String(), key) {
			t.Fatalf("response lacks %s: %s", key, created.Body)
		}
	}
	f.expectStatus(f.put(alice, acl.Workspace, x.w, "user", bob.id, acl.Read), 200, "")
	f.exec("UPDATE meta.users SET status='disabled' WHERE id=$1", alice.id)
	f.resolver.Invalidate()
	f.expectStatus(f.api(alice, "GET", "/grants?resource=workspace:"+x.w, ""), 401, "ERR_UNAUTHENTICATED")
	f.expectStatus(f.put(alice, acl.Workspace, x.w, "user", bob.id, acl.Write), 401, "ERR_UNAUTHENTICATED")
	f.expectProbe(alice, acl.Workspace, x.w, acl.Read, 401)
	disabledTarget := f.user("user", false)
	f.exec("UPDATE meta.users SET status='disabled' WHERE id=$1", disabledTarget.id)
	f.expectStatus(f.put(owner2, acl.Workspace, x.w, "user", disabledTarget.id, acl.Read), 409, "ERR_PRINCIPAL_DISABLED")
	f.expectStatus(f.put(owner2, acl.Workspace, x.w, "user", "018f0000-0000-7000-8000-0000000000f9", acl.Read), 404, "ERR_PRINCIPAL_NOT_FOUND")
}

func TestGrantRoutesAbsentWithoutACLService(t *testing.T) {
	pool := authDatabase.Open(t)
	service, err := auth.New(pool)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := New(slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)), testAssets(),
		AuthOptions{Service: service, BaseURL: "https://table.example"})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "/api/v1/grants?resource=workspace:018f0000-0000-7000-8000-0000000000fa", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 404 {
		t.Fatalf("grants routes exist only when the ACL service is configured, got %d", response.Code)
	}
}
