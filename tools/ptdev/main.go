package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:])
	stop()
	os.Exit(code)
}

func run(ctx context.Context, args []string) int {
	opts, err := parseArgs(args)
	if err == nil {
		var root string
		root, err = os.Getwd()
		if err == nil {
			runner := runner{root: root}
			err = runner.dispatch(ctx, opts)
		}
	}
	if err == nil {
		return 0
	}
	fmt.Fprintln(os.Stderr, "ptdev:", err)
	var usage usageError
	if errors.As(err, &usage) {
		return 2
	}
	return 1
}

func (runner runner) dispatch(ctx context.Context, opts options) error {
	if override := os.Getenv("PT_ENGINE"); override != "" && override != "podman" && override != "docker" {
		return usageError{"PT_ENGINE must be podman or docker"}
	}
	switch opts.command {
	case "help":
		fmt.Println(`Usage: go run ./tools/ptdev <command> [flags]

  help                 List commands and detected engine (default)
  version              Print the Git-derived version
  engine               Show the engine and selection reason
  db up                Start PostgreSQL 18 and wait for readiness
  db down              Stop PostgreSQL, retaining data
  db reset [--yes]      Delete dev data and recreate PostgreSQL
  db status            Show container state and DSN
  db psql              Open interactive psql
  build [--linux]      Build web assets (if present) and Go binary
  lint                 Check formatting, vet, optional golangci-lint/web lint
  test [--no-db]       Run tests; forward -run, -v, -count
  check                Run lint, test, build and print a summary
  dev                  Start backend and Vite; stop both on exit or Ctrl+C
  web build            Build frontend and copy embedded assets
  e2e                  Acceptance tests (T0.7)
  image build [--version VERSION]
                       Build linux/amd64 release image + latest
  image save [--version VERSION] [--with-postgres]
                       Export gzip Docker archive(s) to dist
  deploy --host user@server --yes [--dir /opt/pandatable]
         [--version VERSION] [--with-postgres]
                       Transfer existing archives, load, update .env, start app
                       Requires pre-provisioned server files/network; never sends secrets`)
		selected, err := detectEngine(os.Getenv("PT_ENGINE"), exec.LookPath)
		if err != nil {
			fmt.Println("\nEngine: unavailable (" + err.Error() + ")")
		} else {
			fmt.Printf("\nEngine: %s (%s)\n", selected.name, selected.reason)
		}
		return nil
	case "engine":
		selected, err := detectEngine(os.Getenv("PT_ENGINE"), exec.LookPath)
		if err == nil {
			fmt.Printf("%s (%s)\n", selected.name, selected.reason)
		}
		return err
	case "version":
		fmt.Println(runner.version(ctx))
		return ctx.Err()
	case "build":
		return runner.build(ctx, opts.linux)
	case "lint":
		return runner.lint(ctx)
	case "test":
		return runner.test(ctx, opts)
	case "check":
		return runner.check(ctx)
	case "dev":
		return runner.dev(ctx)
	case "web build":
		return runner.buildWeb(ctx)
	case "e2e":
		return usageError{"not implemented yet (T0.7)"}
	case "image build", "image save":
		return runner.image(ctx, opts)
	case "deploy":
		return runner.deploy(ctx, opts)
	default:
		return runner.db(ctx, strings.TrimPrefix(opts.command, "db "), opts.yes)
	}
}

func (runner runner) check(ctx context.Context) error {
	start := time.Now()
	type result struct {
		name     string
		duration time.Duration
		err      error
	}
	var results []result
	var failures []error
	for _, step := range []struct {
		name string
		run  func(context.Context) error
	}{
		{"lint", runner.lint},
		{"test", func(ctx context.Context) error { return runner.test(ctx, options{}) }},
		{"build", func(ctx context.Context) error { return runner.build(ctx, false) }},
	} {
		stepStart := time.Now()
		err := step.run(ctx)
		results = append(results, result{step.name, time.Since(stepStart), err})
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", step.name, err)
			failures = append(failures, fmt.Errorf("%s: %w", step.name, err))
		}
		if ctx.Err() != nil {
			break
		}
	}
	fmt.Printf("\n%-10s %-6s %s\n", "STEP", "RESULT", "DURATION")
	for _, result := range results {
		status := "PASS"
		if result.err != nil {
			status = "FAIL"
		}
		fmt.Printf("%-10s %-6s %s\n", result.name, status, result.duration.Round(time.Millisecond))
	}
	fmt.Printf("Total: %s\n", time.Since(start).Round(time.Millisecond))
	return errors.Join(failures...)
}
