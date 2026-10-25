package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	"pandatable/internal/acl"
	"pandatable/internal/admin"
	"pandatable/internal/auth"
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
	authService, err := auth.New(pool)
	if err != nil {
		return fmt.Errorf("configure auth service: %w", err)
	}
	resolver, err := acl.NewResolver(pool)
	if err != nil {
		return fmt.Errorf("configure ACL resolver: %w", err)
	}
	aclService, err := acl.NewService(pool, resolver)
	if err != nil {
		return fmt.Errorf("configure grants service: %w", err)
	}
	adminService, err := admin.NewService(pool, authService, resolver)
	if err != nil {
		return fmt.Errorf("configure admin service: %w", err)
	}
	setupToken, setupNeeded, err := authService.InitializeSetup(ctx)
	if err != nil {
		return fmt.Errorf("initialize first-run setup: %w", err)
	}
	if setupNeeded {
		logger.Warn("SETUP_TOKEN=" + setupToken)
	}
	handler, err := httpapi.New(logger, assets, httpapi.AuthOptions{
		Service: authService, ACL: aclService, Admin: adminService, BaseURL: cfg.BaseURL, TrustedProxies: cfg.TrustedProxies,
	})
	if err != nil {
		return fmt.Errorf("configure HTTP handler: %w", err)
	}
	listener, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return fmt.Errorf("listen for HTTP: %w", err)
	}
	return httpapi.Run(ctx, listener, handler, logger)
}
