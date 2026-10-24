package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

func healthcheck(ctx context.Context, listen string) error {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return errors.New("invalid healthcheck listen address")
	}
	switch host {
	case "", "0.0.0.0":
		host = "127.0.0.1"
	case "::":
		host = "::1"
	}
	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			Proxy:             nil,
			DisableKeepAlives: true,
		},
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	defer client.CloseIdleConnections()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+net.JoinHostPort(host, port)+"/healthz", nil)
	if err != nil {
		return errors.New("invalid healthcheck target")
	}
	response, err := client.Do(request)
	if err != nil {
		return errors.New("healthcheck HTTP request failed (2s limit)")
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("healthcheck returned HTTP %d", response.StatusCode)
	}
	return nil
}
