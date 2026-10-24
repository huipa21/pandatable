package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

const rollbackTimeout = 5 * time.Second

type transaction interface {
	Commit(context.Context) error
	Rollback(context.Context) error
}

func (pool *Pool) WithTx(ctx context.Context, fn func(pgx.Tx) error) error {
	if pool == nil || pool.Pool == nil {
		return errors.New("database pool is nil")
	}
	if fn == nil {
		return errors.New("transaction callback is nil")
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	return withTransaction(ctx, tx, func() error {
		return fn(tx)
	})
}

func withTransaction(ctx context.Context, tx transaction, fn func() error) error {
	if err := fn(); err != nil {
		if rollbackErr := rollback(tx, ctx); rollbackErr != nil {
			return errors.Join(err, fmt.Errorf("rollback transaction: %w", rollbackErr))
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		commitErr := fmt.Errorf("commit transaction: %w", err)
		if rollbackErr := rollback(tx, ctx); rollbackErr != nil {
			return errors.Join(commitErr, fmt.Errorf("rollback transaction after commit failure: %w", rollbackErr))
		}
		return commitErr
	}
	return nil
}

func rollback(tx transaction, ctx context.Context) error {
	rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), rollbackTimeout)
	defer cancel()
	err := tx.Rollback(rollbackCtx)
	if errors.Is(err, pgx.ErrTxClosed) {
		return nil
	}
	return err
}
