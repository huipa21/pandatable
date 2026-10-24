package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"pandatable/migrations"
)

const migrationLockKey int64 = 0x50544D49

func RunMigrations(ctx context.Context, pool *Pool) (result error) {
	if pool == nil || pool.Pool == nil {
		return errors.New("database pool is nil")
	}
	connConfig := pool.Config().ConnConfig.Copy()
	migrationDB := stdlib.OpenDB(*connConfig)
	defer func() {
		if err := migrationDB.Close(); err != nil {
			result = errors.Join(result, fmt.Errorf("close migration database: %s", redactDatabaseError(err, pool.databaseURL)))
		}
	}()
	migrationDB.SetMaxOpenConns(2)
	migrationDB.SetMaxIdleConns(1)

	if err := runMigrations(ctx, migrationDB); err != nil {
		return errors.New(redactDatabaseError(err, pool.databaseURL))
	}
	return nil
}

func runMigrations(ctx context.Context, migrationDB *sql.DB) (result error) {
	connection, err := migrationDB.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration lock connection: %w", err)
	}
	defer func() {
		result = errors.Join(result, connection.Close())
	}()

	if _, err := connection.ExecContext(ctx, "SELECT pg_advisory_lock($1)", migrationLockKey); err != nil {
		return fmt.Errorf("acquire migration advisory lock: %w", err)
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := connection.ExecContext(unlockCtx, "SELECT pg_advisory_unlock($1)", migrationLockKey); err != nil {
			result = errors.Join(result, fmt.Errorf("release migration advisory lock: %w", err))
		}
	}()

	provider, err := goose.NewProvider(goose.DialectPostgres, migrationDB, migrations.FS)
	if err != nil {
		return fmt.Errorf("configure Goose migrations: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("apply database migrations: %w", err)
	}
	return nil
}
