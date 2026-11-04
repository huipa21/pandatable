package main

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// bindChildrenToProcess puts ptdev in a kill-on-close job object. Children
// inherit the job, so Windows terminates them when ptdev exits for any reason,
// including a killed terminal or go run wrapper. The handle is intentionally
// kept open for ptdev's lifetime.
func bindChildrenToProcess() error {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return fmt.Errorf("create dev job object: %w", err)
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		_ = windows.CloseHandle(job)
		return fmt.Errorf("configure dev job object: %w", err)
	}
	if err := windows.AssignProcessToJobObject(job, windows.CurrentProcess()); err != nil {
		_ = windows.CloseHandle(job)
		return fmt.Errorf("assign ptdev to dev job object: %w", err)
	}
	return nil
}
