package main

import (
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestImageTag(t *testing.T) {
	for _, test := range []struct{ version, tag string }{
		{"1.2.3", "1.2.3"},
		{"0.0.0-dev+d710743.dirty", "0.0.0-dev-d710743.dirty"},
		{"1.0.0-2-gabc-dirty", "1.0.0-2-gabc-dirty"},
	} {
		got, err := imageTag(test.version)
		if err != nil || got != test.tag {
			t.Fatalf("%q: %q, %v", test.version, got, err)
		}
	}
	for _, version := range []string{"", "-bad", ".bad", "v/1", "1 2", "x\nx", `x" -X y=z`, strings.Repeat("x", 129)} {
		if _, err := imageTag(version); err == nil {
			t.Errorf("invalid version accepted: %q", version)
		}
	}
}

func TestImageCommands(t *testing.T) {
	for _, name := range []string{"GOPROXY", "NPM_REGISTRY", "APT_MIRROR"} {
		t.Setenv(name, "private-mirror-not-for-command-output")
	}
	for _, engine := range []string{"podman", "docker"} {
		args := imageBuildArgs(engine, "1.2.3+abc", "1.2.3-abc")
		joined := strings.Join(args, "|")
		for _, want := range []string{"linux/amd64", "VERSION=1.2.3+abc", "localhost/pandatable:1.2.3-abc", "localhost/pandatable:latest", filepath.Join("deploy", "Dockerfile")} {
			if !strings.Contains(joined, want) {
				t.Errorf("%s missing %s: %v", engine, want, args)
			}
		}
		if strings.Contains(joined, "private-mirror") || strings.Contains(joined, "--format|docker") != (engine == "podman") || args[len(args)-1] != "." {
			t.Fatalf("incorrect build command: %v", args)
		}
		target := filepath.Join(t.TempDir(), "path with spaces", "image.tar")
		got := imageSaveArgs(engine, "localhost/pandatable:1.2.3", target)
		want := []string{"save"}
		if engine == "podman" {
			want = append(want, "--format", "docker-archive")
		}
		want = append(want, "-o", target, "localhost/pandatable:1.2.3")
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("save args = %v, want %v", got, want)
		}
	}
}

func TestExportImage(t *testing.T) {
	for _, mode := range []string{"success", "save failure", "empty", "missing", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			target := filepath.Join(root, "path with spaces", "image.tar.gz")
			if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(target, []byte("previous archive"), 0o600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			payload := strings.Repeat("tar payload", 1024)
			err := exportImage(ctx, target, func(tar string) error {
				switch mode {
				case "save failure":
					return errors.New("engine failed")
				case "missing":
					return nil
				case "empty":
					return os.WriteFile(tar, nil, 0o600)
				case "cancelled":
					cancel()
				}
				return os.WriteFile(tar, []byte(payload), 0o600)
			})
			if mode == "success" {
				if err != nil {
					t.Fatal(err)
				}
				file, err := os.Open(target)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = file.Close() }()
				reader, err := gzip.NewReader(file)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = reader.Close() }()
				content, err := io.ReadAll(reader)
				if err != nil || string(content) != payload {
					t.Fatalf("gzip completeness: %d bytes, %v", len(content), err)
				}
			} else {
				if err == nil {
					t.Fatal("export failure returned success")
				}
				content, err := os.ReadFile(target)
				if err != nil || string(content) != "previous archive" {
					t.Fatalf("failed save changed previous archive: %q, %v", content, err)
				}
			}
			entries, err := os.ReadDir(filepath.Dir(target))
			if err != nil || len(entries) != 1 || entries[0].Name() != "image.tar.gz" {
				t.Fatalf("staging files leaked: %v, %v", entries, err)
			}
		})
	}
}
