package processcontrol

import (
	"time"
)

// ProcessState represents the current lifecycle state of the process control
type ProcessState string

const (
	ProcessStateIdle        ProcessState = "idle"         // No process, ready to start
	ProcessStateStarting    ProcessState = "starting"     // Process startup in progress
	ProcessStateRunning     ProcessState = "running"      // Process running normally
	ProcessStateStopping    ProcessState = "stopping"     // Graceful shutdown initiated
	ProcessStateTerminating ProcessState = "terminating"  // Force termination in progress
	ProcessStateFailedStart ProcessState = "failed_start" // Failed to start process
)

// ProcessError categorizes process control errors
type ProcessError struct {
	Category    string    // Error category constant
	Details     string    // Human-readable description
	Underlying  error     // Original error
	Timestamp   time.Time // When the error occurred
	Recoverable bool      // Whether this error is potentially recoverable
}

// Error category constants
const (
	ErrorCategoryExecutableNotFound = "executable_not_found"
	ErrorCategoryPermissionDenied   = "permission_denied"
	ErrorCategoryResourceLimit      = "resource_limit"
	ErrorCategoryNetworkIssue       = "network_issue"
	ErrorCategoryTimeout            = "timeout"
	ErrorCategoryProcessCrash       = "process_crash"
	ErrorCategoryUnknown            = "unknown"
)

// ProcessDiagnostics provides detailed process status information
type ProcessDiagnostics struct {
	State            ProcessState
	LastError        *ProcessError
	ProcessID        int
	StartTime        *time.Time
	ExecutablePath   string
	ExecutableExists bool
	FailureCount     int
	LastAttemptTime  time.Time
}
