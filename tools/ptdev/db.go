package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	dbContainer = "pt-dev-db"
	dbVolume    = "pt-dev-pgdata"
	dbDSN       = "postgres://postgres:postgres@127.0.0.1:55432/postgres?sslmode=disable"
)

func (runner runner) dbState(ctx context.Context, name string) (string, error) {
	output, err := runner.execute(ctx, "", nil, true, name, "container", "ls", "-a", "--filter", "name=^"+dbContainer+"$", "--format", "{{.Names}}")
	if err != nil {
		return "", fmt.Errorf("query dev container: %w: %s", err, strings.TrimSpace(output))
	}
	if strings.TrimSpace(output) == "" {
		return "absent", nil
	}
	output, err = runner.execute(ctx, "", nil, true, name, "inspect", "--format", "{{.State.Running}}", dbContainer)
	if err != nil {
		return "", fmt.Errorf("inspect dev container: %w: %s", err, strings.TrimSpace(output))
	}
	if strings.TrimSpace(output) == "true" {
		return "running", nil
	}
	return "stopped", nil
}

func (runner runner) db(ctx context.Context, action string, yes bool) error {
	selected, err := detectEngine(os.Getenv("PT_ENGINE"), exec.LookPath)
	if err != nil {
		return err
	}
	if action == "psql" {
		return runner.run(ctx, selected.name, "exec", "-it", dbContainer, "psql", "-U", "postgres")
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
			return ctx.Err()
		case line := <-answer:
			if !strings.EqualFold(line, "y") && !strings.EqualFold(line, "yes") {
				fmt.Println("Reset cancelled.")
				return nil
			}
		}
	}
	state, err := runner.dbState(ctx, selected.name)
	if err != nil {
		return err
	}
	switch action {
	case "status":
		fmt.Printf("%s: %s\n%s\n", dbContainer, state, dbDSN)
		return nil
	case "down":
		if state == "running" {
			return runner.run(ctx, selected.name, "stop", dbContainer)
		}
		fmt.Println(dbContainer + ": " + state)
		return nil
	case "reset":
		if state != "absent" {
			if err := runner.run(ctx, selected.name, "rm", "-f", dbContainer); err != nil {
				return err
			}
		}
		output, err := runner.execute(ctx, "", nil, true, selected.name, "volume", "ls", "--filter", "name=^"+dbVolume+"$", "--format", "{{.Name}}")
		if err != nil {
			return err
		}
		if strings.TrimSpace(output) != "" {
			if err := runner.run(ctx, selected.name, "volume", "rm", "--force", dbVolume); err != nil {
				return err
			}
		}
		state = "absent"
	}
	if state == "absent" {
		if err := runner.run(ctx, selected.name, "run", "-d", "--name", dbContainer,
			"-e", "POSTGRES_PASSWORD=postgres", "-p", "127.0.0.1:55432:5432",
			"-v", dbVolume+":/var/lib/postgresql", "docker.io/library/postgres:18"); err != nil {
			return err
		}
	} else if state == "stopped" {
		if err := runner.run(ctx, selected.name, "start", dbContainer); err != nil {
			return err
		}
	}
	waitCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	for {
		if _, err := runner.execute(waitCtx, "", nil, true, selected.name, "exec", dbContainer, "pg_isready", "-U", "postgres"); err == nil {
			fmt.Println(dbDSN)
			return nil
		}
		select {
		case <-waitCtx.Done():
			return fmt.Errorf("PostgreSQL readiness (60s limit): %w", waitCtx.Err())
		case <-time.After(time.Second):
		}
	}
}
