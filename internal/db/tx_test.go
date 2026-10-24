package db

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

type fakeTransaction struct {
	commitErr   error
	rollbackErr error
	commits     int
	rollbacks   int
}

func (tx *fakeTransaction) Commit(context.Context) error {
	tx.commits++
	return tx.commitErr
}

func (tx *fakeTransaction) Rollback(context.Context) error {
	tx.rollbacks++
	return tx.rollbackErr
}

func TestWithTxRejectsNilPool(t *testing.T) {
	var pool *Pool
	if err := pool.WithTx(context.Background(), func(pgx.Tx) error { return nil }); err == nil {
		t.Fatal("WithTx() error = nil, want nil-pool error")
	}
}

func TestWithTransactionCommitsOnSuccess(t *testing.T) {
	tx := &fakeTransaction{}
	if err := withTransaction(context.Background(), tx, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if tx.commits != 1 || tx.rollbacks != 0 {
		t.Fatalf("commits=%d rollbacks=%d, want 1 and 0", tx.commits, tx.rollbacks)
	}
}

func TestWithTransactionRollsBackCallbackFailure(t *testing.T) {
	callbackErr := errors.New("callback failed")
	tx := &fakeTransaction{}
	err := withTransaction(context.Background(), tx, func() error { return callbackErr })
	if !errors.Is(err, callbackErr) {
		t.Fatalf("error = %v, want original callback error", err)
	}
	if tx.commits != 0 || tx.rollbacks != 1 {
		t.Fatalf("commits=%d rollbacks=%d, want 0 and 1", tx.commits, tx.rollbacks)
	}
}

func TestWithTransactionPreservesCallbackAndRollbackFailures(t *testing.T) {
	callbackErr := errors.New("callback failed")
	rollbackErr := errors.New("rollback failed")
	tx := &fakeTransaction{rollbackErr: rollbackErr}
	err := withTransaction(context.Background(), tx, func() error { return callbackErr })
	if !errors.Is(err, callbackErr) || !errors.Is(err, rollbackErr) {
		t.Fatalf("error = %v, want both callback and rollback errors", err)
	}
}

func TestWithTransactionRollsBackCommitFailure(t *testing.T) {
	commitErr := errors.New("commit failed")
	tx := &fakeTransaction{commitErr: commitErr}
	err := withTransaction(context.Background(), tx, func() error { return nil })
	if !errors.Is(err, commitErr) {
		t.Fatalf("error = %v, want original commit error", err)
	}
	if tx.commits != 1 || tx.rollbacks != 1 {
		t.Fatalf("commits=%d rollbacks=%d, want 1 and 1", tx.commits, tx.rollbacks)
	}
}

func TestWithTransactionPreservesCommitAndRollbackFailures(t *testing.T) {
	commitErr := errors.New("commit failed")
	rollbackErr := errors.New("rollback failed")
	tx := &fakeTransaction{commitErr: commitErr, rollbackErr: rollbackErr}
	err := withTransaction(context.Background(), tx, func() error { return nil })
	if !errors.Is(err, commitErr) || !errors.Is(err, rollbackErr) {
		t.Fatalf("error = %v, want both commit and rollback errors", err)
	}
}
