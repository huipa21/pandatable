package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDevIntegration(t *testing.T) {
	databaseURL := os.Getenv("PTDEV_TEST_DEV_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PTDEV_TEST_DEV_DATABASE_URL is not set; use a disposable pt_test_* database for the real dev-process test")
	}
	parsed, err := url.Parse(databaseURL)
	if err != nil || !strings.HasPrefix(parsed.Path, "/pt_test_") {
		t.Fatal("PTDEV_TEST_DEV_DATABASE_URL must name a disposable pt_test_* database")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	backendAddress := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	viteAddress := "127.0.0.1:5173"
	probe, err := net.Listen("tcp", viteAddress)
	if err != nil {
		t.Fatal("Vite port 5173 must be free for the opt-in dev integration test")
	}
	if err := probe.Close(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PT_DATABASE_URL", databaseURL)
	t.Setenv("PT_LISTEN", backendAddress)
	t.Setenv("PT_BASE_URL", "http://"+viteAddress)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	done := make(chan error, 1)
	go func() { done <- (runner{root: root}).dev(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("dev shutdown: %v", err)
			}
		case <-time.After(15 * time.Second):
			t.Error("dev failed to reap its processes within 15 seconds")
		}
		for _, address := range []string{backendAddress, viteAddress} {
			connection, err := net.DialTimeout("tcp", address, time.Second)
			if err == nil {
				connection.Close()
				t.Errorf("dev listener remains at %s", address)
			}
		}
	})
	client := &http.Client{Timeout: time.Second}
	health := func(address string) bool {
		response, err := client.Get("http://" + address + "/healthz")
		if err != nil {
			return false
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		return err == nil && response.StatusCode == http.StatusOK && string(body) == "{\"status\":\"ok\"}\n"
	}
	for !health(backendAddress) || !health(viteAddress) {
		select {
		case err := <-done:
			done <- err
			t.Fatalf("dev exited before readiness: %v", err)
		case <-ctx.Done():
			t.Fatal("dev startup/proxy readiness timed out")
		case <-time.After(100 * time.Millisecond):
		}
	}
	for _, test := range []struct {
		path, contentType, contains string
		status                      int
	}{
		{"/", "text/html", "/src/main.ts", http.StatusOK},
		{"/api/v1/missing", "application/json", "ERR_NOT_FOUND", http.StatusNotFound},
	} {
		response, err := client.Get("http://" + viteAddress + test.path)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || response.StatusCode != test.status ||
			!strings.Contains(response.Header.Get("Content-Type"), test.contentType) ||
			!strings.Contains(string(body), test.contains) {
			t.Fatalf("dev %s: status=%d, content-type=%q, read=%v", test.path, response.StatusCode, response.Header.Get("Content-Type"), err)
		}
	}
	t.Log("real backend and Vite ready; health/API proxy and dev HTML pass; cleanup asserts both listeners close")
}
