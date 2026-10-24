package main

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
)

func binaryName(goos string, linux bool) string {
	if linux {
		return "pandatable-linux-amd64"
	}
	if goos == "windows" {
		return "pandatable.exe"
	}
	return "pandatable"
}

func (runner runner) build(ctx context.Context, linux bool) error {
	if runner.webExists() {
		if err := runner.buildWeb(ctx); err != nil {
			return err
		}
	} else {
		fmt.Println("web: skipped (no web/ yet)")
	}
	if err := os.MkdirAll(filepath.Join(runner.root, "bin"), 0o750); err != nil {
		return err
	}
	var env map[string]string
	if linux {
		env = map[string]string{"GOOS": "linux", "GOARCH": "amd64", "CGO_ENABLED": "0"}
	}
	_, err := runner.execute(ctx, "", env, false, "go", "build", "-trimpath", "-ldflags", "-s -w -X main.version="+runner.version(ctx), "-o", filepath.Join("bin", binaryName(runtime.GOOS, linux)), "./cmd/pandatable")
	return err
}

func (runner runner) buildWeb(ctx context.Context) error {
	if !runner.webExists() {
		return fmt.Errorf("web/package.json is missing")
	}
	if err := runner.pnpm(ctx, "install", "--frozen-lockfile"); err != nil {
		return err
	}
	if err := runner.pnpm(ctx, "build"); err != nil {
		return err
	}
	if err := runner.copyWeb(); err != nil {
		return fmt.Errorf("copy web dist: %w", err)
	}
	return nil
}

func (runner runner) copyWeb() error {
	source := filepath.Join(runner.root, "web", "dist")
	target := filepath.Join(runner.root, "internal", "web", "dist")
	index, err := os.ReadFile(filepath.Join(source, "index.html"))
	if err != nil {
		return fmt.Errorf("read built index: %w", err)
	}
	if len(bytes.TrimSpace(index)) == 0 {
		return fmt.Errorf("built index.html is empty")
	}
	// Validate before removing old assets; retain the tracked embed placeholder.
	if err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && !entry.Type().IsRegular() {
			return fmt.Errorf("unsupported web asset: %s", path)
		}
		return nil
	}); err != nil {
		return err
	}
	if err := os.MkdirAll(target, 0o750); err != nil {
		return err
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() != ".gitkeep" {
			if err := os.RemoveAll(filepath.Join(target, entry.Name())); err != nil {
				return err
			}
		}
	}
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o750)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("unsupported web asset: %s", path)
		}
		return copyFile(path, destination)
	})
}
