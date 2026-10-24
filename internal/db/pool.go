package db

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"pandatable/internal/config"
)

const connectRetryInterval = 2 * time.Second

type Pool struct {
	*pgxpool.Pool
	databaseURL string
}

func Open(ctx context.Context, cfg config.Config) (*Pool, error) {
	poolConfig, err := makePoolConfig(cfg)
	if err != nil {
		return nil, err
	}
	pool, attempts, err := connectWithRetries(ctx, cfg.DBConnectRetries, connectRetryInterval, func() (*pgxpool.Pool, error) {
		pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
		if err == nil {
			err = pool.Ping(ctx)
		}
		if err != nil && pool != nil {
			pool.Close()
		}
		return pool, err
	})
	if err != nil {
		return nil, fmt.Errorf("connect to PostgreSQL after %d attempt(s): %s", attempts, redactDatabaseError(err, cfg.DatabaseURL))
	}
	return &Pool{Pool: pool, databaseURL: cfg.DatabaseURL}, nil
}

func makePoolConfig(cfg config.Config) (*pgxpool.Config, error) {
	if cfg.DBMaxConns < 1 || int64(cfg.DBMaxConns) > int64(^uint32(0)>>1) {
		return nil, fmt.Errorf("PT_DB_MAX_CONNS must be between 1 and %d", int64(^uint32(0)>>1))
	}
	if cfg.DBConnectRetries < 0 {
		return nil, fmt.Errorf("PT_DB_CONNECT_RETRIES must be non-negative")
	}
	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse PostgreSQL connection configuration: %s", redactDatabaseError(err, cfg.DatabaseURL))
	}
	poolConfig.MaxConns = int32(cfg.DBMaxConns)
	return poolConfig, nil
}

func connectWithRetries[T any](ctx context.Context, retries int, interval time.Duration, connect func() (T, error)) (T, int, error) {
	var zero T
	for attempt := 1; ; attempt++ {
		value, err := connect()
		if err == nil {
			return value, attempt, nil
		}
		if ctx.Err() != nil {
			return zero, attempt, ctx.Err()
		}
		if attempt > retries {
			return zero, attempt, err
		}

		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return zero, attempt, ctx.Err()
		case <-timer.C:
		}
	}
}

func redactDatabaseError(err error, databaseURL string) string {
	message := err.Error()
	if databaseURL != "" {
		message = strings.ReplaceAll(message, databaseURL, "PostgreSQL connection URL")
		if parsed, parseErr := url.Parse(databaseURL); parseErr == nil {
			if password, ok := parsed.User.Password(); ok && password != "" {
				message = strings.ReplaceAll(message, password, "[REDACTED]")
				message = strings.ReplaceAll(message, url.QueryEscape(password), "[REDACTED]")
			}
		}
	}
	return message
}
