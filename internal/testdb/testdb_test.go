package testdb

import (
	"context"
	"errors"
	"net"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestConnectionFailures(t *testing.T) {
	for _, value := range []string{"not a URL", "postgres://user:secret@localhost/db?connect_timeout=no", "https://localhost"} {
		var suite Suite
		if err := suite.open(value); err == nil || suite.unavailable || strings.Contains(err.Error(), "secret") {
			t.Fatalf("invalid config must fail without credentials: %v, unavailable=%v", err, suite.unavailable)
		}
	}
	var absent Suite
	if err := absent.open(""); err == nil || !absent.unavailable {
		t.Fatal("absent configuration must be clearly unavailable")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	var closed Suite
	start := time.Now()
	err = closed.open("postgres://user:secret@" + address + "/postgres?sslmode=disable")
	if err == nil || !closed.unavailable || time.Since(start) > 4*time.Second || strings.Contains(err.Error(), "secret") {
		t.Fatalf("closed port: %v, unavailable=%v, duration=%s", err, closed.unavailable, time.Since(start))
	}
	for _, err := range []error{
		&pgconn.PgError{Code: "28P01", Message: "secret"},
		errors.New("TLS negotiation failed"),
		context.DeadlineExceeded,
		&net.OpError{Op: "read", Err: context.DeadlineExceeded},
	} {
		if unreachable(err) {
			t.Fatalf("operational error classified as unreachable: %T", err)
		}
	}
}

func TestSuiteIsolationPostgreSQL(t *testing.T) {
	var first, second Suite
	for _, suite := range []*Suite{&first, &second} {
		t.Cleanup(func() {
			if err := suite.close(); err != nil {
				t.Error(err)
			}
		})
	}
	firstPool, secondPool := first.Open(t), second.Open(t)
	if first.name == second.name || first.name == "" {
		t.Fatal("suites reused the same database")
	}
	if first.URL(t) != first.databaseURL {
		t.Fatal("suite did not reuse its per-package database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := firstPool.Exec(ctx, `INSERT INTO meta.settings (key, value) VALUES ('testdb_isolation', 'true')`); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := secondPool.QueryRow(ctx, `SELECT count(*) FROM meta.settings WHERE key = 'testdb_isolation'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("cross-suite data leakage: count=%d, err=%v", count, err)
	}
	for _, mode := range []string{"authentication", "missing database"} {
		t.Run(mode, func(t *testing.T) {
			parsed, err := url.Parse(first.databaseURL)
			if err != nil {
				t.Fatal("parse disposable URL")
			}
			if mode == "authentication" {
				parsed.User = url.UserPassword(first.adminConfig.User, "deliberately-incorrect-password")
				query := parsed.Query()
				query.Del("password")
				parsed.RawQuery = query.Encode()
			} else {
				parsed.Path = "/" + first.name + "_absent"
			}
			var broken Suite
			t.Cleanup(func() {
				if err := broken.close(); err != nil {
					t.Error(err)
				}
			})
			err = broken.open(parsed.String())
			if err == nil || broken.unavailable || !strings.Contains(err.Error(), "SQLSTATE") ||
				strings.Contains(err.Error(), "deliberately-incorrect-password") {
				t.Fatalf("%s must fail, never skip, with redacted SQLSTATE: %v", mode, err)
			}
			t.Logf("%s: %v (not an unavailable/skip result)", mode, err)
		})
	}
	firstPool.Close()
	name := first.name
	// The shared budget above is already spent; the forced-cleanup probe and
	// its verification get their own deadline.
	cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancelCleanup()
	// FORCE must also reap forgotten connections, not only cooperative pools.
	leaked, err := pgx.Connect(cleanupCtx, first.databaseURL)
	if err != nil {
		t.Fatal("open deliberate cleanup probe connection")
	}
	defer leaked.Close(cleanupCtx)
	if err := first.close(); err != nil {
		t.Fatal(err)
	}
	admin, err := pgx.Connect(cleanupCtx, os.Getenv("PT_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal("connect cleanup verification")
	}
	defer admin.Close(cleanupCtx)
	if err := admin.QueryRow(cleanupCtx, `SELECT count(*) FROM pg_database WHERE datname = $1`, name).Scan(&count); err != nil || count != 0 {
		t.Fatalf("owned database remains: count=%d, err=%v", count, err)
	}
	t.Log("separate migrated databases, same-suite reuse, no shared data, forced connection cleanup and database removal verified")
}
