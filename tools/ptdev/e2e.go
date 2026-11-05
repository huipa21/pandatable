package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type e2eSteps struct {
	build, up, ready, test, diagnostics, down, removeImage func(context.Context) error
}

var setupTokenLogPattern = regexp.MustCompile(`SETUP_TOKEN=([A-Za-z0-9_-]{43})`)
var composeContainerIDPattern = regexp.MustCompile(`(?i)\b[0-9a-f]{12,64}\b`)

func composeContainerIDs(output string) []string {
	return composeContainerIDPattern.FindAllString(output, -1)
}

func setupTokenFromLogs(logs string) (string, error) {
	matches := setupTokenLogPattern.FindAllStringSubmatch(logs, -1)
	if len(matches) != 1 {
		return "", errors.New("expected exactly one setup token in the owned app startup log")
	}
	return matches[0][1], nil
}

func redactSetupToken(logs string) string {
	return setupTokenLogPattern.ReplaceAllString(logs, "SETUP_TOKEN=[REDACTED]")
}

func runE2E(ctx context.Context, steps e2eSteps) (err error) {
	started := false
	defer func() {
		if started && err != nil {
			diagnosticCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			err = errors.Join(err, steps.diagnostics(diagnosticCtx))
			cancel()
		}
		if started {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			err = errors.Join(err, steps.down(cleanupCtx))
			cancel()
		}
		imageCtx, cancelImage := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancelImage()
		err = errors.Join(err, steps.removeImage(imageCtx))
	}()
	buildStart := time.Now()
	if err := steps.build(ctx); err != nil {
		return fmt.Errorf("e2e image build: %w", errors.Join(err, ctx.Err()))
	}
	fmt.Printf("E2E image build: %s\n", time.Since(buildStart).Round(time.Second))
	start := time.Now()
	readyCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	started = true
	if err := steps.up(readyCtx); err != nil {
		return fmt.Errorf("e2e Compose startup: %w", errors.Join(err, ctx.Err()))
	}
	if err := steps.ready(readyCtx); err != nil {
		return fmt.Errorf("e2e health readiness (60s): %w", err)
	}
	fmt.Printf("E2E health ready in %s\n", time.Since(start).Round(time.Millisecond))
	if err := steps.test(ctx); err != nil {
		return fmt.Errorf("e2e Playwright: %w", errors.Join(err, ctx.Err()))
	}
	return ctx.Err()
}

func e2eComposeArgs(root, envFile, project string, args ...string) []string {
	base := []string{"compose", "--env-file", envFile, "-p", project,
		"-f", filepath.Join(root, "deploy", "compose.yaml"),
		"-f", filepath.Join(root, "deploy", "compose.local.yaml")}
	return append(base, args...)
}

func e2eImageBuildArgs(engine, tag string, fast bool) []string {
	args := imageBuildArgs(engine, tag, tag, false)
	if !fast {
		return args
	}
	return append(args[:len(args)-1:len(args)-1], "--target", "prebuilt", ".")
}

func (runner runner) e2e(ctx context.Context, opts options) (err error) {
	cli := filepath.Join(runner.root, "web", "node_modules", "@playwright", "test", "cli.js")
	if !exists(cli) {
		return errors.New("Playwright is missing; run pnpm --dir web install --frozen-lockfile, then pnpm --dir web e2e:install")
	}
	selected, err := runner.readyEngine(ctx)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("reserve E2E loopback port: %w", err)
	}
	_, port, err := net.SplitHostPort(listener.Addr().String())
	err = errors.Join(err, listener.Close())
	if err != nil {
		return err
	}
	project := "pt-e2e-" + strings.ToLower(rand.Text())
	tag := "0.0.0-" + project
	baseURL := "http://127.0.0.1:" + port
	password := rand.Text()
	var smtpKey [32]byte
	if _, err := rand.Read(smtpKey[:]); err != nil {
		return fmt.Errorf("generate E2E SMTP key: %w", err)
	}
	dir, err := os.MkdirTemp("", "ptdev-e2e-")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(dir)) }()
	envFile := filepath.Join(dir, "empty.env")
	if err := os.WriteFile(envFile, nil, 0o600); err != nil {
		return err
	}
	results := filepath.Join(runner.root, "web", "test-results", project)
	if err := os.MkdirAll(results, 0o750); err != nil {
		return err
	}
	env := map[string]string{
		"COMPOSE_PROJECT_NAME": project, "PT_VERSION": tag,
		"PT_DB_PASSWORD": password, "PT_LOCAL_HTTP_PORT": port,
		"PT_BASE_URL": baseURL, "PT_TRUSTED_PROXIES": "",
		"PT_SMTP_KEY":     base64.StdEncoding.EncodeToString(smtpKey[:]),
		"PT_E2E_BASE_URL": baseURL, "PT_E2E_RESULTS_DIR": results,
		"PT_DB_NAME": "pandatable", "PT_E2E_PERF": "",
	}
	if opts.perf {
		env["PT_DB_NAME"] = "pt_seed_" + strings.ToLower(rand.Text())
		env["PT_E2E_PERF"] = "1"
	}
	compose := func(ctx context.Context, capture bool, args ...string) (string, error) {
		return runner.execute(ctx, "", env, capture, selected.name, e2eComposeArgs(runner.root, envFile, project, args...)...)
	}
	fmt.Printf("E2E project: %s; URL: %s; results: %s\n", project, baseURL, results)
	return runE2E(ctx, e2eSteps{
		build: func(ctx context.Context) error {
			if opts.fast {
				if err := runner.buildVersion(ctx, true, tag); err != nil {
					return err
				}
			}
			return runner.run(ctx, selected.name, e2eImageBuildArgs(selected.name, tag, opts.fast)...)
		},
		up: func(ctx context.Context) error {
			_, err := compose(ctx, false, "up", "-d")
			return err
		},
		ready: func(ctx context.Context) error { return waitE2EHealth(ctx, baseURL) },
		test: func(ctx context.Context) error {
			containers, err := runner.execute(ctx, "", nil, true, selected.name,
				"ps", "--filter", "label=com.docker.compose.project="+project,
				"--filter", "label=com.docker.compose.service=app", "--format", "{{.ID}}")
			containerIDs := composeContainerIDs(containers)
			if err != nil || len(containerIDs) != 1 {
				return errors.New("find owned app container for resource snapshot")
			}
			for _, fixture := range []struct{ directory, size string }{{"files", "4096"}, {"backups", "2048"}} {
				if err := runner.run(ctx, selected.name, "exec", "--user", "10001:10001", containerIDs[0],
					"dd", "if=/dev/zero", "of=/data/"+fixture.directory+"/pt-e2e-dashboard-fixture",
					"bs="+fixture.size, "count=1", "status=none"); err != nil {
					return errors.New("create dashboard fixture in owned disposable app storage")
				}
			}
			statsArgs := []string{"stats", "--no-stream", "--format", "{{.Name}} {{.MemUsage}} {{.CPUPerc}}"}
			statsArgs = append(statsArgs, containerIDs...)
			stats, err := runner.execute(ctx, "", nil, true, selected.name, statsArgs...)
			if err != nil {
				return errors.New("read owned app resource snapshot")
			}
			fmt.Printf("E2E app resource snapshot: %s\n", strings.TrimSpace(stats))
			logs, err := compose(ctx, true, "logs", "--no-color", "--tail", "100", "app")
			if err != nil {
				return errors.New("read setup token from owned app startup log")
			}
			setupToken, err := setupTokenFromLogs(logs)
			if err != nil {
				return err
			}
			testEnv := make(map[string]string, len(env)+1)
			for key, value := range env {
				testEnv[key] = value
			}
			testEnv["PT_E2E_SETUP_TOKEN"] = setupToken
			testEnv["PT_E2E_ENGINE"] = selected.name
			testEnv["PT_E2E_APP_CONTAINER"] = containerIDs[0]
			if opts.perf {
				return runner.performance(ctx, selected.name, project, baseURL, results, testEnv, compose)
			}
			args := append([]string{filepath.Join(runner.root, "web", "e2e", "run.mjs"), "test"}, opts.e2eArgs...)
			command := runner.command(ctx, "web", testEnv, "node", args...)
			return runCooperative(command)
		},
		diagnostics: func(ctx context.Context) error {
			output, commandErr := compose(ctx, true, "logs", "--no-color", "--tail", "200")
			output = strings.ReplaceAll(output, password, "[REDACTED]")
			output = redactSetupToken(output)
			fileErr := os.WriteFile(filepath.Join(results, "compose.log"), []byte(output), 0o600)
			fmt.Fprintln(os.Stderr, output)
			return errors.Join(commandErr, fileErr)
		},
		down: func(ctx context.Context) error {
			_, err := compose(ctx, false, "down", "--volumes", "--remove-orphans", "--timeout", "10")
			if err != nil {
				return fmt.Errorf("cleanup owned Compose project %s: %w", project, err)
			}
			return nil
		},
		removeImage: func(ctx context.Context) error {
			output, err := runner.execute(ctx, "", nil, true, selected.name,
				"image", "ls", "localhost/pandatable:"+tag, "--format", "{{.Id}}")
			if err != nil {
				return fmt.Errorf("query owned E2E image: %w", err)
			}
			if strings.TrimSpace(output) == "" {
				fmt.Println("E2E image tag absent; nothing to remove")
				return nil
			}
			return runner.run(ctx, selected.name, "image", "rm", "localhost/pandatable:"+tag)
		},
	})
}

// EOF asks the Node runner to interrupt Playwright cooperatively on Windows too,
// so its workers and browsers shut down before the direct child is reaped.
func runCooperative(command *exec.Cmd) (err error) {
	command.Stdin = nil
	input, err := command.StdinPipe()
	if err != nil {
		return err
	}
	defer func() { _ = input.Close() }()
	command.Cancel = input.Close
	command.WaitDelay = 20 * time.Second
	return command.Run()
}

func waitE2EHealth(ctx context.Context, baseURL string) error {
	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			Proxy: nil,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	defer client.CloseIdleConnections()
	var last string
	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/healthz", nil)
		if err != nil {
			return err
		}
		response, err := client.Do(request)
		if err != nil {
			last = err.Error()
		} else {
			body, readErr := io.ReadAll(io.LimitReader(response.Body, 1024))
			closeErr := response.Body.Close()
			if response.StatusCode == http.StatusOK && readErr == nil && closeErr == nil &&
				strings.TrimSpace(string(body)) == `{"status":"ok"}` {
				return nil
			}
			last = fmt.Sprintf("HTTP %d or invalid health body", response.StatusCode)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%w (last probe: %s)", ctx.Err(), last)
		case <-time.After(200 * time.Millisecond):
		}
	}
}
