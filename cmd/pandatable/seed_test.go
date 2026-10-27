package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestDeveloperSeedCLI(t *testing.T) {
	t.Setenv("PT_DATABASE_URL", "")
	for _, args := range [][]string{{"dev", "--help"}, {"dev", "seed", "--help"}} {
		var out, stderr bytes.Buffer
		if runWith(args, &out, &stderr) != 0 || !strings.Contains(out.String(), "--yes-dev-seed") {
			t.Fatal("seed help requires runtime or omits safety boundary")
		}
	}
	valid := []string{"--confirm-database", "pt_seed_owned", "--owner-id", "00000000-0000-4000-8000-000000000001", "--yes-dev-seed"}
	t.Setenv("PT_DATABASE_URL", "postgres://seed:top-secret@127.0.0.1:1/pt_seed_owned")
	if opts, err := parseSeed(valid); err != nil || opts.rows != 200000 {
		t.Fatalf("valid seed flags: %+v %v", opts, err)
	}
	for _, args := range [][]string{nil, {"seed"}, {"--rows", "0"}, {"--rows", "200001"},
		{"--confirm-database", "production"}, {"--unknown", "top-secret"}, {"--owner-id", "bad"},
		{"--database-url", "postgres://user:top-secret@localhost/postgres"},
		{"--database-url", "dbname=pt_seed_owned password=top-secret"},
		{"--database-url", "postgres:///pt_seed_owned"},
		{"--yes-dev-seed=false"}, {"unexpected"}} {
		all := append(append([]string{}, valid...), args...)
		if args == nil {
			all = nil
		}
		var out, stderr bytes.Buffer
		if code := runWith(append([]string{"dev", "seed"}, all...), &out, &stderr); code != 2 {
			t.Fatalf("invalid seed args returned %d: %s %s", code, out.String(), stderr.String())
		}
		if strings.Contains(out.String()+stderr.String(), "top-secret") {
			t.Fatal("seed usage failure leaked credentials")
		}
	}
	var out, stderr bytes.Buffer
	if code := runWith(append([]string{"dev", "seed"}, valid...), &out, &stderr); code != 1 ||
		!strings.Contains(out.String(), `"complete":false`) || !strings.Contains(out.String(), `"stage":"connect"`) ||
		strings.Contains(out.String()+stderr.String(), "top-secret") {
		t.Fatalf("connection failure boundary: code=%d %s %s", code, out.String(), stderr.String())
	}
}
