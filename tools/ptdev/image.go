package main

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const postgresImage = "docker.io/library/postgres:18.6-bookworm"

var versionPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.+-]{0,127}$`)

func imageTag(version string) (string, error) {
	if !versionPattern.MatchString(version) {
		return "", usageError{"version must contain 1-128 tag-safe characters (letters, digits, _, ., +, -), starting with a letter, digit or _"}
	}
	// OCI tags exclude SemVer's build-metadata separator; the binary keeps it.
	return strings.ReplaceAll(version, "+", "-"), nil
}

func (runner runner) release(ctx context.Context, override string) (version, tag string, err error) {
	version = override
	if version == "" {
		version = runner.version(ctx)
	}
	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	tag, err = imageTag(version)
	return version, tag, err
}

func imageBuildArgs(engine, version, tag string) []string {
	args := []string{"build", "--platform", "linux/amd64"}
	if engine == "podman" {
		args = append(args, "--format", "docker")
	}
	args = append(args, "-f", filepath.Join("deploy", "Dockerfile"),
		"--build-arg", "VERSION="+version,
		"-t", "localhost/pandatable:"+tag, "-t", "localhost/pandatable:latest")
	for _, name := range []string{"GOPROXY", "NPM_REGISTRY", "APT_MIRROR"} {
		if _, ok := os.LookupEnv(name); ok {
			args = append(args, "--build-arg", name)
		}
	}
	return append(args, ".")
}

func imageSaveArgs(engine, image, target string) []string {
	args := []string{"save"}
	if engine == "podman" {
		args = append(args, "--format", "docker-archive")
	}
	return append(args, "-o", target, image)
}

func (runner runner) image(ctx context.Context, opts options) error {
	selected, err := detectEngine(os.Getenv("PT_ENGINE"), exec.LookPath)
	if err != nil {
		return err
	}
	version, tag, err := runner.release(ctx, opts.releaseVersion)
	if err != nil {
		return err
	}
	if opts.command == "image build" {
		return runner.run(ctx, selected.name, imageBuildArgs(selected.name, version, tag)...)
	}
	if err := runner.saveImage(ctx, selected.name, "localhost/pandatable:"+tag, "pandatable-"+tag+".tar.gz"); err != nil {
		return err
	}
	if opts.withPostgres {
		if err := runner.run(ctx, selected.name, "pull", "--platform", "linux/amd64", postgresImage); err != nil {
			return err
		}
		return runner.saveImage(ctx, selected.name, postgresImage, "postgres-18.tar.gz")
	}
	return nil
}

func (runner runner) saveImage(ctx context.Context, engine, image, filename string) error {
	target := filepath.Join(runner.root, "dist", filename)
	if err := exportImage(ctx, target, func(tar string) error {
		return runner.run(ctx, engine, imageSaveArgs(engine, image, tar)...)
	}); err != nil {
		return fmt.Errorf("export %s: %w", image, err)
	}
	fmt.Println("Saved " + target)
	return nil
}

func exportImage(ctx context.Context, target string, save func(string) error) (err error) {
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return err
	}
	dir, err := os.MkdirTemp(filepath.Dir(target), ".ptdev-image-")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(dir)) }()
	tar := filepath.Join(dir, "image.tar")
	if err := save(tar); err != nil {
		return err
	}
	compressed := filepath.Join(dir, "image.tar.gz")
	if err := gzipFile(ctx, tar, compressed); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return os.Rename(compressed, target)
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader contextReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(buffer)
}

func gzipFile(ctx context.Context, source, target string) (err error) {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, input.Close()) }()
	info, err := input.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return errors.New("engine produced an empty or non-regular archive")
	}
	output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, output.Close()) }()
	compressor := gzip.NewWriter(output)
	_, copyErr := io.Copy(compressor, contextReader{ctx: ctx, reader: input})
	return errors.Join(copyErr, compressor.Close())
}
