package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5"

	"pandatable/internal/acl"
	"pandatable/internal/apperr"
	"pandatable/internal/config"
	"pandatable/internal/db"
	"pandatable/internal/devseed"
)

const seedUsage = `Usage: pandatable dev seed --rows 200000 --confirm-database NAME --owner-id UUID --yes-dev-seed

Requires PT_DATABASE_URL (or --database-url), a migrated PG18 pt_seed_*/pt_test_*
database with no workspaces, and an existing active, non-forced-change owner.
No database/account creation, reset, resume or automatic cleanup.
Reruns refuse occupied targets. Failure JSON records confirmed and attempted rows;
an interrupted commit may be uncertain. Drop only the recorded disposable database.
Limits: 1..200000 rows; 300 rows/batch, 10s/batch, 10min overall.`

type seedOptions struct {
	databaseURL, database, owner string
	rows                         int
	yes                          bool
}

func parseSeed(args []string) (seedOptions, error) {
	opts := seedOptions{databaseURL: os.Getenv("PT_DATABASE_URL")}
	flags := flag.NewFlagSet("dev seed", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&opts.databaseURL, "database-url", opts.databaseURL, "")
	flags.StringVar(&opts.database, "confirm-database", "", "")
	flags.StringVar(&opts.owner, "owner-id", "", "")
	flags.IntVar(&opts.rows, "rows", devseed.MaxRows, "")
	flags.BoolVar(&opts.yes, "yes-dev-seed", false, "")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return opts, err
		}
		return opts, errors.New("invalid developer seed flags (use --help)")
	}
	if len(flags.Args()) != 0 || !opts.yes || !devseed.ValidDatabase(opts.database) ||
		opts.rows < 1 || opts.rows > devseed.MaxRows {
		return opts, errors.New("seed requires --yes-dev-seed, --confirm-database pt_seed_*/pt_test_* and 1..200000 rows")
	}
	if opts.databaseURL == "" {
		return opts, errors.New("seed requires an explicit PostgreSQL URL via --database-url or PT_DATABASE_URL")
	}
	parsed, parseErr := url.Parse(opts.databaseURL)
	conn, err := pgx.ParseConfig(opts.databaseURL)
	if parseErr != nil || parsed == nil || parsed.Hostname() == "" ||
		(parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") ||
		err != nil || conn.Database != opts.database {
		return opts, errors.New("seed connection must name exactly --confirm-database")
	}
	if _, ok := acl.ParseUUID(opts.owner); !ok {
		return opts, errors.New("seed requires --owner-id UUID")
	}
	return opts, nil
}

func runDev(args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(stdout, seedUsage)
		return 0
	}
	if len(args) == 0 || args[0] != "seed" {
		fmt.Fprintln(stderr, "pandatable: dev supports only seed (use dev seed --help)")
		return 2
	}
	opts, err := parseSeed(args[1:])
	if errors.Is(err, flag.ErrHelp) {
		fmt.Fprintln(stdout, seedUsage)
		return 0
	}
	if err != nil {
		fmt.Fprintf(stderr, "pandatable: %v\n", err)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, devseed.Timeout)
	defer cancel()
	pool, err := db.Open(ctx, config.Config{DatabaseURL: opts.databaseURL, DBMaxConns: 3})
	result := devseed.Manifest{Database: opts.database, RequestedRows: opts.rows, Stage: "connect"}
	if err == nil {
		defer pool.Close()
		result, err = devseed.Run(ctx, pool, opts.database, opts.owner, opts.rows)
	}
	message := ""
	if err != nil {
		message = "Seed failed; inspect the recorded disposable database. No reset or retry was performed."
		var app *apperr.Error
		var refusal devseed.Refusal
		switch {
		case errors.Is(err, context.Canceled):
			message = "Seed canceled; committed fixture remains. An attempted commit may be uncertain."
		case errors.Is(err, context.DeadlineExceeded):
			message = "Seed deadline exceeded; committed fixture remains. An attempted commit may be uncertain."
		case errors.As(err, &app):
			message += " " + app.Code
		case errors.As(err, &refusal):
			message += " " + refusal.Error()
		}
	}
	if encodeErr := json.NewEncoder(stdout).Encode(struct {
		devseed.Manifest
		Error string `json:"error,omitempty"`
	}{result, message}); encodeErr != nil {
		fmt.Fprintln(stderr, "pandatable: cannot write developer seed result")
		return 1
	}
	if err != nil {
		return 1
	}
	return 0
}
