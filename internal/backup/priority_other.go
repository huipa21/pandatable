//go:build !linux

package backup

func runLowPriority(work func() error) error { return work() }
