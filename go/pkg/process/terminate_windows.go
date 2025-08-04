//go:build windows

package process

import (
	"fmt"
	"sync"
	"syscall"
	"time"

	"github.com/core-tools/hsu-core/pkg/processstate"
)

// Windows console operation lock to prevent race conditions
var consoleOperationLock sync.Mutex

// Enhanced sendTerminationSignal using the improved console signal fix
func SendTerminationSignal(pid int, idDead bool, timeout time.Duration) error {
	if pid <= 0 {
		return fmt.Errorf("invalid PID: %d", pid)
	}

	consoleOperationLock.Lock()
	defer consoleOperationLock.Unlock()

	if idDead {
		isRunning, _ := processstate.IsProcessRunning(pid)
		idDead = !isRunning
	}

	dll, err := syscall.LoadDLL("kernel32.dll")
	if err != nil {
		return fmt.Errorf("failed to load kernel32.dll: %v", err)
	}
	defer dll.Release()

	if idDead {
		// Apply the AttachConsole dead PID hack to fix Windows signal handling
		// This restores Ctrl+C functionality for the master process
		return consoleSignalFix(dll, pid)
	} else {
		// Send actual Ctrl+Break signal to alive process with safety checks
		return sendCtrlBreakToProcessSafe(dll, pid, timeout)
	}
}

// consoleSignalFix implements the AttachConsole dead PID hack to fix Windows signal handling
func consoleSignalFix(dll *syscall.DLL, deadPID int) error {
	// Try to attach to the dead process - this triggers console state reset
	err := attachConsole(dll, deadPID)
	if err != nil {
		// Expected to fail for dead process - that's the magic!
		return nil
	}
	// Unexpected success - should not happen with dead PID
	return fmt.Errorf("warning: AttachConsole unexpectedly succeeded for PID %d", deadPID)
}

// sendCtrlBreakToProcessSafe sends Ctrl+Break with liveness check and timeout protection
func sendCtrlBreakToProcessSafe(dll *syscall.DLL, pid int, timeout time.Duration) error {
	// Use timeout to prevent hanging
	done := make(chan error, 1)
	go func() {
		done <- generateConsoleCtrlEvent(dll, pid)
	}()

	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("failed to send Ctrl+Break to PID %d: %v", pid, err)
		}
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("timeout sending Ctrl+Break to PID %d after %v", pid, timeout)
	}
}

// Helper functions for Windows API calls
func attachConsole(dll *syscall.DLL, pid int) error {
	attachConsole, err := dll.FindProc("AttachConsole")
	if err != nil {
		return err
	}

	result, _, err := attachConsole.Call(uintptr(pid))
	if result == 0 {
		// This is expected to fail for dead PIDs - that's the magic!
		return err
	}
	return nil
}

func generateConsoleCtrlEvent(dll *syscall.DLL, pid int) error {
	generateConsoleCtrlEvent, err := dll.FindProc("GenerateConsoleCtrlEvent")
	if err != nil {
		return err
	}

	result, _, err := generateConsoleCtrlEvent.Call(
		uintptr(syscall.CTRL_BREAK_EVENT),
		uintptr(pid),
	)
	if result == 0 {
		return err
	}
	return nil
}

// Advanced console reset strategy (alternative approach)
func resetConsoleSession() error {
	dll, err := syscall.LoadDLL("kernel32.dll")
	if err != nil {
		return err
	}
	defer dll.Release()

	// Free current console
	if err := freeConsole(dll); err != nil {
		// FreeConsole failed (might be expected)
	}

	// Allocate new console
	if err := allocConsole(dll); err != nil {
		return fmt.Errorf("failed to allocate new console: %v", err)
	}

	return nil
}

// Console management helpers for advanced scenarios
func freeConsole(dll *syscall.DLL) error {
	freeConsole, err := dll.FindProc("FreeConsole")
	if err != nil {
		return err
	}

	result, _, err := freeConsole.Call()
	if result == 0 {
		return err
	}
	return nil
}

func allocConsole(dll *syscall.DLL) error {
	allocConsole, err := dll.FindProc("AllocConsole")
	if err != nil {
		return err
	}

	result, _, err := allocConsole.Call()
	if result == 0 {
		return err
	}
	return nil
}
