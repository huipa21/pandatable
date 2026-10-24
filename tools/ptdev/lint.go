package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

func (runner runner) lint(ctx context.Context) error {
	output, err := runner.execute(ctx, "", nil, true, "gofmt", "-l", ".")
	if err != nil {
		return fmt.Errorf("gofmt: %w: %s", err, output)
	}
	if strings.TrimSpace(output) != "" {
		return fmt.Errorf("gofmt required:\n%s", output)
	}
	if err := runner.run(ctx, "go", "vet", "./..."); err != nil {
		return err
	}
	if _, err := exec.LookPath("golangci-lint"); err == nil {
		if err := runner.run(ctx, "golangci-lint", "run"); err != nil {
			return err
		}
	} else {
		fmt.Println("warning: golangci-lint not on PATH; install v2: https://golangci-lint.run/docs/welcome/install/")
	}
	if runner.webExists() {
		return runner.pnpm(ctx, "lint")
	}
	return nil
}
