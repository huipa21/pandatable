package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"pandatable/internal/backup"
	"pandatable/internal/config"
	"pandatable/internal/db"
)

func runRestore(args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(stdout, "Usage: pandatable restore <archive> --yes\n\nStop the application first. Uses PT_DATABASE_URL and PT_DATA_DIR; restoring replaces the database and attachments.")
		return 0
	}
	var archive string
	confirmed := false
	for _, arg := range args {
		switch {
		case arg == "--yes" && !confirmed:
			confirmed = true
		case strings.HasPrefix(arg, "-") || archive != "":
			fmt.Fprintln(stderr, "pandatable: invalid restore arguments (use --help)")
			return 2
		default:
			archive = arg
		}
	}
	if archive == "" || !confirmed {
		fmt.Fprintln(stderr, "pandatable: restore requires an archive and --yes")
		return 2
	}
	cfg, err := config.Parse(nil)
	if err != nil {
		fmt.Fprintf(stderr, "pandatable: %v\n", err)
		var usage config.UsageError
		if errors.As(err, &usage) {
			return 2
		}
		return 1
	}
	logger := newJSONLogger(stdout, cfg.LogLevel)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := db.Open(ctx, cfg)
	if err != nil {
		logger.Error("backup restore failed", "error", err)
		return 1
	}
	defer pool.Close()
	if err := backup.Restore(ctx, archive, backup.RestoreOptions{
		Pool: pool, DatabaseURL: cfg.DatabaseURL, DataDir: cfg.DataDir, AppVersion: version, Logger: logger,
	}); err != nil {
		logger.Error("backup restore failed", "error", err)
		return 1
	}
	logger.Info("backup restore complete")
	return 0
}
