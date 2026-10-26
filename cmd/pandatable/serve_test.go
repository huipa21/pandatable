package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"pandatable/internal/config"
	"pandatable/internal/testdb"
)

var database testdb.Suite

func TestMain(m *testing.M) {
	os.Exit(database.Run(m))
}

func TestServePostgreSQLRestarts(t *testing.T) {
	databaseURL := database.URL(t)
	appConfig, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal("invalid disposable database URL")
	}
	pool := database.Open(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cfg := config.Config{
		DatabaseURL: databaseURL,
		DataDir:     t.TempDir(),
		DBMaxConns:  4,
		Listen:      "127.0.0.1:0",
		BaseURL:     "http://127.0.0.1",
	}
	migrationRecords := func() string {
		t.Helper()
		var records string
		if err := pool.QueryRow(ctx, `SELECT jsonb_agg(to_jsonb(v) ORDER BY id)::text
			FROM public.goose_db_version v`).Scan(&records); err != nil {
			t.Fatal(err)
		}
		return records
	}
	beforeRestarts := migrationRecords()
	previousSetupToken := ""
	for iteration := 1; iteration <= 3; iteration++ {
		address, stop, output := startTestServe(t, cfg)
		const setupLogPrefix = `"msg":"SETUP_TOKEN=`
		if strings.Count(output.String(), setupLogPrefix) != 1 {
			stop()
			t.Fatalf("startup %d must log exactly one setup token", iteration)
		}
		tokenStart := strings.Index(output.String(), setupLogPrefix) + len(setupLogPrefix)
		setupToken := strings.SplitN(output.String()[tokenStart:], `"`, 2)[0]
		if len(setupToken) != 43 || strings.ContainsAny(setupToken, "=+/ ") || setupToken == previousSetupToken {
			stop()
			t.Fatalf("startup %d emitted an invalid or reused setup token", iteration)
		}
		client := &http.Client{Timeout: 5 * time.Second}
		for _, test := range []struct {
			path   string
			status int
		}{
			{"/healthz", http.StatusOK},
			{"/api/v1/missing", http.StatusNotFound},
		} {
			response, err := client.Get("http://" + address + test.path)
			if err != nil {
				t.Fatalf("startup %d %s: %v", iteration, test.path, err)
			}
			body, readErr := io.ReadAll(response.Body)
			response.Body.Close()
			if readErr != nil {
				t.Fatal(readErr)
			}
			if response.StatusCode != test.status {
				t.Fatalf("startup %d %s: status=%d, want %d", iteration, test.path, response.StatusCode, test.status)
			}
			if test.path == "/healthz" && string(body) != "{\"status\":\"ok\"}\n" {
				t.Fatalf("unexpected health response: %q", body)
			}
		}
		if previousSetupToken != "" {
			request, err := http.NewRequest(http.MethodPost, "http://"+address+"/api/v1/setup", strings.NewReader(
				`{"token":"`+previousSetupToken+`","username":"restart-admin","email":"restart@example.test","displayName":"Restart Admin","password":"12345678","locale":"en"}`))
			if err != nil {
				stop()
				t.Fatal(err)
			}
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Origin", "http://127.0.0.1")
			request.Header.Set("X-PT-CSRF", "1")
			response, err := client.Do(request)
			if err != nil {
				stop()
				t.Fatal(err)
			}
			body, readErr := io.ReadAll(response.Body)
			closeErr := response.Body.Close()
			if readErr != nil || closeErr != nil || response.StatusCode != http.StatusUnauthorized ||
				!strings.Contains(string(body), `"code":"ERR_SETUP_TOKEN"`) {
				stop()
				t.Fatalf("previous-process setup token response=%d body=%s errors=%v/%v", response.StatusCode, body, readErr, closeErr)
			}
		}
		previousSetupToken = setupToken
		stop()

		verification, err := pgx.ConnectConfig(ctx, appConfig)
		if err != nil {
			t.Fatal("connect to disposable database for verification failed")
		}
		var count, initialCount, metadataCount int
		err = verification.QueryRow(ctx, `SELECT count(*),
			count(*) FILTER (WHERE version_id = 1 AND is_applied),
			count(*) FILTER (WHERE version_id = 2 AND is_applied)
			FROM public.goose_db_version`).Scan(&count, &initialCount, &metadataCount)
		closeErr := verification.Close(ctx)
		if err != nil || closeErr != nil {
			t.Fatalf("read/close migration verification: %v, %v", err, closeErr)
		}
		if count != 3 || initialCount != 1 || metadataCount != 1 {
			t.Fatalf("startup %d: version records=%d, initial=%d, metadata=%d; want 3/1/1", iteration, count, initialCount, metadataCount)
		}
		if migrationRecords() != beforeRestarts {
			t.Fatal("restart changed migration records")
		}
		var connections int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND pid <> pg_backend_pid()`).Scan(&connections); err != nil {
			t.Fatal(err)
		}
		if connections != 0 {
			t.Fatalf("startup %d: %d database connections remain after shutdown", iteration, connections)
		}
		for line := range bytes.SplitSeq(bytes.TrimSpace(output.Bytes()), []byte("\n")) {
			var event struct {
				Level string `json:"level"`
			}
			if err := json.Unmarshal(line, &event); err != nil {
				t.Fatal(err)
			}
			if event.Level == "ERROR" {
				t.Fatalf("startup %d logged an error: %s", iteration, line)
			}
		}
		if strings.Contains(output.String(), databaseURL) || strings.Contains(output.String(), "postgres://") {
			t.Fatal("startup logs exposed database connection information")
		}
		t.Logf("serve startup %d: health=200, API=404, graceful stop, migrations 1/2 each recorded once and unchanged, zero remaining connections", iteration)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	cfg.Listen = listener.Addr().String()
	logger := newJSONLogger(io.Discard, slog.LevelInfo)
	if err := serve(ctx, cfg, logger); err == nil || !strings.Contains(err.Error(), "listen for HTTP") {
		t.Fatalf("occupied port error=%v, want explicit startup failure", err)
	}
}

type readinessHandler struct {
	slog.Handler
	ready chan string
}

func (handler readinessHandler) Handle(ctx context.Context, record slog.Record) error {
	if err := handler.Handler.Handle(ctx, record); err != nil {
		return err
	}
	if record.Message == "HTTP server listening" {
		record.Attrs(func(attr slog.Attr) bool {
			if attr.Key == "address" {
				handler.ready <- attr.Value.String()
			}
			return true
		})
	}
	return nil
}

func startTestServe(t *testing.T, cfg config.Config) (string, func(), *bytes.Buffer) {
	t.Helper()
	var output bytes.Buffer
	ready := make(chan string, 1)
	logger := slog.New(readinessHandler{Handler: slog.NewJSONHandler(&output, nil), ready: ready})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var serveErr error
	go func() {
		defer close(done)
		serveErr = serve(ctx, cfg, logger)
	}()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			select {
			case <-done:
				if serveErr != nil {
					t.Errorf("serve failed: %v", serveErr)
				}
			case <-time.After(15 * time.Second):
				t.Error("serve did not stop within the shutdown bound")
			}
		})
	}
	t.Cleanup(stop)
	select {
	case address := <-ready:
		return address, stop, &output
	case <-done:
		t.Fatalf("serve failed before readiness: %v", serveErr)
	case <-time.After(15 * time.Second):
		t.Fatal("serve startup timed out")
	}
	return "", stop, &output
}
