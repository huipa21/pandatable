package main

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestAPITarget(t *testing.T) {
	for listen, want := range map[string]string{
		":8080": "http://127.0.0.1:8080", "0.0.0.0:9090": "http://127.0.0.1:9090",
		"[::]:8080": "http://[::1]:8080", "localhost:8081": "http://localhost:8081",
	} {
		got, err := apiTarget(listen)
		if err != nil || got != want {
			t.Errorf("apiTarget(%q) = %q, %v", listen, got, err)
		}
	}
	for _, listen := range []string{"invalid", "127.0.0.1:0"} {
		if _, err := apiTarget(listen); err == nil {
			t.Errorf("apiTarget(%q) accepted", listen)
		}
	}
}

func TestDevConfig(t *testing.T) {
	for _, key := range []string{"PT_DATABASE_URL", "PT_LISTEN", "PT_BASE_URL"} {
		previous, ok := os.LookupEnv(key)
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if ok {
				if err := os.Setenv(key, previous); err != nil {
					t.Error(err)
				}
			} else if err := os.Unsetenv(key); err != nil {
				t.Error(err)
			}
		})
	}
	cfg, err := devConfig()
	if err != nil || cfg.DatabaseURL != dbDSN || cfg.Listen != "127.0.0.1:8080" || cfg.BaseURL != "http://127.0.0.1:5173" {
		t.Fatalf("defaults: %+v, %v", cfg, err)
	}
	t.Setenv("PT_LISTEN", "127.0.0.1:8081")
	t.Setenv("PT_BASE_URL", "http://localhost:5173")
	cfg, err = devConfig()
	if err != nil || cfg.Listen != "127.0.0.1:8081" || cfg.BaseURL != "http://localhost:5173" {
		t.Fatalf("overrides: %+v, %v", cfg, err)
	}
	t.Setenv("PT_DATABASE_URL", "")
	if _, err := devConfig(); err == nil {
		t.Fatal("explicit empty database URL must fail")
	}
}

func TestDevProcessHelper(t *testing.T) {
	switch os.Getenv("PTDEV_PROCESS_TEST") {
	case "wait":
		for {
			time.Sleep(time.Second)
		}
	case "fail":
		time.Sleep(100 * time.Millisecond)
		os.Exit(7)
	case "success":
		os.Exit(0)
	}
}

func TestSupervise(t *testing.T) {
	for _, mode := range []string{"cancel", "fail", "success", "start-error"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var children []*exec.Cmd
			child := func(mode string) func(context.Context) *exec.Cmd {
				return func(ctx context.Context) *exec.Cmd {
					command := (runner{}).command(ctx, "", map[string]string{"PTDEV_PROCESS_TEST": mode}, os.Args[0], "-test.run=^TestDevProcessHelper$")
					children = append(children, command)
					return command
				}
			}
			commands := []func(context.Context) *exec.Cmd{child("wait")}
			if mode == "cancel" {
				timer := time.AfterFunc(100*time.Millisecond, cancel)
				defer timer.Stop()
			} else if mode == "start-error" {
				commands = append(commands, func(ctx context.Context) *exec.Cmd {
					return exec.CommandContext(ctx, "ptdev-missing-test-executable")
				})
			} else {
				commands = append(commands, child(mode))
			}
			start := time.Now()
			err := supervise(ctx, commands...)
			if mode == "cancel" && err != nil {
				t.Fatal(err)
			}
			if mode != "cancel" && err == nil {
				t.Fatal("unexpected child exit/start failure was not reported")
			}
			if mode == "fail" && !strings.Contains(err.Error(), "exit status 7") {
				t.Fatalf("missing exit status: %v", err)
			}
			if time.Since(start) > 4*time.Second {
				t.Fatal("cleanup was not bounded")
			}
			for _, command := range children {
				if command.ProcessState == nil {
					t.Fatal("child was not reaped")
				}
			}
		})
	}
}
