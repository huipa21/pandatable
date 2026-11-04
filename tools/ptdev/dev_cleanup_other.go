//go:build !windows

package main

import (
	"fmt"
	"os"
)

func removeDevDir(dir string) error {
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove dev temporary directory %s: %w", dir, err)
	}
	return nil
}
