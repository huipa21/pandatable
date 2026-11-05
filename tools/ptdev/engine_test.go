package main

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDetectEngine(t *testing.T) {
	for _, test := range []struct {
		name, override, want string
		available            []string
		calls                []string
		wantErr, usage       bool
	}{
		{name: "prefer podman", available: []string{"podman", "docker"}, want: "podman", calls: []string{"podman"}},
		{name: "docker fallback", available: []string{"docker"}, want: "docker", calls: []string{"podman", "docker"}},
		{name: "override", override: "docker", available: []string{"podman", "docker"}, want: "docker", calls: []string{"docker"}},
		{name: "missing override", override: "docker", available: []string{"podman"}, calls: []string{"docker"}, wantErr: true},
		{name: "invalid override", override: "foo", wantErr: true, usage: true},
		{name: "none", calls: []string{"podman", "docker"}, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls []string
			got, err := detectEngine(test.override, func(name string) (string, error) {
				calls = append(calls, name)
				for _, available := range test.available {
					if name == available {
						return name, nil
					}
				}
				return "", errors.New("not found")
			})
			var usage usageError
			if (err != nil) != test.wantErr || got.name != test.want || errors.As(err, &usage) != test.usage || !reflect.DeepEqual(calls, test.calls) {
				t.Fatalf("got engine=%+v err=%v calls=%v", got, err, calls)
			}
		})
	}
}

func TestEnsureEngine(t *testing.T) {
	for _, test := range []struct {
		label, name, goos, machines string
		ready, failStart            bool
		want                        []string
		wantErr                     string
	}{
		{label: "already ready", name: "podman", goos: "windows", ready: true, want: []string{"podman info"}},
		{label: "start Podman", name: "podman", goos: "windows", machines: `[{"Name":"dev"}]`,
			want: []string{"podman info", "podman machine list --format json", "podman machine start dev", "podman info"}},
		{label: "init Podman", name: "podman", goos: "darwin", machines: `[]`,
			want: []string{"podman info", "podman machine list --format json", "podman machine init", "podman machine start", "podman info"}},
		{label: "default machine", name: "podman", goos: "windows", machines: `[{"Name":"other"},{"Name":"dev","Default":true}]`,
			want: []string{"podman info", "podman machine list --format json", "podman machine start dev", "podman info"}},
		{label: "running machine reconnects", name: "podman", goos: "windows", machines: `[{"Name":"dev","Running":true}]`,
			want: []string{"podman info", "podman machine list --format json", "podman info"}},
		{label: "starting machine reconnects", name: "podman", goos: "windows", machines: `[{"Name":"dev","Starting":true}]`,
			want: []string{"podman info", "podman machine list --format json", "podman info"}},
		{label: "ambiguous machines", name: "podman", goos: "windows", machines: `[{"Name":"one"},{"Name":"two"}]`,
			wantErr: "no default Podman machine"},
		{label: "malformed machines", name: "podman", goos: "windows", machines: `not json`, wantErr: "decode Podman machines"},
		{label: "Podman Linux failure", name: "podman", goos: "linux", wantErr: "daemonless"},
		{label: "Docker Windows", name: "docker", goos: "windows",
			want: []string{"docker info", "docker desktop start", "docker info"}},
		{label: "Docker macOS", name: "docker", goos: "darwin",
			want: []string{"docker info", "docker desktop start", "docker info"}},
		{label: "Docker Linux", name: "docker", goos: "linux",
			want: []string{"docker info", "systemctl start docker", "docker info"}},
		{label: "startup failure", name: "docker", goos: "windows", failStart: true, wantErr: "startup denied"},
	} {
		t.Run(test.label, func(t *testing.T) {
			var calls []string
			probes := 0
			command := func(_ context.Context, name string, args ...string) (string, error) {
				calls = append(calls, name+" "+strings.Join(args, " "))
				if args[0] == "info" {
					probes++
					if !test.ready && probes == 1 {
						return "daemon offline", errors.New("connection refused")
					}
					return "", nil
				}
				if strings.Join(args, " ") == "machine list --format json" {
					return test.machines, nil
				}
				if test.failStart {
					return "startup denied", errors.New("exit status 1")
				}
				return "", nil
			}
			err := ensureEngine(context.Background(), test.name, test.goos, command)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("error=%v, want %s", err, test.wantErr)
				}
			} else if err != nil || !reflect.DeepEqual(calls, test.want) {
				t.Fatalf("err=%v calls=%v, want=%v", err, calls, test.want)
			}
		})
	}
}

func TestEnsureEngineBounded(t *testing.T) {
	for _, cancelBeforeStart := range []bool{false, true} {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		if cancelBeforeStart {
			cancel()
		}
		start := time.Now()
		err := ensureEngine(ctx, "docker", "windows", func(ctx context.Context, _ string, args ...string) (string, error) {
			if _, ok := ctx.Deadline(); !ok {
				t.Error("engine command lacks deadline")
			}
			if args[0] == "info" {
				return "offline", errors.New("offline")
			}
			return "", nil
		})
		cancel()
		if err == nil || time.Since(start) > time.Second ||
			(!errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)) {
			t.Fatalf("engine cancellation/timeout lost: %v", err)
		}
	}
}

func TestPodmanMachineListFailure(t *testing.T) {
	err := ensureEngine(context.Background(), "podman", "windows", func(_ context.Context, _ string, args ...string) (string, error) {
		return strings.Join(args, " "), errors.New("permission denied")
	})
	if err == nil || !strings.Contains(err.Error(), "list Podman machines") {
		t.Fatalf("machine query failure lost: %v", err)
	}
}
