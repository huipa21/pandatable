package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type composeCommand func(context.Context, bool, ...string) (string, error)

const performanceRows = 200_000

// Keep ptdev stdlib-only: this is the CLI wire result, not a service import.
type seedManifest struct {
	Database      string            `json:"database"`
	WorkspaceID   string            `json:"workspaceId,omitempty"`
	BookID        string            `json:"bookId,omitempty"`
	TableID       string            `json:"tableId,omitempty"`
	ViewID        string            `json:"viewId,omitempty"`
	FieldIDs      map[string]string `json:"fieldIds,omitempty"`
	RequestedRows int               `json:"requestedRows"`
	CommittedRows int               `json:"committedRows"`
	AttemptedRows int               `json:"attemptedRows"`
	Stage         string            `json:"stage"`
	Complete      bool              `json:"complete"`
}

var terminalSGR = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func seedManifestFromOutput(output string) (seedManifest, error) {
	var result seedManifest
	found := 0
	for _, line := range strings.Split(terminalSGR.ReplaceAllString(output, ""), "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "{") {
			continue
		}
		var candidate seedManifest
		if err := json.Unmarshal([]byte(line), &candidate); err != nil {
			return result, errors.New("invalid developer seed result JSON")
		}
		if candidate.Stage != "" {
			result = candidate
			found++
		}
	}
	if found != 1 || !result.Complete || result.CommittedRows != performanceRows ||
		result.RequestedRows != performanceRows || result.TableID == "" || result.ViewID == "" {
		return result, errors.New("developer seed did not confirm the complete 200000-row fixture")
	}
	return result, nil
}

func performanceOwner(ctx context.Context, baseURL string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/v1/auth/login",
		strings.NewReader(`{"login":"first-admin","password":"Setup-password-123"}`))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", baseURL)
	request.Header.Set("X-PT-CSRF", "1")
	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: &http.Transport{Proxy: nil},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	defer client.CloseIdleConnections()
	response, err := client.Do(request)
	if err != nil {
		return "", errors.New("performance owner login failed")
	}
	defer func() { _ = response.Body.Close() }()
	var body struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if response.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(response.Body, 64*1024)).Decode(&body) != nil ||
		body.User.ID == "" {
		return "", errors.New("performance owner login did not return an authenticated user")
	}
	return body.User.ID, nil
}

func (runner runner) performance(ctx context.Context, engine, project, baseURL, results string, env map[string]string, compose composeCommand) error {
	env["PT_E2E_RESULTS_DIR"] = filepath.Join(results, "browser")
	playwright := func(args ...string) error {
		command := runner.command(ctx, "web", env, "node",
			append([]string{filepath.Join(runner.root, "web", "e2e", "run.mjs"), "test"}, args...)...)
		return runCooperative(command)
	}
	if err := playwright("--project=setup-chromium"); err != nil {
		return err
	}
	owner, err := performanceOwner(ctx, baseURL)
	if err != nil {
		return err
	}
	seedStart := time.Now()
	seedCtx, cancel := context.WithTimeout(ctx, 10*time.Minute+15*time.Second)
	output, seedErr := compose(seedCtx, true, "exec", "-T", "app", "pandatable", "dev", "seed",
		"--rows", "200000", "--confirm-database", env["PT_DB_NAME"], "--owner-id", owner, "--yes-dev-seed")
	cancel()
	if err := os.WriteFile(filepath.Join(results, "seed-command.log"),
		[]byte(redactSetupToken(strings.ReplaceAll(output, env["PT_DB_PASSWORD"], "[REDACTED]"))), 0o600); err != nil {
		return err
	}
	manifest, parseErr := seedManifestFromOutput(output)
	if manifest.Database != env["PT_DB_NAME"] {
		parseErr = errors.Join(parseErr, errors.New("seed result names a different database"))
	}
	if manifest.Database == "" {
		manifest.Database = env["PT_DB_NAME"]
		manifest.RequestedRows = performanceRows
		manifest.Stage = "seed-command-no-manifest"
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(results, "seed.json"), data, 0o600); err != nil {
		return err
	}
	if seedErr != nil || parseErr != nil {
		return errors.Join(errors.New("performance seed failed; recorded fixture remains owned by this disposable stack"), seedErr, parseErr)
	}
	fmt.Printf("Performance seed: %d committed rows in %s; table %s; view %s\n",
		manifest.CommittedRows, time.Since(seedStart).Round(time.Millisecond), manifest.TableID, manifest.ViewID)
	env["PT_E2E_PERF_TABLE"] = manifest.TableID
	env["PT_E2E_PERF_VIEW"] = manifest.ViewID
	reportPath := filepath.Join(results, "performance.json")
	env["PT_E2E_PERF_REPORT"] = reportPath
	info, err := runner.execute(ctx, "", nil, true, engine, "info", "--format", "{{json .}}")
	if err != nil {
		return fmt.Errorf("read performance engine host: %w", err)
	}
	host, err := performanceHost(info)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(results, "host.json"), host, 0o600); err != nil {
		return err
	}
	sampleCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- runner.samplePerformance(sampleCtx, engine, project, results) }()
	browserErr := playwright("performance.spec.ts", "--project=desktop-chromium", "--no-deps")
	stop()
	_, reportErr := os.Stat(reportPath)
	if reportErr != nil {
		reportErr = fmt.Errorf("browser performance report was not persisted: %w", reportErr)
	}
	return errors.Join(browserErr, reportErr, <-done)
}

func performanceHost(info string) ([]byte, error) {
	var parsed struct {
		NCPU            int    `json:"NCPU"`
		MemTotal        uint64 `json:"MemTotal"`
		OperatingSystem string `json:"OperatingSystem"`
		KernelVersion   string `json:"KernelVersion"`
		Host            struct {
			CPUs     int    `json:"cpus"`
			MemTotal uint64 `json:"memTotal"`
			OS       string `json:"os"`
			Kernel   string `json:"kernel"`
			Distro   struct {
				Distribution string `json:"distribution"`
				Version      string `json:"version"`
			} `json:"distribution"`
		} `json:"host"`
	}
	if err := json.Unmarshal([]byte(info), &parsed); err != nil {
		return nil, errors.New("invalid performance engine host JSON")
	}
	cpus, memory, osName, kernel := parsed.NCPU, parsed.MemTotal, parsed.OperatingSystem, parsed.KernelVersion
	if parsed.Host.CPUs != 0 {
		cpus, memory, kernel = parsed.Host.CPUs, parsed.Host.MemTotal, parsed.Host.Kernel
		osName = strings.TrimSpace(parsed.Host.OS + " " + parsed.Host.Distro.Distribution + " " + parsed.Host.Distro.Version)
	}
	if cpus < 1 || memory == 0 || osName == "" || kernel == "" {
		return nil, errors.New("performance engine host resources are unavailable")
	}
	return json.MarshalIndent(struct {
		ObservedAt time.Time `json:"observedAt"`
		CPUs       int       `json:"cpus"`
		RAMBytes   uint64    `json:"ramBytes"`
		OS         string    `json:"os"`
		Kernel     string    `json:"kernel"`
		Boundary   string    `json:"boundary"`
	}{time.Now().UTC(), cpus, memory, osName, kernel,
		"Engine-visible host, not container quotas or target-host certification; verify physical host/VM allocation separately."}, "", "  ")
}

func (runner runner) samplePerformance(ctx context.Context, engine, project, results string) (err error) {
	file, err := os.OpenFile(filepath.Join(results, "resources.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		sampleCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		output, sampleErr := runner.execute(sampleCtx, "", nil, true, engine, "stats", "--no-stream",
			"--format", "{{.Name}} {{.MemUsage}} {{.CPUPerc}}", "--", project+"-app-1", project+"-db-1")
		cancel()
		if ctx.Err() != nil {
			return nil
		}
		if sampleErr != nil {
			return fmt.Errorf("sample performance stack resources: %w", sampleErr)
		}
		if err := json.NewEncoder(file).Encode(struct {
			At    time.Time `json:"at"`
			Stats string    `json:"stats"`
		}{time.Now().UTC(), strings.TrimSpace(output)}); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
