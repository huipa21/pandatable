package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	"pandatable/internal/config"
	"pandatable/internal/db"
	"pandatable/internal/httpapi"
	"pandatable/internal/web"
)

func serve(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	pool, err := db.Open(ctx, cfg)
	if err != nil {
		if ctx.Err() != nil {
			logger.Info("HTTP startup canceled")
			return nil
		}
		return fmt.Errorf("open database: %w", err)
	}
	defer pool.Close()
	if err := db.RunMigrations(ctx, pool); err != nil {
		if ctx.Err() != nil {
			logger.Info("HTTP startup canceled")
			return nil
		}
		return fmt.Errorf("migrate database: %w", err)
	}
	logger.Info("database migrations complete")
	assets, err := web.Assets()
	if err != nil {
		return fmt.Errorf("open embedded web assets: %w", err)
	}
	handler, err := httpapi.New(logger, assets)
	if err != nil {
		return fmt.Errorf("configure HTTP handler: %w", err)
	}
	listener, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return fmt.Errorf("listen for HTTP: %w", err)
	}
	return httpapi.Run(ctx, listener, handler, logger)
}
