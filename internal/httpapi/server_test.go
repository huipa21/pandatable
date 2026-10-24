package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestServerGracefullyDrainsRequests(t *testing.T) {
	listener := testListener(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started, release := make(chan struct{}), make(chan struct{})
	requestCanceled := make(chan struct{}, 1)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-release:
			_, _ = w.Write([]byte("complete"))
		case <-r.Context().Done():
			requestCanceled <- struct{}{}
		}
	})
	result := make(chan error, 1)
	go func() { result <- Run(ctx, listener, handler, testLogger()) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-release:
		default:
			close(release)
		}
	})
	response := make(chan error, 1)
	go func() {
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get("http://" + listener.Addr().String())
		if err == nil {
			defer resp.Body.Close()
			var body []byte
			body, err = io.ReadAll(resp.Body)
			if err == nil && string(body) != "complete" {
				err = errors.New("in-flight response was not completed")
			}
		}
		response <- err
	}()
	waitSignal(t, started)
	cancel()
	select {
	case err := <-result:
		t.Fatalf("server exited before in-flight request completed: %v", err)
	case <-requestCanceled:
		t.Fatal("shutdown canceled the in-flight request before the drain deadline")
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	if err := waitError(t, response); err != nil {
		t.Fatal(err)
	}
	if err := waitError(t, result); err != nil {
		t.Fatal(err)
	}
	connection, err := net.DialTimeout("tcp", listener.Addr().String(), 100*time.Millisecond)
	if err == nil {
		connection.Close()
		t.Fatal("listener still accepts connections after shutdown")
	}
}

func TestServerShutdownDeadlineClosesConnections(t *testing.T) {
	listener := testListener(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started, finished := make(chan struct{}), make(chan struct{})
	handler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
		close(finished)
	})
	result := make(chan error, 1)
	server := &http.Server{Handler: handler, ReadHeaderTimeout: time.Second}
	go func() { result <- serve(ctx, server, listener, testLogger(), 30*time.Millisecond) }()
	requestResult := make(chan error, 1)
	go func() {
		client := &http.Client{Timeout: 5 * time.Second}
		response, err := client.Get("http://" + listener.Addr().String())
		if response != nil {
			response.Body.Close()
		}
		requestResult <- err
	}()
	waitSignal(t, started)
	cancel()
	if err := waitError(t, result); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shutdown error=%v, want deadline exceeded", err)
	}
	waitSignal(t, finished)
	_ = waitError(t, requestResult)
}

func TestServerReportsRuntimeFailure(t *testing.T) {
	listener := testListener(t)
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	err := Run(context.Background(), listener, http.NotFoundHandler(), testLogger())
	if err == nil {
		t.Fatal("closed listener must report a runtime error")
	}
}

func TestServerCanceledBeforeStartup(t *testing.T) {
	listener := testListener(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Run(ctx, listener, http.NotFoundHandler(), testLogger()); err != nil {
		t.Fatal(err)
	}
	if _, err := listener.Accept(); err == nil {
		t.Fatal("listener was not closed")
	}
}

func testListener(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	return listener
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

func waitSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for server synchronization")
	}
}

func waitError(t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for server result")
		return nil
	}
}
