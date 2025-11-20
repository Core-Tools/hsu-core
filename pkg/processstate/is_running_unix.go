//go:build !windows

package processstate

import (
	"fmt"
	"os"
	"syscall"
)

func IsProcessRunning(pid int) (bool, error) {
	if pid <= 0 {
		return false, fmt.Errorf("invalid PID format")
	}

	// On Unix systems, FindProcess always succeeds and returns a Process
	// for the given pid, regardless of whether the process exists. To test whether
	// the process actually exists, see whether p.Signal(syscall.Signal(0)) reports
	// an error.
	process, err := os.FindProcess(pid)
	if err != nil {
		return false, err
	}

	err = process.Signal(syscall.Signal(0))
	if err == nil {
		return true, nil
	}
	if err.Error() == "os: process already finished" {
		return false, nil
	}
	errno, ok := err.(syscall.Errno)
	if !ok {
		return false, err
	}
	switch errno {
	case syscall.ESRCH:
		return false, nil
	case syscall.EPERM:
		return true, nil
	}
	return false, err
}
