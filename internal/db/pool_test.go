package db

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"pandatable/internal/config"
)

func TestMakePoolConfigSetsConfiguredMaxConnections(t *testing.T) {
	poolConfig, err := makePoolConfig(config.Config{
		DatabaseURL: "postgres://user:password@localhost/pandatable",
		DBMaxConns:  7,
	})
	if err != nil {
		t.Fatal(err)
	}
	if poolConfig.MaxConns != 7 {
		t.Errorf("MaxConns = %d, want 7", poolConfig.MaxConns)
	}
}

func TestMakePoolConfigRejectsInvalidConnectionLimit(t *testing.T) {
	maxConns := []int{0}
	if int64(int(^uint(0)>>1)) > math.MaxInt32 {
		maxConns = append(maxConns, math.MaxInt32+1)
	}
	for _, maxConns := range maxConns {
		_, err := makePoolConfig(config.Config{
			DatabaseURL: "postgres://user:password@localhost/pandatable",
			DBMaxConns:  maxConns,
		})
		if err == nil || strings.Contains(err.Error(), "password") {
			t.Errorf("makePoolConfig(%d) error = %v, want safe validation error", maxConns, err)
		}
	}
}

func TestRedactDatabaseErrorNeverIncludesConnectionURLOrPassword(t *testing.T) {
	databaseURL := "postgres://user:top-secret@localhost/pandatable?sslmode=disable"
	err := errors.New("failed to connect using " + databaseURL)
	message := redactDatabaseError(err, databaseURL)
	if strings.Contains(message, databaseURL) || strings.Contains(message, "top-secret") {
		t.Fatalf("redactDatabaseError() = %q, contains connection credentials", message)
	}
}

func TestConnectWithRetriesStopsAfterConfiguredAttempts(t *testing.T) {
	attempts := 0
	got, actualAttempts, err := connectWithRetries(context.Background(), 2, 0, func() (int, error) {
		attempts++
		if attempts < 3 {
			return 0, errors.New("database unavailable")
		}
		return 42, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != 42 || attempts != 3 || actualAttempts != 3 {
		t.Fatalf("got=%d calls=%d attempts=%d, want 42 and 3 attempts", got, attempts, actualAttempts)
	}
}

func TestConnectWithRetriesReturnsFinalError(t *testing.T) {
	connectErr := errors.New("database unavailable")
	attempts := 0
	_, actualAttempts, err := connectWithRetries(context.Background(), 1, 0, func() (int, error) {
		attempts++
		return 0, connectErr
	})
	if !errors.Is(err, connectErr) {
		t.Fatalf("error = %v, want original connection error", err)
	}
	if attempts != 2 || actualAttempts != 2 {
		t.Fatalf("calls=%d attempts=%d, want 2 attempts", attempts, actualAttempts)
	}
}

func TestConnectWithRetriesHonorsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	attempts := 0
	_, actualAttempts, err := connectWithRetries(ctx, 3, time.Hour, func() (int, error) {
		attempts++
		return 0, errors.New("database unavailable")
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context cancellation", err)
	}
	if attempts != 1 || actualAttempts != 1 {
		t.Fatalf("calls=%d attempts=%d, want 1 attempt", attempts, actualAttempts)
	}
}
