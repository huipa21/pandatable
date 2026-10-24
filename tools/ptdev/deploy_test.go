package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeployDestination(t *testing.T) {
	for _, test := range []struct {
		host, dir string
		valid     bool
	}{
		{"ops@server.example.com", "/opt/pandatable", true},
		{"my-server", "/opt/table's data", true},
		{"user@192.0.2.1", "/srv/pandatable", true},
		{"", "/opt/pandatable", false},
		{"-oProxyCommand=bad", "/opt/pandatable", false},
		{"host;bad", "/opt/pandatable", false},
		{"host", "/", false},
		{"host", "relative", false},
		{"host", "/opt/../etc", false},
		{"host", "/opt/x\nbad", false},
		{"host", `C:\data`, false},
	} {
		if err := validateDestination(test.host, test.dir); (err == nil) != test.valid {
			t.Errorf("%q %q: %v", test.host, test.dir, err)
		}
	}
}

func TestDeployCommands(t *testing.T) {
	dir := "/opt/table's data"
	preflight := deployPreflight(dir)
	if !strings.Contains(preflight, shellQuote(dir)) || !strings.Contains(preflight, "config --quiet") ||
		strings.Contains(preflight, "cat .env") || !strings.Contains(preflight, "grep -c '^PT_VERSION='") {
		t.Fatalf("unsafe preflight: %s", preflight)
	}
	archives := []string{"/tmp/stage/app.tar.gz", "/tmp/stage/postgres-18.tar.gz"}
	start := deployStart(dir, "1.2.3-build", archives)
	for _, want := range []string{"set -eu", "docker load -i '/tmp/stage/app.tar.gz'", "docker load -i '/tmp/stage/postgres-18.tar.gz'",
		"PT_VERSION=1.2.3-build", "up -d --wait --wait-timeout 60 app", "pandatable healthcheck"} {
		if !strings.Contains(start, want) {
			t.Errorf("missing %q: %s", want, start)
		}
	}
	if got := shellQuote("a'b c;$"); got != `'a'"'"'b c;$'` {
		t.Fatalf("shell quote = %s", got)
	}
}

func TestDeployRequiresArchivesBeforeSSH(t *testing.T) {
	err := (runner{root: t.TempDir()}).deploy(context.Background(), options{
		host: "no-real-server.invalid", remoteDir: "/opt/pandatable", yes: true, releaseVersion: "1.2.3",
	})
	if err == nil || !strings.Contains(err.Error(), "existing image save archives") {
		t.Fatalf("missing archive: %v", err)
	}
}

func TestDeployShell(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX sh unavailable; remote command execution tests require Linux")
	}
	for _, failure := range []string{"", "compose config", "load", "compose up", "compose exec"} {
		t.Run(failure, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, "table's data")
			bin := filepath.Join(root, "bin")
			for _, directory := range []string{dir, bin} {
				if err := os.MkdirAll(directory, 0o750); err != nil {
					t.Fatal(err)
				}
			}
			for name, content := range map[string]string{
				"compose.yaml":    "services: {}\n",
				"postgresql.conf": "",
				".env":            "PT_VERSION=old\nPT_DB_PASSWORD=fixture-only\n",
			} {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$TRACE\"\nif [ -n \"$FAIL_AT\" ]; then\ncase \"$*\" in \"$FAIL_AT\"*) exit 9;; esac\nfi\n"
			if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0o700); err != nil {
				t.Fatal(err)
			}
			trace := filepath.Join(root, "trace")
			env := mergeEnv(os.Environ(), map[string]string{
				"PATH":    bin + string(os.PathListSeparator) + os.Getenv("PATH"),
				"TRACE":   trace,
				"FAIL_AT": failure,
			})
			command := exec.Command(shell, "-c", deployPreflight(filepath.ToSlash(dir)))
			command.Env = env
			output, err := command.CombinedOutput()
			if failure == "compose config" {
				if err == nil || exists(filepath.Join(dir, "images")) {
					t.Fatalf("failed preflight continued: %v, %s", err, output)
				}
				return
			}
			if err != nil {
				t.Fatalf("preflight: %v, %s", err, output)
			}
			command = exec.Command(shell, "-c", deployStart(filepath.ToSlash(dir), "1.2.3", []string{"/tmp/app archive.tar.gz", "/tmp/postgres.tar.gz"}))
			command.Env = env
			output, err = command.CombinedOutput()
			if (err != nil) != (failure != "") {
				t.Fatalf("start error = %v, output=%s", err, output)
			}
			if strings.Contains(string(output), "fixture-only") {
				t.Fatal("command output exposed .env contents")
			}
			content, err := os.ReadFile(filepath.Join(dir, ".env"))
			if err != nil {
				t.Fatal(err)
			}
			wantVersion := "1.2.3"
			if failure == "load" {
				wantVersion = "old"
			}
			if string(content) != "PT_VERSION="+wantVersion+"\nPT_DB_PASSWORD=fixture-only\n" {
				t.Fatalf("unexpected .env change: %q", content)
			}
			calls, err := os.ReadFile(trace)
			if err != nil {
				t.Fatal(err)
			}
			if failure == "load" && strings.Contains(string(calls), "compose up") {
				t.Fatal("failed load continued to compose up")
			}
			if failure == "compose up" && strings.Contains(string(calls), "compose exec") {
				t.Fatal("failed start continued to healthcheck")
			}
			if failure == "" {
				for _, want := range []string{"load -i /tmp/app archive.tar.gz", "load -i /tmp/postgres.tar.gz", "compose exec -T app pandatable healthcheck"} {
					if !strings.Contains(string(calls), want) {
						t.Fatalf("missing %q in %s", want, calls)
					}
				}
			}
		})
	}
}
