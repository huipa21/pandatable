package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	dbContainer = "pt-dev-db"
	dbVolume    = "pt-dev-pgdata"
	dbDSN       = "postgres://postgres:postgres@127.0.0.1:55432/postgres?sslmode=disable"
)

type devDatabase struct {
	command func(context.Context, ...string) (string, error)
}

type dbInspection struct {
	State      struct{ Running bool }
	Config     struct{ Image string }
	HostConfig struct {
		PortBindings map[string][]struct{ HostIP, HostPort string }
	}
	Mounts []struct{ Type, Name, Destination string }
}

func (db devDatabase) inspect(ctx context.Context) (*dbInspection, error) {
	output, err := db.command(ctx, "container", "ls", "-a", "--filter", "name=^"+dbContainer+"$", "--format", "{{.Names}}")
	if err != nil {
		return nil, fmt.Errorf("query dev container: %w: %s", err, strings.TrimSpace(output))
	}
	if strings.TrimSpace(output) == "" {
		return nil, nil
	}
	output, err = db.command(ctx, "inspect", "--format", "{{json .}}", dbContainer)
	if err != nil {
		return nil, fmt.Errorf("inspect dev container: %w: %s", err, strings.TrimSpace(output))
	}
	var info dbInspection
	if err := json.Unmarshal([]byte(output), &info); err != nil {
		return nil, fmt.Errorf("decode dev container: %w", err)
	}
	return &info, nil
}

func (info *dbInspection) port() (string, error) {
	bindings := info.HostConfig.PortBindings["5432/tcp"]
	if len(bindings) != 1 || bindings[0].HostIP != "127.0.0.1" {
		return "", fmt.Errorf("%s must publish PostgreSQL on exactly one IPv4 loopback port", dbContainer)
	}
	port := bindings[0].HostPort
	if number, err := strconv.Atoi(port); err != nil || number < 1 || number > 65535 {
		return "", fmt.Errorf("%s has invalid PostgreSQL host port %q", dbContainer, port)
	}
	return port, nil
}

func (info *dbInspection) canRecreate() error {
	if info.State.Running || info.Config.Image == "" || len(info.Mounts) != 1 ||
		info.Mounts[0].Type != "volume" || info.Mounts[0].Name != dbVolume ||
		info.Mounts[0].Destination != "/var/lib/postgresql" {
		return fmt.Errorf("refusing to recreate %s: expected a stopped container using only volume %s at /var/lib/postgresql", dbContainer, dbVolume)
	}
	return nil
}

func databaseDSN(port string) string {
	return strings.Replace(dbDSN, "127.0.0.1:55432", net.JoinHostPort("127.0.0.1", port), 1)
}

func databasePort(preferred string) (string, error) {
	listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", preferred))
	if err != nil && preferred != "0" {
		fmt.Fprintf(os.Stderr, "Dev DB port %s is unavailable (%v); selecting a free loopback port.\n", preferred, err)
		listener, err = net.Listen("tcp4", "127.0.0.1:0")
	}
	if err != nil {
		return "", fmt.Errorf("select dev DB port: %w", err)
	}
	_, port, splitErr := net.SplitHostPort(listener.Addr().String())
	return port, errors.Join(splitErr, listener.Close())
}

func dbPortConflict(output string) bool {
	output = strings.ToLower(output)
	for _, marker := range []string{"address already in use", "port is already allocated", "ports are not available", "cannot listen on the tcp port", "forbidden by its access permissions"} {
		if strings.Contains(output, marker) {
			return true
		}
	}
	return false
}

func (db devDatabase) up(ctx context.Context) (string, error) {
	image := "docker.io/library/postgres:18"
	for attempt := range 3 {
		info, err := db.inspect(ctx)
		if err != nil {
			return "", err
		}
		port := "55432"
		if info != nil {
			port, err = info.port()
			if err != nil {
				return "", err
			}
			if info.State.Running {
				return db.wait(ctx, port)
			}
			image = info.Config.Image
		}
		preferred := port
		if attempt > 0 {
			preferred = "0"
		}
		port, err = databasePort(preferred)
		if err != nil {
			return "", err
		}
		if info != nil && (port != preferred || attempt > 0) {
			if err := info.canRecreate(); err != nil {
				return "", err
			}
			fmt.Fprintf(os.Stderr, "Recreating stopped %s on port %s; retaining volume %s.\n", dbContainer, port, dbVolume)
			if output, err := db.command(ctx, "rm", dbContainer); err != nil {
				return "", fmt.Errorf("remove stopped dev container: %w: %s", err, strings.TrimSpace(output))
			}
			info = nil
		}
		var output string
		if info == nil {
			output, err = db.command(ctx, "run", "-d", "--name", dbContainer,
				"-e", "POSTGRES_PASSWORD=postgres", "-p", "127.0.0.1:"+port+":5432",
				"-v", dbVolume+":/var/lib/postgresql", image)
		} else {
			output, err = db.command(ctx, "start", dbContainer)
		}
		if err == nil {
			return db.wait(ctx, port)
		}
		if !dbPortConflict(output) || ctx.Err() != nil || attempt == 2 {
			return "", fmt.Errorf("start dev DB: %w: %s", err, strings.TrimSpace(output))
		}
		fmt.Fprintf(os.Stderr, "Dev DB port conflict during startup: %s; retrying with another port.\n", strings.TrimSpace(output))
	}
	return "", fmt.Errorf("dev DB startup exhausted port retries")
}

func (db devDatabase) wait(ctx context.Context, port string) (string, error) {
	waitCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	for {
		output, err := db.command(waitCtx, "exec", dbContainer, "pg_isready", "-U", "postgres")
		if err == nil {
			return databaseDSN(port), nil
		}
		select {
		case <-waitCtx.Done():
			return "", fmt.Errorf("PostgreSQL readiness (60s limit): %w; last probe: %v: %s", waitCtx.Err(), err, strings.TrimSpace(output))
		case <-time.After(time.Second):
		}
	}
}

func (runner runner) db(ctx context.Context, action string, yes bool) error {
	_, err := runner.database(ctx, action, yes)
	return err
}

func (runner runner) database(ctx context.Context, action string, yes bool) (string, error) {
	selected, err := runner.readyEngine(ctx)
	if err != nil {
		return "", err
	}
	if action == "psql" {
		return "", runner.run(ctx, selected.name, "exec", "-it", dbContainer, "psql", "-U", "postgres")
	}
	if action == "reset" && !yes {
		fmt.Print("Delete the dev database container and ALL data? [y/N] ")
		answer := make(chan string, 1)
		go func() {
			line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
			answer <- strings.TrimSpace(line)
		}()
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case line := <-answer:
			if !strings.EqualFold(line, "y") && !strings.EqualFold(line, "yes") {
				fmt.Println("Reset cancelled.")
				return "", nil
			}
		}
	}
	db := devDatabase{command: func(ctx context.Context, args ...string) (string, error) {
		return runner.engineCommand(ctx, selected.name, args...)
	}}
	if action == "up" {
		dsn, err := db.up(ctx)
		if err == nil {
			fmt.Println(dsn)
		}
		return dsn, err
	}
	info, err := db.inspect(ctx)
	if err != nil {
		return "", err
	}
	state := "absent"
	dsn := ""
	if info != nil {
		state = "stopped"
		if info.State.Running {
			state = "running"
		}
		port, err := info.port()
		if err != nil {
			return "", err
		}
		dsn = databaseDSN(port)
	}
	switch action {
	case "status":
		fmt.Printf("%s: %s\n", dbContainer, state)
		if dsn != "" {
			fmt.Println(dsn)
		}
		return dsn, nil
	case "down":
		if state == "running" {
			return "", runner.run(ctx, selected.name, "stop", dbContainer)
		}
		fmt.Println(dbContainer + ": " + state)
		return "", nil
	case "reset":
		if state != "absent" {
			if err := runner.run(ctx, selected.name, "rm", "-f", dbContainer); err != nil {
				return "", err
			}
		}
		output, err := runner.execute(ctx, "", nil, true, selected.name, "volume", "ls", "--filter", "name=^"+dbVolume+"$", "--format", "{{.Name}}")
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(output) != "" {
			if err := runner.run(ctx, selected.name, "volume", "rm", "--force", dbVolume); err != nil {
				return "", err
			}
		}
	}
	dsn, err = db.up(ctx)
	if err == nil {
		fmt.Println(dsn)
	}
	return dsn, err
}
