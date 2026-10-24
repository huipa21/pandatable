package main

import (
	"fmt"
)

type usageError struct{ message string }

func (err usageError) Error() string { return err.message }

type engine struct {
	name   string
	reason string
}

func detectEngine(override string, lookPath func(string) (string, error)) (engine, error) {
	if override != "" {
		if override != "podman" && override != "docker" {
			return engine{}, usageError{"PT_ENGINE must be podman or docker"}
		}
		if _, err := lookPath(override); err != nil {
			return engine{}, fmt.Errorf("PT_ENGINE=%s: executable not found on PATH: %w", override, err)
		}
		return engine{override, "PT_ENGINE override"}, nil
	}
	for _, name := range []string{"podman", "docker"} {
		if _, err := lookPath(name); err == nil {
			return engine{name, "auto-detected on PATH (podman preferred)"}, nil
		}
	}
	return engine{}, fmt.Errorf("no container engine found: install podman or docker, or set PT_ENGINE")
}
