package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const defaultTrustedProxies = "10.0.0.0/8,172.16.0.0/12,192.168.0.0/16"

type Config struct {
	DatabaseURL      string
	DataDir          string
	Listen           string
	BaseURL          string
	TrustedProxies   []netip.Prefix
	DBMaxConns       int
	LogLevel         slog.Level
	DBConnectRetries int
}

type UsageError struct {
	message string
}

func (err UsageError) Error() string {
	return err.message
}

func Parse(args []string) (Config, error) {
	return parse(args, os.LookupEnv)
}

func parse(args []string, lookup func(string) (string, bool)) (Config, error) {
	read := func(name, fallback string) string {
		if value, ok := lookup(name); ok {
			return value
		}
		return fallback
	}

	databaseURL := read("PT_DATABASE_URL", "")
	dataDir := read("PT_DATA_DIR", "/data")
	listen := read("PT_LISTEN", ":8080")
	baseURL := read("PT_BASE_URL", "")
	trustedProxies := read("PT_TRUSTED_PROXIES", defaultTrustedProxies)
	dbMaxConns := read("PT_DB_MAX_CONNS", "20")
	logLevel := read("PT_LOG_LEVEL", "info")
	dbConnectRetries := read("PT_DB_CONNECT_RETRIES", "30")

	flags := flag.NewFlagSet("pandatable", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.Usage = func() {}
	flags.StringVar(&databaseURL, "database-url", databaseURL, "PostgreSQL connection URL")
	flags.StringVar(&dataDir, "data-dir", dataDir, "application data directory")
	flags.StringVar(&listen, "listen", listen, "HTTP listen address")
	flags.StringVar(&baseURL, "base-url", baseURL, "public application URL")
	flags.StringVar(&trustedProxies, "trusted-proxies", trustedProxies, "comma-separated trusted proxy CIDRs")
	flags.StringVar(&dbMaxConns, "db-max-conns", dbMaxConns, "maximum database connections")
	flags.StringVar(&logLevel, "log-level", logLevel, "debug, info, warn, or error")
	flags.StringVar(&dbConnectRetries, "db-connect-retries", dbConnectRetries, "database startup retries")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return Config{}, flag.ErrHelp
		}
		return Config{}, UsageError{message: "invalid command flags (use --help for options)"}
	}
	if len(flags.Args()) != 0 {
		return Config{}, UsageError{message: "unexpected positional arguments"}
	}

	var cfg Config
	var err error
	if cfg.DatabaseURL, err = validateDatabaseURL(databaseURL); err != nil {
		return Config{}, err
	}
	if cfg.DataDir, err = required("PT_DATA_DIR", dataDir); err != nil {
		return Config{}, err
	}
	if cfg.Listen, err = validateListen(listen); err != nil {
		return Config{}, err
	}
	if cfg.BaseURL, err = validateBaseURL(baseURL); err != nil {
		return Config{}, err
	}
	if cfg.TrustedProxies, err = parseTrustedProxies(trustedProxies); err != nil {
		return Config{}, err
	}
	if cfg.DBMaxConns, err = parseInteger("PT_DB_MAX_CONNS", dbMaxConns, 1); err != nil {
		return Config{}, err
	}
	if cfg.LogLevel, err = parseLogLevel(logLevel); err != nil {
		return Config{}, err
	}
	if cfg.DBConnectRetries, err = parseInteger("PT_DB_CONNECT_RETRIES", dbConnectRetries, 0); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func required(name, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

func validateDatabaseURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("PT_DATABASE_URL is required")
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || parsed.Opaque != "" || parsed.Hostname() == "" || !validURLPort(parsed) {
		return "", errors.New("PT_DATABASE_URL must be a valid postgres:// or postgresql:// URL with a host")
	}
	return value, nil
}

func validateBaseURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("PT_BASE_URL is required")
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Opaque != "" || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || !validURLPort(parsed) {
		return "", errors.New("PT_BASE_URL must be an absolute http:// or https:// URL with a host and without credentials, query, or fragment")
	}
	return value, nil
}

func validURLPort(parsed *url.URL) bool {
	port := parsed.Port()
	if port == "" {
		return true
	}
	number, err := strconv.Atoi(port)
	return err == nil && number >= 1 && number <= 65535
}

func validateListen(value string) (string, error) {
	value = strings.TrimSpace(value)
	host, port, err := net.SplitHostPort(value)
	if err != nil || (host == "" && port == "") {
		return "", errors.New("PT_LISTEN must be a host:port address (for example :8080)")
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return "", errors.New("PT_LISTEN must include a port from 1 to 65535")
	}
	return value, nil
}

func parseTrustedProxies(value string) ([]netip.Prefix, error) {
	if strings.TrimSpace(value) == "" {
		return []netip.Prefix{}, nil
	}
	parts := strings.Split(value, ",")
	prefixes := make([]netip.Prefix, 0, len(parts))
	for _, part := range parts {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(part))
		if err != nil {
			return nil, errors.New("PT_TRUSTED_PROXIES must be a comma-separated list of CIDRs")
		}
		prefixes = append(prefixes, prefix.Masked())
	}
	return prefixes, nil
}

func parseInteger(name, value string, minimum int) (int, error) {
	number, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || number < minimum {
		if minimum == 0 {
			return 0, fmt.Errorf("%s must be a non-negative integer", name)
		}
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return number, nil
}

func parseLogLevel(value string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, errors.New("PT_LOG_LEVEL must be debug, info, warn, or error")
	}
}
