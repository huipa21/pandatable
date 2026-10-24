package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBinaryName(t *testing.T) {
	for _, test := range []struct {
		goos, want string
		linux      bool
	}{
		{"windows", "pandatable.exe", false},
		{"linux", "pandatable", false},
		{"darwin", "pandatable", false},
		{"windows", "pandatable-linux-amd64", true},
		{"linux", "pandatable-linux-amd64", true},
	} {
		if got := binaryName(test.goos, test.linux); got != test.want {
			t.Errorf("binaryName(%q, %v) = %q, want %q", test.goos, test.linux, got, test.want)
		}
	}
}

func TestCopyWeb(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "web", "dist", "assets")
	if err := os.MkdirAll(source, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "app.js"), []byte("asset"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "web", "dist", "index.html"), []byte("<html>app</html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "internal", "web", "dist")
	if err := os.MkdirAll(target, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "stale.js"), []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, ".gitkeep"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := (runner{root: root}).copyWeb(); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(target, "assets", "app.js"))
	if err != nil || string(content) != "asset" {
		t.Fatalf("copied content=%q, err=%v", content, err)
	}
	if exists(filepath.Join(target, "stale.js")) {
		t.Fatal("stale web asset was not removed")
	}
	if !exists(filepath.Join(target, ".gitkeep")) {
		t.Fatal("tracked embed placeholder was removed")
	}
}

func TestCopyWebRejectsMissingOrEmptyIndex(t *testing.T) {
	for _, content := range []string{"", " \n"} {
		t.Run(content, func(t *testing.T) {
			root := t.TempDir()
			source := filepath.Join(root, "web", "dist")
			target := filepath.Join(root, "internal", "web", "dist")
			for _, dir := range []string{source, target} {
				if err := os.MkdirAll(dir, 0o750); err != nil {
					t.Fatal(err)
				}
			}
			if content != "" {
				if err := os.WriteFile(filepath.Join(source, "index.html"), []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			stale := filepath.Join(target, "index.html")
			if err := os.WriteFile(stale, []byte("existing"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := (runner{root: root}).copyWeb(); err == nil {
				t.Fatal("invalid source accepted")
			}
			if content, err := os.ReadFile(stale); err != nil || string(content) != "existing" {
				t.Fatalf("existing assets changed: %q, %v", content, err)
			}
		})
	}
}
