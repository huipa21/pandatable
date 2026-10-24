package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type runner struct {
	root string
}

func (runner runner) execute(ctx context.Context, dir string, env map[string]string, capture bool, name string, args ...string) (string, error) {
	command := runner.command(ctx, dir, env, name, args...)
	var output bytes.Buffer
	if capture {
		command.Stdout = &output
		command.Stderr = &output
	}
	if err := command.Run(); err != nil {
		return output.String(), fmt.Errorf("%s: %w", name, err)
	}
	return output.String(), nil
}

func (runner runner) command(ctx context.Context, dir string, env map[string]string, name string, args ...string) *exec.Cmd {
	parts := []string{name}
	for _, arg := range args {
		if strings.ContainsAny(arg, " \t\n\"") {
			arg = strconv.Quote(arg)
		}
		parts = append(parts, arg)
	}
	fmt.Fprintln(os.Stderr, "+", strings.Join(parts, " "))
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = filepath.Join(runner.root, dir)
	command.Env = mergeEnv(os.Environ(), env)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Cancel = func() error {
		if err := command.Process.Signal(os.Interrupt); err != nil {
			return command.Process.Kill()
		}
		return nil
	}
	command.WaitDelay = 3 * time.Second
	return command
}

func (runner runner) run(ctx context.Context, name string, args ...string) error {
	_, err := runner.execute(ctx, "", nil, false, name, args...)
	return err
}

func mergeEnv(base []string, overrides map[string]string) []string {
	result := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		key, _, _ := strings.Cut(entry, "=")
		replaced := false
		for override := range overrides {
			if strings.EqualFold(key, override) {
				replaced = true
				break
			}
		}
		if !replaced {
			result = append(result, entry)
		}
	}
	for key, value := range overrides {
		result = append(result, key+"="+value)
	}
	return result
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (runner runner) webExists() bool {
	return exists(filepath.Join(runner.root, "web", "package.json"))
}

func (runner runner) pnpm(ctx context.Context, args ...string) error {
	path, err := exec.LookPath("pnpm")
	if err != nil {
		return fmt.Errorf("web requires pnpm on PATH: %w", err)
	}
	name := path
	if strings.EqualFold(filepath.Ext(path), ".cmd") || strings.EqualFold(filepath.Ext(path), ".bat") {
		found := false
		for _, script := range []string{
			filepath.Join(filepath.Dir(path), "node_modules", "pnpm", "bin", "pnpm.cjs"),
			filepath.Join(filepath.Dir(path), "node_modules", "corepack", "dist", "pnpm.js"),
		} {
			if exists(script) {
				name = "node"
				args = append([]string{script}, args...)
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("cannot launch pnpm shim without a shell: install standalone pnpm or npm/corepack pnpm")
		}
	}
	_, err = runner.execute(ctx, "web", nil, false, name, args...)
	return err
}

func copyFile(source, target string) (err error) {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, input.Close()) }()
	output, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, output.Close()) }()
	_, err = io.Copy(output, input)
	return err
}
