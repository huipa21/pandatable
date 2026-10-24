package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

const shutdownTimeout = 10 * time.Second

func Run(ctx context.Context, listener net.Listener, handler http.Handler, logger *slog.Logger) error {
	server := &http.Server{
		Handler:           handler,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	return serve(ctx, server, listener, logger, shutdownTimeout)
}

func serve(ctx context.Context, server *http.Server, listener net.Listener, logger *slog.Logger, timeout time.Duration) error {
	requestCtx, cancelRequests := context.WithCancel(context.Background())
	defer cancelRequests()
	server.BaseContext = func(net.Listener) context.Context { return requestCtx }
	defer func() { _ = server.Close() }()
	if ctx.Err() != nil {
		return listener.Close()
	}
	result := make(chan error, 1)
	go func() {
		result <- server.Serve(listener)
	}()
	logger.Info("HTTP server listening", "address", listener.Addr().String())
	select {
	case err := <-result:
		return fmt.Errorf("HTTP server stopped unexpectedly: %w", err)
	case <-ctx.Done():
	}
	logger.Info("HTTP server shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	err := server.Shutdown(shutdownCtx)
	if err != nil {
		cancelRequests()
		err = errors.Join(fmt.Errorf("HTTP shutdown: %w", err), server.Close())
	}
	serveErr := <-result
	if !errors.Is(serveErr, http.ErrServerClosed) {
		err = errors.Join(err, fmt.Errorf("HTTP server: %w", serveErr))
	}
	if err == nil {
		logger.Info("HTTP server stopped")
	}
	return err
}
