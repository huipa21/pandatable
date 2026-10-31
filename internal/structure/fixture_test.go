package structure

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"pandatable/internal/acl"
	"pandatable/internal/apperr"
	"pandatable/internal/db"
	"pandatable/internal/testdb"
)

var database testdb.Suite

func TestMain(m *testing.M) { os.Exit(database.Run(m)) }

type fixture struct {
	t        testing.TB
	pool     *db.Pool
	ctx      context.Context
	resolver *acl.Resolver
	service  *Service
	users    []string
	spaces   []string
	groups   []string
}

func newFixture(t testing.TB) *fixture {
	t.Helper()
	pool := database.Open(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	resolver, err := acl.NewResolver(pool)
	if err != nil {
		t.Fatal(err)
	}
	service, err := New(pool, resolver)
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, pool: pool, ctx: ctx, resolver: resolver, service: service}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		principals := append(append([]string{}, f.users...), f.groups...)
		for _, sql := range []struct {
			query string
			ids   []string
		}{{"DELETE FROM meta.grants WHERE resource_id = ANY($1::uuid[]) OR principal_id = ANY($2::uuid[])", f.spaces},
			{"DELETE FROM meta.workspaces WHERE id = ANY($1::uuid[])", f.spaces},
			{"DELETE FROM meta.groups WHERE id = ANY($1::uuid[])", f.groups},
			{"DELETE FROM meta.users WHERE id = ANY($1::uuid[])", f.users}} {
			args := []any{sql.ids}
			if strings.Contains(sql.query, "$2") {
				args = append(args, principals)
			}
			if _, err := pool.Exec(cleanup, sql.query, args...); err != nil {
				t.Error(err)
			}
		}
	})
	return f
}

func (f *fixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(f.ctx, sql, args...); err != nil {
		f.t.Fatalf("%s: %v", sql, err)
	}
}

func scanOne[T any](f *fixture, sql string, args ...any) T {
	f.t.Helper()
	var value T
	if err := f.pool.QueryRow(f.ctx, sql, args...).Scan(&value); err != nil {
		f.t.Fatalf("%s: %v", sql, err)
	}
	return value
}

func (f *fixture) user(role, status, locale string) Actor {
	f.t.Helper()
	name := strings.ToLower(rand.Text())[:12]
	id := scanOne[string](f, `INSERT INTO meta.users (username,email,display_name,password_hash,role,status,locale,avatar_color)
		VALUES ($1,$2,$1,'not-a-login-hash',$3,$4,$5,'blue') RETURNING id::text`, name, name+"@structure.test", role, status, locale)
	f.users = append(f.users, id)
	return Actor{ID: id, Locale: locale}
}

func (f *fixture) group() string {
	id := scanOne[string](f, "INSERT INTO meta.groups (name) VALUES ($1) RETURNING id::text", strings.ToLower(rand.Text())[:12])
	f.groups = append(f.groups, id)
	return id
}

func (f *fixture) member(group string, user Actor) {
	f.exec("INSERT INTO meta.group_members (group_id,user_id) VALUES ($1,$2)", group, user.ID)
	f.resolver.Invalidate()
}

func (f *fixture) workspace(owner Actor) Workspace {
	f.t.Helper()
	workspace, err := f.service.CreateWorkspace(f.ctx, owner, WorkspaceInput{Name: "Workspace"})
	if err != nil {
		f.t.Fatal(err)
	}
	f.spaces = append(f.spaces, workspace.ID)
	return workspace
}

func (f *fixture) book(owner Actor, workspace string) Book {
	f.t.Helper()
	book, err := f.service.CreateBook(f.ctx, owner, workspace, "Book")
	if err != nil {
		f.t.Fatal(err)
	}
	return book
}

func (f *fixture) table(owner Actor, book string) TableSchema {
	f.t.Helper()
	table, err := f.service.CreateTable(f.ctx, owner, book, "Table")
	if err != nil {
		f.t.Fatal(err)
	}
	return table
}

// grant writes a grant directly (a fixture, not an API) and primes nothing.
func (f *fixture) grant(kind acl.ResourceType, resource, principalType, principal string, level acl.Level) {
	f.t.Helper()
	f.exec(`INSERT INTO meta.grants (resource_type,resource_id,principal_type,principal_id,level)
		VALUES ($1,$2,$3,$4,$5)`, string(kind), resource, principalType, principal, level.String())
	f.resolver.Invalidate()
}

func (f *fixture) level(user Actor, kind acl.ResourceType, id string) acl.Level {
	f.t.Helper()
	access, err := f.resolver.Access(f.ctx, user.ID, acl.Resource{Type: kind, ID: mustUUID(f.t, id)})
	if err != nil {
		f.t.Fatal(err)
	}
	return access.Level
}

func expectError(t *testing.T, err error, status int, code string) {
	t.Helper()
	var app *apperr.Error
	if !errors.As(err, &app) || app.Status != status || app.Code != code {
		t.Fatalf("error = %v, want HTTP %d %s", err, status, code)
	}
}

func (f *fixture) schemaExists(name string) bool {
	return scanOne[bool](f, "SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = $1)", name)
}

func (f *fixture) physicalNames(table string) (schema, name string) {
	f.t.Helper()
	err := f.pool.QueryRow(f.ctx, `SELECT b.pg_schema, t.pg_name FROM meta.tables t JOIN meta.books b ON b.id = t.book_id
		WHERE t.id = $1`, table).Scan(&schema, &name)
	if err != nil {
		f.t.Fatal(err)
	}
	return schema, name
}
