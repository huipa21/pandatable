package main

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHealthcheckCLI(t *testing.T) {
	t.Setenv("PT_DATABASE_URL", "")
	t.Setenv("PT_BASE_URL", "")
	t.Setenv("PT_LOG_LEVEL", "invalid")
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	for _, status := range []int{200, 204, 301, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/healthz" {
					t.Errorf("unexpected probe: %s %s", r.Method, r.URL.Path)
				}
				w.Header().Set("Location", "http://127.0.0.1:1/secret")
				w.WriteHeader(status)
			}))
			defer server.Close()
			t.Setenv("PT_LISTEN", strings.TrimPrefix(server.URL, "http://"))
			var stdout, stderr bytes.Buffer
			code := runWith([]string{"healthcheck"}, &stdout, &stderr)
			if status == 200 {
				if code != 0 || stdout.Len() != 0 || stderr.Len() != 0 {
					t.Fatalf("success: %d, %s, %s", code, &stdout, &stderr)
				}
			} else if code != 1 || !strings.Contains(stdout.String(), "healthcheck failed") || strings.Contains(stdout.String(), "secret") {
				t.Fatalf("failure: %d, %s, %s", code, &stdout, &stderr)
			}
		})
	}
}

func TestHealthcheckWildcardAndFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	host, port, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	for _, listen := range []string{net.JoinHostPort(host, port), ":" + port, "0.0.0.0:" + port} {
		if err := healthcheck(context.Background(), listen); err != nil {
			t.Errorf("%s: %v", listen, err)
		}
	}
	server.Close()
	if err := healthcheck(context.Background(), net.JoinHostPort(host, port)); err == nil {
		t.Fatal("closed listener succeeded")
	}
	if err := healthcheck(context.Background(), "invalid"); err == nil {
		t.Fatal("invalid listen succeeded")
	}
}

func TestHealthcheckIsBounded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := healthcheck(ctx, strings.TrimPrefix(server.URL, "http://")); err == nil {
		t.Fatal("hanging handler succeeded")
	}
	if time.Since(start) > time.Second {
		t.Fatal("probe ignored cancellation")
	}
	start = time.Now()
	if err := healthcheck(context.Background(), strings.TrimPrefix(server.URL, "http://")); err == nil {
		t.Fatal("hanging handler succeeded")
	}
	if elapsed := time.Since(start); elapsed < time.Second || elapsed > 3*time.Second {
		t.Fatalf("probe timeout = %s, want ~2s", elapsed)
	}
}

func TestHealthcheckArguments(t *testing.T) {
	t.Setenv("PT_LISTEN", ":8080")
	for _, test := range []struct {
		args []string
		code int
	}{
		{[]string{"healthcheck", "--help"}, 0},
		{[]string{"healthcheck", "extra"}, 2},
		{[]string{"healthcheck", "--unknown"}, 2},
		{[]string{"healthcheck", "--listen", ":0"}, 1},
	} {
		var stdout, stderr bytes.Buffer
		if code := runWith(test.args, &stdout, &stderr); code != test.code {
			t.Errorf("%v: %d, want %d", test.args, code, test.code)
		}
	}
}
