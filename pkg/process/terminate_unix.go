//go:build !windows

package process

import (
	"syscall"
	"time"
)

// sendTerminationSignal sends SIGTERM to the process group on Unix systems
func SendTerminationSignal(pid int, idDead bool, timeout time.Duration) error {
	// Send SIGTERM to the process group (negative PID)
	// This ensures we terminate the entire process tree
	return syscall.Kill(-pid, syscall.SIGTERM)
}
