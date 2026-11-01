//go:build linux

package backup

import (
	"fmt"
	"runtime"

	"golang.org/x/sys/unix"
)

func runLowPriority(work func() error) error {
	runtime.LockOSThread()
	if err := unix.Setpriority(unix.PRIO_PROCESS, unix.Gettid(), 10); err != nil {
		runtime.UnlockOSThread()
		return fmt.Errorf("lower backup priority: %w", err)
	}
	// A locked thread is retired when this job's goroutine exits; its niceness
	// must not leak into unrelated server work.
	return work()
}
