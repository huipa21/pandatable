package main

import (
	"context"
	"strings"
	"testing"
)

func TestMergeEnv(t *testing.T) {
	got := mergeEnv([]string{"Path=tools", "GOOS=windows", "PT_TEST_DATABASE_URL=old"}, map[string]string{
		"GOOS": "linux", "PT_TEST_DATABASE_URL": dbDSN,
	})
	joined := strings.Join(got, "\n")
	if len(got) != 3 || !strings.Contains(joined, "Path=tools") || !strings.Contains(joined, "GOOS=linux") || strings.Contains(joined, "=old") {
		t.Fatalf("incorrect overrides: %v", got)
	}
	got = mergeEnv([]string{"goos=windows"}, map[string]string{"GOOS": "linux"})
	if len(got) != 1 || got[0] != "GOOS=linux" {
		t.Fatalf("case-insensitive override: %v", got)
	}
}

func TestUsageExitCodes(t *testing.T) {
	t.Setenv("PT_ENGINE", "")
	for _, args := range [][]string{
		{"unknown"}, {"db"}, {"e2e", "--bad"}, {"deploy"},
	} {
		if code := run(context.Background(), args); code != 2 {
			t.Errorf("run(%v) = %d, want 2", args, code)
		}
	}
	t.Setenv("PT_ENGINE", "foo")
	if code := run(context.Background(), []string{"version"}); code != 2 {
		t.Errorf("invalid PT_ENGINE exit = %d", code)
	}
}
