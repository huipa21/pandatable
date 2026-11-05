package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type usageError struct{ message string }

func (err usageError) Error() string { return err.message }

type engine struct {
	name   string
	reason string
}

type engineCommand func(context.Context, string, ...string) (string, error)

func (runner runner) engineCommand(ctx context.Context, name string, args ...string) (string, error) {
	return runner.execute(ctx, "", nil, true, name, args...)
}

func (runner runner) readyEngine(ctx context.Context) (engine, error) {
	selected, err := detectEngine(os.Getenv("PT_ENGINE"), exec.LookPath)
	if err != nil {
		return engine{}, err
	}
	if err := ensureEngine(ctx, selected.name, runtime.GOOS, runner.engineCommand); err != nil {
		return engine{}, err
	}
	return selected, nil
}

func engineInfo(ctx context.Context, name string, command engineCommand) error {
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	output, err := command(probeCtx, name, "info")
	if err != nil {
		return fmt.Errorf("%s info: %w: %s", name, err, strings.TrimSpace(output))
	}
	return nil
}

func ensureEngine(ctx context.Context, name, goos string, command engineCommand) error {
	waitCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if err := engineInfo(waitCtx, name, command); err == nil {
		return nil
	} else if waitCtx.Err() != nil {
		return fmt.Errorf("%s readiness: %w", name, waitCtx.Err())
	} else {
		fmt.Fprintf(os.Stderr, "%s is not ready: %v\nStarting it automatically...\n", name, err)
	}
	if err := startEngine(waitCtx, name, goos, command); err != nil {
		return err
	}
	for {
		err := engineInfo(waitCtx, name, command)
		if err == nil {
			return nil
		}
		select {
		case <-waitCtx.Done():
			return fmt.Errorf("%s readiness (2m limit): %w; last probe: %v", name, waitCtx.Err(), err)
		case <-time.After(time.Second):
		}
	}
}

func startEngine(ctx context.Context, name, goos string, command engineCommand) error {
	run := func(executable string, args ...string) error {
		output, err := command(ctx, executable, args...)
		if err != nil {
			return fmt.Errorf("start %s with %s %s: %w: %s", name, executable, strings.Join(args, " "), err, strings.TrimSpace(output))
		}
		return nil
	}
	if name == "podman" {
		if goos != "windows" && goos != "darwin" {
			return fmt.Errorf("local Podman is daemonless; podman info failed: check Podman configuration and any remote connection")
		}
		output, err := command(ctx, name, "machine", "list", "--format", "json")
		if err != nil {
			return fmt.Errorf("list Podman machines: %w: %s", err, strings.TrimSpace(output))
		}
		var machines []struct {
			Name     string
			Default  bool
			Running  bool
			Starting bool
		}
		if err := json.Unmarshal([]byte(output), &machines); err != nil {
			return fmt.Errorf("decode Podman machines: %w", err)
		}
		if len(machines) == 0 {
			if err := run(name, "machine", "init"); err != nil {
				return err
			}
			return run(name, "machine", "start")
		}
		for _, machine := range machines {
			if machine.Default || len(machines) == 1 {
				if machine.Running || machine.Starting {
					return nil
				}
				return run(name, "machine", "start", machine.Name)
			}
		}
		return fmt.Errorf("no default Podman machine; select a default connection before retrying")
	}
	if goos == "windows" || goos == "darwin" {
		return run(name, "desktop", "start")
	}
	return run("systemctl", "start", "docker")
}

func detectEngine(override string, lookPath func(string) (string, error)) (engine, error) {
	if override != "" {
		if override != "podman" && override != "docker" {
			return engine{}, usageError{"PT_ENGINE must be podman or docker"}
		}
		if _, err := lookPath(override); err != nil {
			return engine{}, fmt.Errorf("PT_ENGINE=%s: executable not found on PATH: %w", override, err)
		}
		return engine{override, "PT_ENGINE override"}, nil
	}
	for _, name := range []string{"podman", "docker"} {
		if _, err := lookPath(name); err == nil {
			return engine{name, "auto-detected on PATH (podman preferred)"}, nil
		}
	}
	return engine{}, fmt.Errorf("no container engine found: install podman or docker, or set PT_ENGINE")
}
