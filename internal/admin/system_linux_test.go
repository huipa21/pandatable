//go:build linux

package admin

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestSystemDirectoryPermissionAndSpecialFile(t *testing.T) {
	dir := storageFixture(t)
	blocked := filepath.Join(dir, "files", "blocked")
	if err := os.Mkdir(blocked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(blocked, 0o700); err != nil {
			t.Error(err)
		}
	})
	if os.Geteuid() != 0 {
		size, err := directorySize(context.Background(), dir, "files", scanBounds{entries: 100, depth: 4})
		if size != 0 || !errors.Is(err, os.ErrPermission) {
			t.Fatal("permission failure must invalidate entire total", size, err)
		}
	}
	if err := os.Chmod(blocked, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mkfifo(filepath.Join(dir, "files", "pipe"), 0o600); err != nil {
		t.Fatal(err)
	}
	if size, err := directorySize(context.Background(), dir, "files", scanBounds{entries: 100, depth: 4}); size != 0 || !errors.Is(err, errFileType) {
		t.Fatal("special file must fail without opening/blocking", size, err)
	}
}
