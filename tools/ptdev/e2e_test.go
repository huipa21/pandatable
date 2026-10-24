package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestE2EArguments(t *testing.T) {
	opts, err := parseArgs([]string{"e2e", "--", "shell.spec.ts", "--project=desktop-chromium", "--grep", "theme"})
	want := []string{"shell.spec.ts", "--project=desktop-chromium", "--grep", "theme"}
	if err != nil || !reflect.DeepEqual(opts.e2eArgs, want) {
		t.Fatalf("Playwright arguments: %+v, %v", opts, err)
	}
	args := e2eComposeArgs(filepath.Join("repo with spaces"), "empty.env", "pt-e2e-owned", "down", "--volumes")
	joined := strings.Join(args, "|")
	for _, want := range []string{"--env-file|empty.env|-p|pt-e2e-owned", filepath.Join("deploy", "compose.yaml"), filepath.Join("deploy", "compose.local.yaml"), "down|--volumes"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("unsafe/incomplete Compose command: %v", args)
		}
	}
	if strings.Contains(strings.Join(imageBuildArgs("podman", "test", "test", false), "|"), ":latest") {
		t.Fatal("disposable build must not overwrite the shared latest tag")
	}
}

func TestE2ELifecycle(t *testing.T) {
	for _, mode := range []string{"success", "build", "up", "ready", "test", "cancel-build", "cancel-up", "cancel-test", "down", "diagnostics", "remove-image"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var events []string
			step := func(name string) func(context.Context) error {
				return func(stepCtx context.Context) error {
					events = append(events, name)
					if name == "down" || name == "diagnostics" || name == "remove-image" {
						if stepCtx.Err() != nil {
							t.Fatal("cleanup/diagnostics inherited canceled context")
						}
						if _, ok := stepCtx.Deadline(); !ok {
							t.Fatal("cleanup/diagnostics must be bounded")
						}
					}
					if mode == "cancel-"+name {
						cancel()
						return stepCtx.Err()
					}
					if mode == name || (mode == "diagnostics" && name == "test") {
						return fmt.Errorf("failed %s", name)
					}
					return nil
				}
			}
			err := runE2E(ctx, e2eSteps{step("build"), step("up"), step("ready"), step("test"), step("diagnostics"), step("down"), step("remove-image")})
			if (err == nil) != (mode == "success") {
				t.Fatalf("lost failure/cancellation: %v", err)
			}
			if mode == "build" || mode == "cancel-build" {
				if !reflect.DeepEqual(events, []string{"build", "remove-image"}) {
					t.Fatalf("resources started after build failure: %v", events)
				}
			} else if !reflect.DeepEqual(events[len(events)-2:], []string{"down", "remove-image"}) {
				t.Fatalf("owned resources not cleaned after %s: %v", mode, events)
			}
			if mode == "up" && strings.Contains(strings.Join(events, ","), "ready") {
				t.Fatal("probed/tested after startup failure")
			}
			if mode == "ready" && strings.Contains(strings.Join(events, ","), "test") {
				t.Fatal("ran browser tests after readiness failure")
			}
			if strings.HasPrefix(mode, "cancel-") && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation not surfaced: %v", err)
			}
			if mode == "diagnostics" && (!strings.Contains(err.Error(), "failed test") || !strings.Contains(err.Error(), "failed diagnostics")) {
				t.Fatalf("diagnostics replaced original failure: %v", err)
			}
		})
	}
}

func TestE2EHealth(t *testing.T) {
	for _, mode := range []string{"healthy", "503", "redirect", "wrong body"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/healthz" {
					t.Error("followed redirect or wrong health path")
				}
				switch mode {
				case "503":
					w.WriteHeader(503)
				case "redirect":
					w.Header().Set("Location", "/other")
					w.WriteHeader(302)
				case "healthy":
					fmt.Fprintln(w, `{"status":"ok"}`)
				default:
					fmt.Fprintln(w, "<html>not health</html>")
				}
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			start := time.Now()
			err := waitE2EHealth(ctx, server.URL)
			if (err == nil) != (mode == "healthy") || time.Since(start) > time.Second {
				t.Fatalf("health %s: %v, duration=%s", mode, err, time.Since(start))
			}
		})
	}
}

func TestCooperativeProcessHelper(t *testing.T) {
	if marker := os.Getenv("PTDEV_EOF_MARKER"); marker != "" {
		if _, err := io.Copy(io.Discard, os.Stdin); err != nil {
			os.Exit(6)
		}
		if err := os.WriteFile(marker, []byte("gracefully stopped"), 0o600); err != nil {
			os.Exit(7)
		}
		os.Exit(0)
	}
}

func TestCooperativeCancellation(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "stopped")
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	command := (runner{}).command(ctx, "", map[string]string{"PTDEV_EOF_MARKER": marker}, os.Args[0], "-test.run=^TestCooperativeProcessHelper$")
	start := time.Now()
	if err := runCooperative(command); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancellation not reported: %v", err)
	}
	if command.ProcessState == nil || !command.ProcessState.Success() || time.Since(start) > 3*time.Second {
		t.Fatalf("child not gracefully stopped/reaped: %v", command.ProcessState)
	}
	if body, err := os.ReadFile(marker); err != nil || string(body) != "gracefully stopped" {
		t.Fatalf("EOF did not trigger cooperative shutdown: %q, %v", body, err)
	}
}
