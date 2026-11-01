package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestRunHelpAndVersionDoNotRequireDatabaseConfiguration(t *testing.T) {
	t.Setenv("PT_DATABASE_URL", "")
	t.Setenv("PT_BASE_URL", "")

	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{"root help", []string{"--help"}, "Commands:"},
		{"serve help", []string{"serve", "--help"}, "--database-url"},
		{"migrate help", []string{"migrate", "--help"}, "--database-url"},
		{"doctor help", []string{"doctor", "--help"}, "--database-url"},
		{"restore help", []string{"restore", "--help"}, "--yes"},
		{"version", []string{"version"}, version},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if got := runWith(test.args, &stdout, &stderr); got != 0 {
				t.Fatalf("runWith() = %d, want 0; stderr: %s", got, stderr.String())
			}
			if !strings.Contains(stdout.String(), test.want) {
				t.Errorf("stdout %q does not contain %q", stdout.String(), test.want)
			}
		})
	}
}

func TestRunRejectsMalformedArguments(t *testing.T) {
	t.Setenv("PT_DATABASE_URL", "")
	t.Setenv("PT_BASE_URL", "")
	tests := []struct {
		args []string
		want int
	}{
		{nil, 2},
		{[]string{"unknown"}, 2},
		{[]string{"version", "extra"}, 2},
		{[]string{"serve", "--unknown"}, 2},
		{[]string{"serve", "unexpected"}, 2},
		{[]string{"serve"}, 1},
		{[]string{"doctor"}, 1},
	}
	for _, test := range tests {
		t.Run(strings.Join(test.args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if got := runWith(test.args, &stdout, &stderr); got != test.want {
				t.Errorf("runWith(%v) = %d, want %d; stderr: %s", test.args, got, test.want, stderr.String())
			}
		})
	}
}

func TestServeConnectionFailureAsJSONWithoutSecrets(t *testing.T) {
	t.Setenv("PT_DATABASE_URL", "postgres://admin:top-secret@db:5432/pandatable")
	t.Setenv("PT_BASE_URL", "https://table.example.com")
	t.Setenv("PT_LOG_LEVEL", "warn")
	t.Setenv("PT_DB_CONNECT_RETRIES", "0")

	for _, test := range []struct {
		command string
		message string
	}{
		{"serve", "HTTP server failed"},
	} {
		t.Run(test.command, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if got := runWith([]string{test.command}, &stdout, &stderr); got != 1 {
				t.Fatalf("runWith() = %d, want 1; stderr: %s", got, stderr.String())
			}
			if stderr.Len() != 0 {
				t.Errorf("unexpected stderr: %s", stderr.String())
			}
			if strings.Contains(stdout.String(), "top-secret") || strings.Contains(stdout.String(), "postgres://") {
				t.Fatal("JSON log exposed the database password")
			}
			var event struct {
				Level   string `json:"level"`
				Message string `json:"msg"`
			}
			if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &event); err != nil {
				t.Fatalf("log is not JSON: %v; output: %s", err, stdout.String())
			}
			if event.Level != "ERROR" || event.Message != test.message {
				t.Errorf("unexpected log event: %+v", event)
			}
		})
	}
}

func TestMigrateConnectionFailureDoesNotLogCredentials(t *testing.T) {
	t.Setenv("PT_DATABASE_URL", "postgres://admin:top-secret@127.0.0.1:1/pandatable")
	t.Setenv("PT_BASE_URL", "https://table.example.com")
	t.Setenv("PT_DB_CONNECT_RETRIES", "0")

	var stdout, stderr bytes.Buffer
	if got := runWith([]string{"migrate"}, &stdout, &stderr); got != 1 {
		t.Fatalf("runWith() = %d, want 1; stderr: %s", got, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("unexpected stderr: %s", stderr.String())
	}
	if strings.Contains(stdout.String(), "top-secret") || strings.Contains(stdout.String(), "postgres://") {
		t.Fatalf("migration error log exposed connection credentials or URL: %s", stdout.String())
	}
	var event map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &event); err != nil {
		t.Fatalf("log is not JSON: %v; output: %s", err, stdout.String())
	}
	if event["msg"] != "database migration failed" {
		t.Errorf("unexpected log event: %v", event)
	}
}

func TestRunStubsReportTheirBoundaries(t *testing.T) {
	for _, test := range []struct {
		command string
		task    string
	}{
		{"reset-admin-password", "T1.5"},
	} {
		t.Run(test.command, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if got := runWith([]string{test.command}, &stdout, &stderr); got != 1 {
				t.Fatalf("runWith() = %d, want 1", got)
			}
			if !strings.Contains(stdout.String(), test.task) {
				t.Errorf("output %q does not identify boundary %s", stdout.String(), test.task)
			}
		})
	}
}

func TestDoctorCLIReportsDrift(t *testing.T) {
	pool := database.Open(t)
	t.Setenv("PT_DATABASE_URL", database.URL(t))
	t.Setenv("PT_BASE_URL", "https://table.example")
	t.Setenv("PT_DATA_DIR", t.TempDir())
	t.Setenv("PT_DB_CONNECT_RETRIES", "0")
	t.Setenv("PT_LOG_LEVEL", "info")
	var stdout, stderr bytes.Buffer
	if got := runWith([]string{"doctor"}, &stdout, &stderr); got != 0 ||
		!strings.Contains(stdout.String(), "doctor metadata and catalog checks passed") {
		t.Fatalf("doctor = %d, stdout=%s stderr=%s", got, stdout.String(), stderr.String())
	}
	if _, err := pool.Exec(context.Background(), `CREATE SCHEMA b_zzzzzzzzzz`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DROP SCHEMA b_zzzzzzzzzz`); err != nil {
			t.Error(err)
		}
	})
	stdout.Reset()
	stderr.Reset()
	if got := runWith([]string{"doctor"}, &stdout, &stderr); got != 1 ||
		!strings.Contains(stdout.String(), "unexpected_schema") {
		t.Fatalf("doctor drift = %d, stdout=%s stderr=%s", got, stdout.String(), stderr.String())
	}
}

func TestRestoreRequiresExplicitConfirmation(t *testing.T) {
	t.Setenv("PT_DATABASE_URL", "")
	t.Setenv("PT_BASE_URL", "")
	for _, args := range [][]string{
		{"restore"}, {"restore", "backup.zip"}, {"restore", "--yes"},
		{"restore", "backup.zip", "--yes", "--yes"}, {"restore", "backup.zip", "--unknown"},
	} {
		var stdout, stderr bytes.Buffer
		if got := runWith(args, &stdout, &stderr); got != 2 || !strings.Contains(stderr.String(), "restore") {
			t.Errorf("runWith(%v) = %d, stderr %q", args, got, stderr.String())
		}
	}
	var stdout, stderr bytes.Buffer
	if got := runWith([]string{"restore", "backup.zip", "--yes"}, &stdout, &stderr); got != 1 {
		t.Errorf("confirmed restore without configuration = %d, want 1", got)
	}
}

func TestUnknownCommandDoesNotEchoArguments(t *testing.T) {
	secretArgument := "postgres://admin:top-secret@db/pandatable"
	var stdout, stderr bytes.Buffer
	if got := runWith([]string{secretArgument}, &stdout, &stderr); got != 2 {
		t.Fatalf("runWith() = %d, want 2", got)
	}
	if strings.Contains(stderr.String(), secretArgument) || strings.Contains(stderr.String(), "top-secret") {
		t.Fatalf("error output exposed the command argument: %s", stderr.String())
	}
}

func TestJSONLoggerHonorsConfiguredLevel(t *testing.T) {
	var output bytes.Buffer
	logger := newJSONLogger(&output, slog.LevelWarn)
	logger.Debug("debug")
	logger.Info("info")
	logger.Warn("warn")
	logger.Error("error")
	log := output.String()
	if strings.Contains(log, `"msg":"debug"`) || strings.Contains(log, `"msg":"info"`) {
		t.Fatalf("logger emitted messages below warn level: %s", log)
	}
	if !strings.Contains(log, `"msg":"warn"`) || !strings.Contains(log, `"msg":"error"`) {
		t.Fatalf("logger omitted messages at or above warn level: %s", log)
	}
	for _, line := range strings.Split(strings.TrimSpace(log), "\n") {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Errorf("log line is not JSON: %v; line: %s", err, line)
		}
	}
}
