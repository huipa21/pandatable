package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestRemoveDevDirWindowsLockedFile(t *testing.T) {
	for _, release := range []bool{true, false} {
		name := "persistent"
		if release {
			name = "transient"
		}
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "ptdev")
			if err := os.Mkdir(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "pandatable.exe")
			if err := os.WriteFile(path, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			pathUTF16, err := windows.UTF16PtrFromString(path)
			if err != nil {
				t.Fatal(err)
			}
			handle, err := windows.CreateFile(pathUTF16, windows.GENERIC_READ, windows.FILE_SHARE_READ,
				nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
			if err != nil {
				t.Fatal(err)
			}
			if release {
				closed := make(chan error, 1)
				go func() {
					time.Sleep(150 * time.Millisecond)
					closed <- windows.CloseHandle(handle)
				}()
				err := removeDevDir(dir)
				if closeErr := <-closed; closeErr != nil {
					t.Fatal(closeErr)
				}
				if err != nil {
					t.Fatalf("transient lock was not retried: %v", err)
				}
				if _, err := os.Stat(dir); !os.IsNotExist(err) {
					t.Fatalf("temporary directory remains: %v", err)
				}
			} else {
				start := time.Now()
				err := removeDevDir(dir)
				if closeErr := windows.CloseHandle(handle); closeErr != nil {
					t.Fatal(closeErr)
				}
				if err == nil {
					t.Fatal("persistent lock was silently ignored")
				}
				if elapsed := time.Since(start); elapsed < 2*time.Second || elapsed > 4*time.Second {
					t.Fatalf("cleanup retry was not bounded: %s", elapsed)
				}
			}
		})
	}
}
