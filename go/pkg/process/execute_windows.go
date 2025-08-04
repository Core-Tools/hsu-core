//go:build windows

package process

import (
	"os/exec"
	"syscall"
)

// setupProcessAttributes configures Windows-specific process attributes for proper signal handling.
// Sets CREATE_NEW_CONSOLE flag to enable Ctrl+C signal delivery to child processes,
// which is essential for graceful termination in Windows console applications.
func setupProcessAttributes(cmd *exec.Cmd) {
	// This isolates children for termination but preserves master's signal handling
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
		// CREATE_NEW_PROCESS_GROUP: Enables our termination logic (sendCtrlBreak)
		// No DETACHED_PROCESS: Preserves master's console signal handling
	}

	// Note: This approach:
	// 1. Child processes in separate group (enables graceful termination)
	// 2. Master retains normal console signal handling in cmd.exe
	// 3. Our sendCtrlBreakToProcess() can terminate children gracefully
	// 4. Simpler signal routing - less interference during restart cycles
}
