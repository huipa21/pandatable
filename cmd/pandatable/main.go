package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"pandatable/internal/config"
	"pandatable/internal/db"
)

var version = "0.0.0-dev"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	return runWith(args, os.Stdout, os.Stderr)
}

func runWith(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}

	command := args[0]
	commandArgs := args[1:]
	if command == "--help" || command == "-h" || command == "help" {
		if len(commandArgs) != 0 {
			fmt.Fprintln(stderr, "pandatable: unexpected arguments for help")
			return 2
		}
		printUsage(stdout)
		return 0
	}

	switch command {
	case "dev":
		return runDev(commandArgs, stdout, stderr)
	case "healthcheck":
		listen, err := config.HealthcheckListen(commandArgs)
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(stdout, "Usage: pandatable healthcheck [--listen host:port]\n\nUses PT_LISTEN (default :8080); GET /healthz, 2s limit, no database required.")
			return 0
		}
		if err != nil {
			fmt.Fprintf(stderr, "pandatable: %v\n", err)
			var usage config.UsageError
			if errors.As(err, &usage) {
				return 2
			}
			return 1
		}
		if err := healthcheck(context.Background(), listen); err != nil {
			newJSONLogger(stdout, slog.LevelInfo).Error("healthcheck failed", "error", err)
			return 1
		}
		return 0
	case "version":
		if len(commandArgs) != 0 {
			fmt.Fprintln(stderr, "pandatable: version does not accept arguments")
			return 2
		}
		fmt.Fprintln(stdout, version)
		return 0
	case "serve", "migrate":
		cfg, err := config.Parse(commandArgs)
		if errors.Is(err, flag.ErrHelp) {
			printCommandUsage(stdout, command)
			return 0
		}
		if err != nil {
			fmt.Fprintf(stderr, "pandatable: %v\n", err)
			var usage config.UsageError
			if errors.As(err, &usage) {
				return 2
			}
			return 1
		}
		logger := newJSONLogger(stdout, cfg.LogLevel)
		switch command {
		case "serve":
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			if err := serve(ctx, cfg, logger); err != nil {
				logger.Error("HTTP server failed", "error", err)
				return 1
			}
			return 0
		case "migrate":
			pool, err := db.Open(context.Background(), cfg)
			if err != nil {
				logger.Error("database migration failed", "error", err)
				return 1
			}
			defer pool.Close()
			if err := db.RunMigrations(context.Background(), pool); err != nil {
				logger.Error("database migration failed", "error", err)
				return 1
			}
			logger.Info("database migrations complete")
			return 0
		}
	case "restore":
		return runRestore(commandArgs, stdout, stderr)
	case "doctor", "reset-admin-password":
		if len(commandArgs) == 1 && (commandArgs[0] == "--help" || commandArgs[0] == "-h") {
			fmt.Fprintf(stdout, "Usage: pandatable %s\n", command)
			return 0
		}
		if len(commandArgs) != 0 {
			fmt.Fprintf(stderr, "pandatable: %s does not accept arguments\n", command)
			return 2
		}
		task := map[string]string{
			"doctor":               "unassigned",
			"reset-admin-password": "T1.5",
		}[command]
		return reportUnimplemented(newJSONLogger(stdout, slog.LevelInfo), command, task)
	default:
		fmt.Fprintln(stderr, "pandatable: unknown command")
		printUsage(stderr)
		return 2
	}
	return 1
}

func reportUnimplemented(logger *slog.Logger, command, task string) int {
	logger.Error("command is not implemented", "command", command, "implementation", task)
	return 1
}

func newJSONLogger(output io.Writer, level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(output, &slog.HandlerOptions{Level: level}))
}

func printUsage(output io.Writer) {
	fmt.Fprintln(output, `Usage: pandatable <command> [flags]

Commands:
  dev seed              Create an explicitly confirmed disposable developer fixture (T1.20)
  serve                 Validate configuration and start the HTTP server (T0.4)
  migrate               Validate configuration and run database migrations (T0.3)
  healthcheck           Probe HTTP /healthz without database configuration (T0.6)
  restore               Restore a trusted backup with --yes
  doctor                Diagnose the installation (not yet scheduled)
  reset-admin-password  Reset an administrator password (T3.10)
  version               Print the application version
  help                  Show this help`)
}

func printCommandUsage(output io.Writer, command string) {
	fmt.Fprintf(output, "Usage: pandatable %s [flags]\n\nFlags:\n", command)
	fmt.Fprintln(output, `  --database-url          PostgreSQL connection URL (PT_DATABASE_URL)
  --data-dir              application data directory (PT_DATA_DIR)
  --listen                listen address (PT_LISTEN)
  --base-url              public application URL (PT_BASE_URL)
  --trusted-proxies       comma-separated trusted proxy CIDRs (PT_TRUSTED_PROXIES)
  --db-max-conns           maximum database connections (PT_DB_MAX_CONNS)
  --log-level              debug, info, warn, or error (PT_LOG_LEVEL)
  --db-connect-retries    database startup retries (PT_DB_CONNECT_RETRIES)`)
}
