//go:build !windows

package process

import (
	"os/exec"
	"syscall"
)

func resetConsoleSignals(setHandler bool) error {
	return nil
}

// setupProcessAttributes configures Unix-specific process attributes
func setupProcessAttributes(cmd *exec.Cmd) {
	// On Unix, create a new process group that we can signal as a whole
	// This is essential so that when we send SIGTERM to -pid, it affects
	// the entire process tree (parent + all children)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	// Note: Process termination is now handled by ProcessControl.terminateProcess()
	// which works uniformly for both spawned and attached processes
}
