package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"

	"pandatable/internal/acl"
	"pandatable/internal/admin"
	"pandatable/internal/attachments"
	"pandatable/internal/auth"
	"pandatable/internal/config"
	"pandatable/internal/db"
	"pandatable/internal/httpapi"
	"pandatable/internal/realtime"
	"pandatable/internal/structure"
	"pandatable/internal/web"
)

var processStarted = time.Now()

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
	structureService, err := structure.New(pool, resolver)
	if err != nil {
		return fmt.Errorf("configure structure service: %w", err)
	}
	adminService, err := admin.NewService(pool, authService, resolver)
	if err != nil {
		return fmt.Errorf("configure admin service: %w", err)
	}
	attachmentService, err := attachments.New(pool, resolver, cfg.DataDir)
	if err != nil {
		return fmt.Errorf("configure attachment service: %w", err)
	}
	hub, err := realtime.New(pool, resolver, authService, logger)
	if err != nil {
		return fmt.Errorf("configure realtime hub: %w", err)
	}
	if err := hub.Start(ctx); err != nil {
		return fmt.Errorf("start realtime hub: %w", err)
	}
	defer hub.Close()
	monitor, err := admin.NewSystemMonitor(ctx, adminService, cfg.DataDir, version, processStarted, logger)
	if err != nil {
		return fmt.Errorf("configure system monitor: %w", err)
	}
	defer monitor.Close()
	setupToken, setupNeeded, err := authService.InitializeSetup(ctx)
	if err != nil {
		return fmt.Errorf("initialize first-run setup: %w", err)
	}
	if setupNeeded {
		logger.Warn("SETUP_TOKEN=" + setupToken)
	}
	handler, err := httpapi.New(logger, assets, httpapi.AuthOptions{
		Service: authService, ACL: aclService, Structure: structureService, Admin: adminService, System: monitor,
		Attachments: attachmentService, Realtime: hub, BaseURL: cfg.BaseURL, TrustedProxies: cfg.TrustedProxies,
	})
	if err != nil {
		return fmt.Errorf("configure HTTP handler: %w", err)
	}
	listener, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return fmt.Errorf("listen for HTTP: %w", err)
	}
	autoIndexCtx, stopAutoIndex := context.WithCancel(ctx)
	autoIndexDone := make(chan struct{})
	go func() {
		defer close(autoIndexDone)
		structureService.RunAutoIndexWorker(autoIndexCtx, logger)
	}()
	defer func() {
		stopAutoIndex()
		<-autoIndexDone
	}()
	return httpapi.Run(ctx, listener, handler, logger)
}
