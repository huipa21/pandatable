package main

import (
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
		if err := runner.pnpm(ctx, "install", "--frozen-lockfile"); err != nil {
			return err
		}
		if err := runner.pnpm(ctx, "build"); err != nil {
			return err
		}
		if err := runner.copyWeb(); err != nil {
			return fmt.Errorf("copy web dist: %w", err)
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

func (runner runner) copyWeb() error {
	source := filepath.Join(runner.root, "web", "dist")
	target := filepath.Join(runner.root, "internal", "web", "dist")
	if err := os.RemoveAll(target); err != nil {
		return err
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
