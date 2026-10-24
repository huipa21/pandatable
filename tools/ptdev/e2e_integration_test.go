package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestE2EFailureAndCancellationIntegration(t *testing.T) {
	if os.Getenv("PTDEV_TEST_E2E") != "1" {
		t.Skip("set PTDEV_TEST_E2E=1 to build disposable images/stacks and verify browser failure/cancellation cleanup")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	selected, err := detectEngine(os.Getenv("PT_ENGINE"), exec.LookPath)
	if err != nil {
		t.Fatal(err)
	}
	runner := runner{root: root}
	resultsRoot := filepath.Join(root, "web", "test-results")
	if err := os.MkdirAll(resultsRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"failure", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			marker := filepath.Join(t.TempDir(), "browser-ready")
			t.Setenv("PTDEV_E2E_MARKER", marker)
			t.Setenv("PTDEV_E2E_MODE", mode)
			configDir, err := os.MkdirTemp(resultsRoot, "lifecycle-config-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := os.RemoveAll(configDir); err != nil {
					t.Error(err)
				}
			})
			configFile := filepath.Join(configDir, "playwright.config.ts")
			body := fmt.Sprintf("import config from '../../playwright.config'\nexport default { ...config, testDir: %q, testMatch: '**/lifecycle.probe.ts' }\n", filepath.Join(root, "web", "e2e"))
			if err := os.WriteFile(configFile, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			done := make(chan error, 1)
			go func() {
				done <- runner.e2e(ctx, options{e2eArgs: []string{"--config", configFile, "--project=desktop-chromium"}})
			}()
			// Always cancel and wait before test cleanup removes its configuration.
			t.Cleanup(func() {
				cancel()
				select {
				case <-done:
				case <-time.After(110 * time.Second):
					t.Error("E2E did not finish owned-resource cleanup")
				}
			})
			var evidence struct {
				Results string `json:"results"`
				URL     string `json:"url"`
				PIDs    []int  `json:"pids"`
			}
			for {
				data, readErr := os.ReadFile(marker)
				if readErr == nil {
					if err := json.Unmarshal(data, &evidence); err != nil {
						t.Fatal(err)
					}
					break
				}
				if !errors.Is(readErr, os.ErrNotExist) {
					t.Fatal(readErr)
				}
				select {
				case err := <-done:
					done <- err
					t.Fatalf("E2E exited before opening the real browser: %v", err)
				case <-ctx.Done():
					t.Fatal("E2E browser probe never became ready")
				case <-time.After(100 * time.Millisecond):
				}
			}
			if mode == "cancel" {
				cancel()
			}
			select {
			case err := <-done:
				done <- err
				if err == nil {
					t.Fatal("E2E swallowed browser failure/cancellation")
				}
				if mode == "cancel" && !errors.Is(err, context.Canceled) {
					t.Fatalf("cancellation cause was not preserved: %v", err)
				}
			case <-time.After(110 * time.Second):
				t.Fatal("E2E failure/cancellation cleanup timed out")
			}
			project := filepath.Base(evidence.Results)
			assertE2EReaped(t, runner, selected.name, project)
			if !exists(filepath.Join(root, "web", "test-results", project, "compose.log")) {
				t.Fatal("failure diagnostics were not retained")
			}
			if len(evidence.PIDs) < 3 {
				t.Fatal("browser/worker process evidence missing")
			}
			for _, pid := range evidence.PIDs {
				process, err := os.FindProcess(pid)
				if err != nil {
					continue
				}
				alive := runtime.GOOS == "windows" || process.Signal(syscall.Signal(0)) == nil
				_ = process.Release()
				if alive {
					t.Errorf("owned browser/worker/runner process %d remains", pid)
				}
			}
			parsed, err := url.Parse(evidence.URL)
			if err != nil {
				t.Fatal(err)
			}
			connection, err := net.DialTimeout("tcp", parsed.Host, time.Second)
			if err == nil {
				connection.Close()
				t.Fatal("E2E HTTP listener remains after cleanup")
			}
			t.Logf("%s: real browser reached %s; failure reported; owned stack/image and listener removed", mode, evidence.URL)
		})
	}
}

func TestE2EStartupFailureIntegration(t *testing.T) {
	if os.Getenv("PTDEV_TEST_E2E") != "1" {
		t.Skip("set PTDEV_TEST_E2E=1 to verify real partial Compose-startup cleanup")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	selected, err := detectEngine(os.Getenv("PT_ENGINE"), exec.LookPath)
	if err != nil {
		t.Fatal(err)
	}
	runner := runner{root: root}
	project := "pt-e2e-" + strings.ToLower(rand.Text())
	envFile := filepath.Join(t.TempDir(), "empty.env")
	if err := os.WriteFile(envFile, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	occupied, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	_, port, _ := net.SplitHostPort(occupied.Addr().String())
	env := map[string]string{
		"COMPOSE_PROJECT_NAME": project, "PT_VERSION": "latest",
		"PT_DB_PASSWORD": rand.Text(), "PT_LOCAL_HTTP_PORT": port,
		"PT_BASE_URL": "http://127.0.0.1:" + port,
	}
	compose := func(ctx context.Context, args ...string) error {
		_, err := runner.execute(ctx, "", env, false, selected.name, e2eComposeArgs(root, envFile, project, args...)...)
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var reachedBrowser bool
	err = runE2E(ctx, e2eSteps{
		build: func(ctx context.Context) error {
			return runner.run(ctx, selected.name, "image", "inspect", "--format", "{{.Id}}", "localhost/pandatable:latest")
		},
		up: func(ctx context.Context) error { return compose(ctx, "up", "-d") },
		ready: func(ctx context.Context) error {
			return waitE2EHealth(ctx, env["PT_BASE_URL"])
		},
		test: func(context.Context) error { reachedBrowser = true; return nil },
		diagnostics: func(ctx context.Context) error {
			return compose(ctx, "logs", "--no-color", "--tail", "20")
		},
		down: func(ctx context.Context) error {
			return compose(ctx, "down", "--volumes", "--remove-orphans", "--timeout", "10")
		},
		// This test reuses an existing image; it does not own its tag.
		removeImage: func(context.Context) error { return nil },
	})
	if err == nil || reachedBrowser || !strings.Contains(err.Error(), "Compose startup") {
		t.Fatalf("occupied-port startup did not fail before tests: %v", err)
	}
	assertE2EReaped(t, runner, selected.name, project)
	t.Log("real Compose startup failed on an occupied loopback port; partial stack/volumes/networks removed")
}

func assertE2EReaped(t *testing.T, runner runner, engine, project string) {
	t.Helper()
	if !strings.HasPrefix(project, "pt-e2e-") {
		t.Fatal("invalid owned project")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, args := range [][]string{
		{"ps", "-a", "--filter", "label=com.docker.compose.project=" + project, "--format", "{{.Names}}"},
		{"volume", "ls", "--filter", "label=com.docker.compose.project=" + project, "--format", "{{.Name}}"},
		{"network", "ls", "--filter", "label=com.docker.compose.project=" + project, "--format", "{{.Name}}"},
		{"image", "ls", "localhost/pandatable:0.0.0-" + project, "--format", "{{.Id}}"},
	} {
		output, err := runner.execute(ctx, "", nil, true, engine, args...)
		if err != nil || strings.TrimSpace(output) != "" {
			t.Errorf("owned resource remains: %s: %s, %v", fmt.Sprint(args), output, err)
		}
	}
}
