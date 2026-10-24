package main

import (
	"context"
	"strings"
)

func formatVersion(description string, gitErr error) string {
	description = strings.TrimSpace(description)
	if gitErr != nil || description == "" {
		return "0.0.0-dev"
	}
	sha := strings.TrimSuffix(description, "-dirty")
	if sha != "" && strings.Trim(sha, "0123456789abcdefABCDEF") == "" {
		version := "0.0.0-dev+" + sha
		if sha != description {
			version += ".dirty"
		}
		return version
	}
	return strings.TrimPrefix(description, "v")
}

func (runner runner) version(ctx context.Context) string {
	output, err := runner.execute(ctx, "", nil, true, "git", "describe", "--tags", "--dirty", "--always")
	return formatVersion(output, err)
}
