package main

import (
	"errors"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

func removeDevDir(dir string) error {
	deadline := time.Now().Add(2 * time.Second)
	for {
		err := os.RemoveAll(dir)
		if err == nil {
			return nil
		}
		locked := errors.Is(err, windows.ERROR_SHARING_VIOLATION) ||
			errors.Is(err, windows.ERROR_LOCK_VIOLATION) || os.IsPermission(err)
		if !locked || !time.Now().Before(deadline) {
			return fmt.Errorf("remove dev temporary directory %s: %w", dir, err)
		}
		// Windows can retain an executable's image mapping briefly after Wait.
		time.Sleep(50 * time.Millisecond)
	}
}
