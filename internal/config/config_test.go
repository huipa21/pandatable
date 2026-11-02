package config

import (
	"bytes"
	"encoding/base64"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func TestParseDefaults(t *testing.T) {
	cfg, err := parse([]string(nil), lookup(map[string]string{
		"PT_DATABASE_URL": "postgres://user:pass@db:5432/pandatable",
		"PT_BASE_URL":     "https://table.example.com",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DataDir != "/data" || cfg.Listen != ":8080" || cfg.DBMaxConns != 20 || cfg.LogLevel != slog.LevelInfo || cfg.DBConnectRetries != 30 {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if got, want := len(cfg.TrustedProxies), 3; got != want {
		t.Fatalf("len(TrustedProxies) = %d, want %d", got, want)
	}
	if got, want := cfg.TrustedProxies[0].String(), "10.0.0.0/8"; got != want {
		t.Errorf("TrustedProxies[0] = %q, want %q", got, want)
	}
}

func TestParseFlagOverridesEnvironment(t *testing.T) {
	cfg, err := parse([]string{
		"--database-url=postgres://flag:secret@db:5432/flagdb",
		"--data-dir=C:\\pt-data",
		"--listen=127.0.0.1:9090",
		"--base-url=http://localhost:9090",
		"--trusted-proxies=192.0.2.0/24",
		"--db-max-conns=8",
		"--log-level=debug",
		"--db-connect-retries=0",
	}, lookup(map[string]string{
		"PT_DATABASE_URL":       "postgres://env:secret@db:5432/envdb",
		"PT_DATA_DIR":           "env-data",
		"PT_LISTEN":             ":8000",
		"PT_BASE_URL":           "https://env.example.com",
		"PT_TRUSTED_PROXIES":    "10.0.0.0/8",
		"PT_DB_MAX_CONNS":       "12",
		"PT_LOG_LEVEL":          "warn",
		"PT_DB_CONNECT_RETRIES": "10",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseURL != "postgres://flag:secret@db:5432/flagdb" ||
		cfg.DataDir != "C:\\pt-data" ||
		cfg.Listen != "127.0.0.1:9090" ||
		cfg.BaseURL != "http://localhost:9090" ||
		cfg.TrustedProxies[0].String() != "192.0.2.0/24" ||
		cfg.DBMaxConns != 8 ||
		cfg.LogLevel != slog.LevelDebug ||
		cfg.DBConnectRetries != 0 {
		t.Fatalf("flags did not override environment: %+v", cfg)
	}
}

func TestParseEnvironmentOverridesDefaults(t *testing.T) {
	cfg, err := parse(nil, lookup(map[string]string{
		"PT_DATABASE_URL":       "postgresql://db/pandatable",
		"PT_DATA_DIR":           "relative-data",
		"PT_LISTEN":             "127.0.0.1:8081",
		"PT_BASE_URL":           "https://table.example.com/app",
		"PT_TRUSTED_PROXIES":    "10.1.2.3/8",
		"PT_DB_MAX_CONNS":       "7",
		"PT_LOG_LEVEL":          "error",
		"PT_DB_CONNECT_RETRIES": "3",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DataDir != "relative-data" || cfg.Listen != "127.0.0.1:8081" || cfg.DBMaxConns != 7 ||
		cfg.LogLevel != slog.LevelError || cfg.DBConnectRetries != 3 || cfg.TrustedProxies[0].String() != "10.0.0.0/8" {
		t.Fatalf("environment values were not applied: %+v", cfg)
	}
}

func TestParseRequiresDatabaseAndPublicURLs(t *testing.T) {
	for _, name := range []string{"PT_DATABASE_URL", "PT_BASE_URL"} {
		t.Run(name, func(t *testing.T) {
			values := map[string]string{
				"PT_DATABASE_URL": "postgres://db/pandatable",
				"PT_BASE_URL":     "https://table.example.com",
			}
			delete(values, name)
			_, err := parse(nil, lookup(values))
			if err == nil || !strings.Contains(err.Error(), name) {
				t.Fatalf("parse() error = %v, want error mentioning %s", err, name)
			}
		})
	}
}

func TestParseRejectsInvalidSettings(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{"database URL", "PT_DATABASE_URL", "mysql://user:secret@db/app"},
		{"malformed database URL", "PT_DATABASE_URL", "postgres://user:secret@db:bad-port/app"},
		{"data directory", "PT_DATA_DIR", "   "},
		{"listen address", "PT_LISTEN", "https://example.com"},
		{"listen port", "PT_LISTEN", ":bad-port"},
		{"public URL", "PT_BASE_URL", "https://user:secret@example.com"},
		{"malformed public URL", "PT_BASE_URL", "https://%zz"},
		{"proxy CIDR", "PT_TRUSTED_PROXIES", "10.0.0.0/8,not-a-cidr"},
		{"connection count", "PT_DB_MAX_CONNS", "twenty"},
		{"connection count range", "PT_DB_MAX_CONNS", "0"},
		{"log level", "PT_LOG_LEVEL", "verbose"},
		{"retry count", "PT_DB_CONNECT_RETRIES", "-1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			values := map[string]string{
				"PT_DATABASE_URL": "postgres://user:pass@db:5432/pandatable",
				"PT_BASE_URL":     "https://table.example.com",
				test.key:          test.value,
			}
			_, err := parse(nil, lookup(values))
			if err == nil || !strings.Contains(err.Error(), test.key) {
				t.Fatalf("parse() error = %v, want error mentioning %s", err, test.key)
			}
			if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "pass") {
				t.Errorf("parse() exposed credentials: %v", err)
			}
		})
	}
}

func TestParseAllowsEmptyTrustedProxyList(t *testing.T) {
	cfg, err := parse(nil, lookup(map[string]string{
		"PT_DATABASE_URL":    "postgres://db/pandatable",
		"PT_BASE_URL":        "https://table.example.com",
		"PT_TRUSTED_PROXIES": "",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.TrustedProxies) != 0 {
		t.Fatalf("TrustedProxies = %v, want empty", cfg.TrustedProxies)
	}
}

func TestParseSMTPKey(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	valid := base64.StdEncoding.EncodeToString(key)
	values := map[string]string{
		"PT_DATABASE_URL": "postgresql://db/pandatable",
		"PT_BASE_URL":     "https://table.example.com",
	}
	cfg, err := parse(nil, lookup(values))
	if err != nil || len(cfg.SMTPKey) != 0 {
		t.Fatal("SMTP key should be optional", err)
	}
	values["PT_SMTP_KEY"] = valid
	cfg, err = parse(nil, lookup(values))
	if err != nil || !bytes.Equal(cfg.SMTPKey, key) {
		t.Fatal("valid SMTP key was not decoded", err)
	}
	for _, invalid := range []string{
		"private-invalid-key",
		base64.StdEncoding.EncodeToString(key[:31]),
		strings.TrimRight(valid, "="),
		valid + "\n",
	} {
		values["PT_SMTP_KEY"] = invalid
		_, err := parse(nil, lookup(values))
		if err == nil || !strings.Contains(err.Error(), "PT_SMTP_KEY") || strings.Contains(err.Error(), invalid) {
			t.Fatal("invalid SMTP key was accepted or exposed")
		}
	}
}

func TestParseInvalidFlagsAreUsageErrors(t *testing.T) {
	_, err := parse([]string{"--not-a-config-option"}, lookup(map[string]string{}))
	var usage UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("parse() error = %v, want UsageError", err)
	}
}

func lookup(values map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	}
}
