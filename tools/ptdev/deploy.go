package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var destinationPattern = regexp.MustCompile(`^([A-Za-z0-9_][A-Za-z0-9_.-]*@)?[A-Za-z0-9][A-Za-z0-9.-]*$`)

func validateDestination(host, dir string) error {
	if !destinationPattern.MatchString(host) {
		return usageError{"deploy requires --host [user@]hostname (DNS name, IPv4 or SSH alias; no options)"}
	}
	if !path.IsAbs(dir) || path.Clean(dir) != dir || dir == "/" || strings.ContainsAny(dir, "\x00\r\n\\:") {
		return usageError{"deploy --dir must be a clean absolute POSIX directory other than /"}
	}
	return nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func deployPreflight(dir string) string {
	return "set -eu; cd " + shellQuote(dir) +
		`; test -f compose.yaml; test -f postgresql.conf; test -f .env; test "$(grep -c '^PT_VERSION=' .env)" -eq 1; docker compose config --quiet; mkdir -p images`
}

func deployStart(dir, tag string, archives []string) string {
	command := "set -eu; cd " + shellQuote(dir)
	for _, archive := range archives {
		command += "; docker load -i " + shellQuote(archive)
	}
	return command + "; sed -i " + shellQuote("s/^PT_VERSION=.*/PT_VERSION="+tag+"/") +
		" .env; docker compose up -d --wait --wait-timeout 60 app; docker compose exec -T app pandatable healthcheck"
}

func (runner runner) deploy(ctx context.Context, opts options) (err error) {
	if err := validateDestination(opts.host, opts.remoteDir); err != nil {
		return err
	}
	if !opts.yes {
		return usageError{"deploy requires --yes"}
	}
	_, tag, err := runner.release(ctx, opts.releaseVersion)
	if err != nil {
		return err
	}
	filenames := []string{"pandatable-" + tag + ".tar.gz"}
	if opts.withPostgres {
		filenames = append(filenames, "postgres-18.tar.gz")
	}
	for _, name := range filenames {
		info, err := os.Stat(filepath.Join(runner.root, "dist", name))
		if err != nil {
			return fmt.Errorf("deploy requires existing image save archives: %w", err)
		}
		if !info.Mode().IsRegular() || info.Size() == 0 {
			return fmt.Errorf("deploy archive %s is empty or not regular", name)
		}
	}
	sshArgs := []string{"-o", "BatchMode=yes", "-o", "ConnectTimeout=10", opts.host}
	if err := runner.run(ctx, "ssh", append(sshArgs, deployPreflight(opts.remoteDir))...); err != nil {
		return fmt.Errorf("remote preflight: %w", err)
	}
	var output bytes.Buffer
	command := runner.command(ctx, "", nil, "ssh", append(sshArgs,
		"mktemp -d "+shellQuote(path.Join(opts.remoteDir, "images", ".ptdev-XXXXXXXX")))...)
	command.Stdout = &output
	if err := command.Run(); err != nil {
		return fmt.Errorf("create remote staging directory: %w", err)
	}
	stage := strings.TrimSpace(output.String())
	prefix := path.Join(opts.remoteDir, "images") + "/.ptdev-"
	if !strings.HasPrefix(stage, prefix) || strings.ContainsAny(strings.TrimPrefix(stage, prefix), "/\x00\r\n\\") || len(stage) <= len(prefix) {
		return errors.New("remote mktemp returned an invalid staging path; no files transferred")
	}
	var remoteArchives []string
	for _, name := range filenames {
		remoteArchives = append(remoteArchives, path.Join(stage, name))
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		command := "set -eu; rm -f"
		for _, archive := range remoteArchives {
			command += " " + shellQuote(archive)
		}
		command += "; rmdir " + shellQuote(stage)
		err = errors.Join(err, runner.run(cleanupCtx, "ssh", append(sshArgs, command)...))
	}()
	// Modern OpenSSH scp uses SFTP: pass remote paths literally, not shell-quoted.
	args := []string{"-o", "BatchMode=yes", "-o", "ConnectTimeout=10"}
	for _, name := range filenames {
		args = append(args, filepath.Join(runner.root, "dist", name))
	}
	args = append(args, opts.host+":"+stage+"/")
	if err := runner.run(ctx, "scp", args...); err != nil {
		return fmt.Errorf("transfer archives: %w", err)
	}
	if err := runner.run(ctx, "ssh", append(sshArgs, deployStart(opts.remoteDir, tag, remoteArchives))...); err != nil {
		return fmt.Errorf("remote load/start failed; inspect server state before retrying: %w", err)
	}
	fmt.Println("Remote load/start and internal health probe passed; verify external HTTPS through Caddy separately.")
	return nil
}
