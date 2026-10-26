//go:build !linux

package admin

import (
	"context"
	"time"
)

func containerMemory(_ context.Context, at time.Time) Metric[ContainerMemory] {
	return unavailable[ContainerMemory](at, "unsupported")
}
