//go:build linux

package admin

import (
	"context"
	"os"
	"time"
)

func containerMemory(ctx context.Context, at time.Time) Metric[ContainerMemory] {
	return readContainerMemory(ctx, at, os.DirFS("/"))
}
