package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"pandatable/internal/config"
)

func devConfig() (config.Config, error) {
	defaults := []struct{ env, flag, value string }{
		{"PT_DATABASE_URL", "--database-url", dbDSN},
		{"PT_LISTEN", "--listen", "127.0.0.1:8080"},
		{"PT_BASE_URL", "--base-url", "http://127.0.0.1:5173"},
	}
	var args []string
	for _, item := range defaults {
		if _, ok := os.LookupEnv(item.env); !ok {
			args = append(args, item.flag, item.value)
		}
	}
	return config.Parse(args)
}

func apiTarget(listen string) (string, error) {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return "", fmt.Errorf("dev backend listen address: %w", err)
	}
	if port == "0" {
		return "", fmt.Errorf("dev backend requires a fixed PT_LISTEN port")
	}
	switch host {
	case "", "0.0.0.0":
		host = "127.0.0.1"
	case "::":
		host = "::1"
	}
	return "http://" + net.JoinHostPort(host, port), nil
}

func (runner runner) dev(ctx context.Context) (err error) {
	vite := filepath.Join(runner.root, "web", "node_modules", "vite", "bin", "vite.js")
	if !exists(vite) {
		return fmt.Errorf("Vite is missing; run pnpm install --frozen-lockfile in web first")
	}
	cfg, err := devConfig()
	if err != nil {
		return fmt.Errorf("dev config: %w", err)
	}
	target, err := apiTarget(cfg.Listen)
	if err != nil {
		return err
	}
	if err := devPortAvailable("backend (PT_LISTEN)", cfg.Listen); err != nil {
		return err
	}
	if err := devPortAvailable("Vite", "127.0.0.1:5173"); err != nil {
		return err
	}
	if err := bindChildrenToProcess(); err != nil {
		return err
	}
	if _, ok := os.LookupEnv("PT_DATABASE_URL"); !ok {
		if err := runner.db(ctx, "up", false); err != nil {
			return err
		}
	}
	dir, err := os.MkdirTemp("", "ptdev-")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, removeDevDir(dir)) }()
	binary := filepath.Join(dir, binaryName(runtime.GOOS, false))
	// Do not supervise "go run" or "pnpm dev": their grandchildren can outlive them.
	if err := runner.run(ctx, "go", "build", "-o", binary, "./cmd/pandatable"); err != nil {
		return err
	}
	env := map[string]string{
		"PT_DATABASE_URL": cfg.DatabaseURL,
		"PT_LISTEN":       cfg.Listen,
		"PT_BASE_URL":     cfg.BaseURL,
	}
	fmt.Printf("Dev UI: %s; API proxy: %s (WebSocket-ready)\n", cfg.BaseURL, target)
	return supervise(ctx,
		func(ctx context.Context) *exec.Cmd {
			command := runner.command(ctx, "", env, binary, "serve")
			command.WaitDelay = 12 * time.Second
			return command
		},
		func(ctx context.Context) *exec.Cmd {
			return runner.command(ctx, "web", map[string]string{
				"PT_DEV_API_TARGET": target,
				"PT_BASE_URL":       cfg.BaseURL,
			}, "node", vite)
		},
	)
}

func devPortAvailable(name, address string) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("%s address %s is unavailable: %w; stop the existing listener before restarting ptdev dev", name, address, err)
	}
	if err := listener.Close(); err != nil {
		return fmt.Errorf("close %s port probe: %w", name, err)
	}
	return nil
}

// Every launched process is a direct child and is reaped before returning.
func supervise(ctx context.Context, commands ...func(context.Context) *exec.Cmd) error {
	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	type result struct {
		name string
		err  error
	}
	results := make(chan result, len(commands))
	started := 0
	var failure error
	recordStop := func(result result) {
		var exit *exec.ExitError
		if result.err != nil && !errors.As(result.err, &exit) &&
			!errors.Is(result.err, context.Canceled) && !errors.Is(result.err, context.DeadlineExceeded) {
			failure = errors.Join(failure, fmt.Errorf("stop %s: %w", result.name, result.err))
		}
	}
	for _, makeCommand := range commands {
		command := makeCommand(childCtx)
		if err := command.Start(); err != nil {
			failure = fmt.Errorf("start %s: %w", command.Path, err)
			break
		}
		started++
		go func() { results <- result{command.Path, command.Wait()} }()
	}
	if failure == nil && started > 0 {
		select {
		case <-ctx.Done():
		case result := <-results:
			started--
			if ctx.Err() != nil {
				recordStop(result)
			} else if result.err == nil {
				failure = fmt.Errorf("%s exited unexpectedly", result.name)
			} else {
				failure = fmt.Errorf("%s exited: %w", result.name, result.err)
			}
		}
	}
	cancel()
	for range started {
		recordStop(<-results)
	}
	return failure
}
