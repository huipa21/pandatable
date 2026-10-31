// Package testdb owns one migrated, disposable PostgreSQL 18 database per test
// package. Declare a Suite at package scope and pass TestMain to Suite.Run.
package testdb

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"pandatable/internal/config"
	"pandatable/internal/db"
)

const connectTimeout = 3 * time.Second

// cleanupTimeout covers the whole owned-database teardown. `go test ./...`
// drops every package database against one loaded server, where DROP DATABASE
// needs far more than a connect budget.
const cleanupTimeout = 60 * time.Second

// objectInUse is returned when a session reconnected between the FORCE
// termination and the drop itself.
const objectInUse = "55006"

type Suite struct {
	once        sync.Once
	adminConfig *pgx.ConnConfig
	name        string
	databaseURL string
	unavailable bool
	err         error
}

func (suite *Suite) Run(m *testing.M) int {
	code := m.Run()
	if err := suite.close(); err != nil {
		fmt.Fprintln(os.Stderr, "testdb:", err)
		return 1
	}
	return code
}

func (suite *Suite) URL(t testing.TB) string {
	t.Helper()
	suite.once.Do(func() {
		suite.err = suite.open(os.Getenv("PT_TEST_DATABASE_URL"))
	})
	if suite.err != nil {
		if suite.unavailable && os.Getenv("PT_TEST_DATABASE_REQUIRED") != "1" {
			t.Skipf("testdb: %v; skipping PostgreSQL integration (set PT_TEST_DATABASE_URL or run ptdev test)", suite.err)
		}
		t.Fatalf("testdb: %v", suite.err)
	}
	t.Logf("testdb: isolated PostgreSQL 18 database %s", suite.name)
	return suite.databaseURL
}

func (suite *Suite) Open(t testing.TB) *db.Pool {
	t.Helper()
	databaseURL := suite.URL(t)
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
	pool, err := db.Open(ctx, config.Config{DatabaseURL: databaseURL, DBMaxConns: 4})
	if err != nil {
		t.Fatal("testdb: open disposable database failed (connection details redacted)")
	}
	t.Cleanup(pool.Close)
	return pool
}

// CheckPoolOwnership ensures that a pool connects to this suite's disposable database.
func (suite *Suite) CheckPoolOwnership(ctx context.Context, pool *db.Pool) error {
	if suite == nil || suite.name == "" {
		return errors.New("testdb: an initialized Suite-owned database is required")
	}
	if pool == nil {
		return errors.New("testdb: a database pool is required")
	}
	var name string
	if err := pool.QueryRow(ctx, "SELECT current_database()").Scan(&name); err != nil {
		return operationError("identify database", err)
	}
	if name != suite.name {
		return errors.New("testdb: database is not owned by this Suite")
	}
	return nil
}

func (suite *Suite) open(databaseURL string) (result error) {
	if databaseURL == "" {
		suite.unavailable = true
		return errors.New("PT_TEST_DATABASE_URL is not set")
	}
	parsed, err := url.Parse(databaseURL)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		return errors.New("PT_TEST_DATABASE_URL must be a valid PostgreSQL URL (details redacted)")
	}
	adminConfig, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		return errors.New("invalid PT_TEST_DATABASE_URL (details redacted)")
	}
	adminConfig.ConnectTimeout = connectTimeout
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	admin, err := pgx.ConnectConfig(ctx, adminConfig)
	cancel()
	if err != nil {
		suite.unavailable = unreachable(err)
		if suite.unavailable {
			return errors.New("PostgreSQL unreachable within 3s")
		}
		return operationError("connect to PostgreSQL", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
		defer cancel()
		if err := admin.Close(ctx); err != nil {
			result = errors.Join(result, operationError("close admin connection", err))
		}
	}()
	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var version int
	if err := admin.QueryRow(ctx, "SELECT current_setting('server_version_num')::integer").Scan(&version); err != nil {
		return operationError("read PostgreSQL version", err)
	}
	if version < 180000 || version >= 190000 {
		return fmt.Errorf("PostgreSQL 18 required, got server_version_num=%d", version)
	}
	name := "pt_test_" + strings.ToLower(rand.Text())
	// A timed-out CREATE may have committed; retain the unique name for cleanup.
	suite.adminConfig, suite.name = adminConfig, name
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()+" TEMPLATE template0"); err != nil {
		return operationError("create disposable database (CREATEDB required)", err)
	}
	// Record ownership immediately, including when migration/open fails later.
	parsed.Path, parsed.RawPath = "/"+name, ""
	query := parsed.Query()
	query.Del("dbname")
	query.Del("database")
	parsed.RawQuery = query.Encode()
	suite.databaseURL = parsed.String()
	pool, err := db.Open(ctx, config.Config{DatabaseURL: suite.databaseURL, DBMaxConns: 4})
	if err != nil {
		return errors.New("open disposable database failed (connection details redacted)")
	}
	defer pool.Close()
	if err := db.RunMigrations(ctx, pool); err != nil {
		return errors.New("migrate disposable database failed (connection details redacted)")
	}
	return nil
}

func unreachable(err error) bool {
	var server *pgconn.PgError
	if errors.As(err, &server) {
		return false
	}
	var dns *net.DNSError
	var op *net.OpError
	return errors.As(err, &dns) || (errors.As(err, &op) && op.Op == "dial")
}

func operationError(operation string, err error) error {
	var server *pgconn.PgError
	if errors.As(err, &server) {
		return fmt.Errorf("%s failed (SQLSTATE %s; connection details redacted)", operation, server.Code)
	}
	return fmt.Errorf("%s failed (connection details redacted)", operation)
}

func (suite *Suite) close() error {
	if suite.name == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()
	connectCtx, cancelConnect := context.WithTimeout(ctx, connectTimeout)
	admin, err := pgx.ConnectConfig(connectCtx, suite.adminConfig.Copy())
	cancelConnect()
	if err != nil {
		return fmt.Errorf("cleanup %s: %w", suite.name, operationError("connect", err))
	}
	dropErr := dropDatabase(ctx, admin, suite.name)
	closeErr := admin.Close(ctx)
	if dropErr != nil {
		return fmt.Errorf("cleanup %s: %w", suite.name, operationError("drop owned database", dropErr))
	}
	if closeErr != nil {
		return operationError("close cleanup connection", closeErr)
	}
	fmt.Fprintln(os.Stderr, "testdb: dropped owned database", suite.name)
	suite.name = ""
	return nil
}

func dropDatabase(ctx context.Context, admin *pgx.Conn, name string) error {
	statement := "DROP DATABASE IF EXISTS " + pgx.Identifier{name}.Sanitize() + " WITH (FORCE)"
	for {
		_, err := admin.Exec(ctx, statement)
		var server *pgconn.PgError
		if err == nil || !errors.As(err, &server) || server.Code != objectInUse {
			return err
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(time.Second):
		}
	}
}
