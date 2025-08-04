# Enhanced Error Reporting Implementation

This document describes the implementation of enhanced error reporting for the HSU Master system, addressing specific architectural concerns around failure diagnostics and package dependencies.

## Overview

The enhanced error reporting system provides detailed diagnostics for worker failures, with special handling for the "executable not found" scenario and proper support for different worker types (managed vs unmanaged).

## Key Improvements Implemented

### 1. **Unmanaged Units Support** ✅

**Problem**: Unmanaged units don't have executable paths configured, making it unclear how they would appear in diagnostics.

**Solution**: Enhanced `GetDiagnostics()` to properly distinguish between worker types:

```go
// ProcessControl.GetDiagnostics() now handles:
if pc.config.ExecuteCmd != nil {
    executablePath = "managed_process"
    // Uses actual start error to determine if executable exists
} else if pc.config.AttachCmd != nil {
    executablePath = "unmanaged_process" 
    // Unmanaged processes attach to existing processes
    executableExists = true
} else {
    executablePath = "no_execution_method"
    executableExists = false
}
```

**Client Usage**:
```go
diagnostics, _ := master.GetWorkerProcessDiagnostics("worker-id")
if diagnostics.ExecutablePath == "unmanaged_process" {
    // Client knows this is an unmanaged worker
    // No executable installation needed
}
```

### 2. **Optimized Error Handling** ✅

**Problem**: Duplicate file existence checks - `os.Stat()` in `GetDiagnostics()` vs existing `ensureExecutable()` in `execute.go`.

**Solution**: Leverage actual start process errors instead of duplicating checks:

```go
// In processControl.startInternal():
process, stdout, healthCheckConfig, err := pc.startProcess(ctx)
if err != nil {
    // Categorize and store the actual error
    pc.lastError = categorizeProcessError(err)
    pc.failureCount++
    pc.state = processcontrol.ProcessStateFailedStart
}

// In GetDiagnostics():
if pc.lastError != nil && pc.lastError.Category == processcontrol.ErrorCategoryExecutableNotFound {
    executableExists = false
}
```

**Benefits**:
- **No duplicate file system calls**
- **Uses actual execution errors** from `ensureExecutable()`
- **More accurate error categorization**
- **Captures real failure scenarios**

### 3. **Package Dependency Cleanup** ✅

**Problem**: `workerstatemachine` package had unnecessary dependency on `workers/processcontrol`, violating separation of concerns.

**Solution**: Moved orchestration logic to Master and simplified workerstatemachine:

**Before**:
```
pkg/master/workerstatemachine/
├── worker_state_machine.go (depends on processcontrol)
└── (contains ProcessState, ProcessDiagnostics logic)

Master.GetWorkerStateInfo() → WorkerStateMachine.GetStateInfoWithProcessDiagnostics()
```

**After**:
```
pkg/master/workerstatemachine/
├── worker_state_machine.go (no processcontrol dependency)
└── (pure worker state management)

Master.GetWorkerStateInfo() → combines WorkerStateInfo + ProcessDiagnostics
```

**Enhanced Master API**:
```go
type WorkerStateWithDiagnostics struct {
    workerstatemachine.WorkerStateInfo
    ProcessDiagnostics processcontrol.ProcessDiagnostics // Includes State field
}

func (m *Master) GetWorkerStateWithDiagnostics(id string) (WorkerStateWithDiagnostics, error)
func (m *Master) GetAllWorkerStatesWithDiagnostics() map[string]WorkerStateWithDiagnostics
func (m *Master) GetWorkerProcessDiagnostics(id string) (processcontrol.ProcessDiagnostics, error)
```

**Note**: The previous `GetWorkerFailureDiagnostics()` method was removed as redundant since `WorkerStateWithDiagnostics` now provides comprehensive failure information combining both worker state and process diagnostics.

## Detailed Error Categorization

The system now provides comprehensive error categorization:

### Error Categories
- `executable_not_found` - **Perfect for your scenario!**
- `permission_denied`
- `resource_limit`
- `timeout`
- `network_issue`
- `process_crash`
- `unknown`

### Error Source Handling
The `categorizeProcessError` function handles error messages from multiple sources:
- **OS-level errors**: `"no such file or directory"` (Unix), `"cannot find the file"` (Windows)
- **Go standard library**: `"executable file not found in $PATH"`
- **Wrapped internal errors**: When `ensureExecutable` wraps OS errors in `DomainError`, the original OS error is preserved in `DomainError.Cause` and included in the final error message, so OS-level patterns are still detectable.

This design leverages the error chain to maintain consistent categorization regardless of error wrapping.

### Enhanced ProcessError Structure
```go
type ProcessError struct {
    Category    string    // Error category constant
    Details     string    // Human-readable description
    Underlying  error     // Original error
    Timestamp   time.Time // When the error occurred
    Recoverable bool      // Whether this error is potentially recoverable
}
```

### Enhanced ProcessDiagnostics
```go
type ProcessDiagnostics struct {
    State            ProcessState
    LastError        *ProcessError
    ProcessID        int
    StartTime        *time.Time
    ExecutablePath   string    // "managed_process", "unmanaged_process", etc.
    ExecutableExists bool      // Based on actual execution results
    FailureCount     int
    LastAttemptTime  time.Time
}
```

## Usage Examples

### 1. Detecting "Executable Not Found" Scenario

```go
// Enhanced state info with process diagnostics
stateInfo, err := master.GetWorkerStateInfo("my-worker")
if err != nil {
    log.Fatal(err)
}

// Check worker-level error category
if stateInfo.ErrorCategory == "executable_not_found" {
    fmt.Printf("Worker %s failed: executable not found\n", stateInfo.WorkerID)
    fmt.Printf("Suggested action: %s\n", diagnostics.SuggestedAction)
}

// Check process-level diagnostics
if !stateInfo.ProcessDiagnostics.ExecutableExists {
    fmt.Printf("Process executable not available\n")
    if stateInfo.ProcessDiagnostics.LastError != nil {
        fmt.Printf("Last error: %s\n", stateInfo.ProcessDiagnostics.LastError.Details)
    }
}
```

### 2. Distinguishing Worker Types

```go
stateInfo, _ := master.GetWorkerStateInfo("worker-id")

switch stateInfo.ProcessDiagnostics.ExecutablePath {
case "managed_process":
    // This is a managed worker - check if executable exists
    if !stateInfo.ProcessDiagnostics.ExecutableExists {
        fmt.Println("Need to install executable")
    }
case "unmanaged_process":
    // This is an unmanaged worker - attaches to existing process
    fmt.Println("Worker attaches to existing process")
case "no_execution_method":
    // Misconfigured worker
    fmt.Println("Worker has no execution method configured")
}
```

### 3. Failure Analysis and Recovery

```go
stateInfo, _ := master.GetWorkerStateInfo("worker-id")

// Check worker-level error category
switch stateInfo.ErrorCategory {
case "executable_not_found":
    fmt.Println("Need to install executable")
    installWorkerExecutable(stateInfo.WorkerID)
case "permission_denied":
    fmt.Println("Need to fix permissions")
    fixPermissions(stateInfo.ProcessDiagnostics.ExecutablePath)
case "resource_limit":
    fmt.Println("Need to increase resources")
    increaseResources(stateInfo.WorkerID)
}

// Check process-level error details
if stateInfo.ProcessDiagnostics.LastError != nil {
    fmt.Printf("Process error details: %s\n", stateInfo.ProcessDiagnostics.LastError.Details)
    if stateInfo.ProcessDiagnostics.LastError.Recoverable {
        fmt.Println("Error is automatically recoverable")
    }
}
```

## Architectural Benefits

### 1. **Clean Separation of Concerns**
- **WorkerStateMachine**: Pure worker lifecycle management
- **ProcessControl**: OS process interaction and error tracking
- **Master**: Orchestration and API composition

### 2. **Enhanced Error Intelligence**
- **Real execution errors** instead of speculative checks
- **Categorized failures** with recovery suggestions
- **Detailed diagnostics** for both worker and process levels

### 3. **Type-Aware Diagnostics**
- **Managed workers**: Track executable availability
- **Unmanaged workers**: Track attachment success
- **Misconfigured workers**: Clear error indication

### 4. **Future-Ready Architecture**
- **Package management** can easily integrate with error categories
- **Recovery automation** can use suggested actions
- **Monitoring systems** can leverage detailed diagnostics

## Integration with Future Package Management

The enhanced error reporting provides perfect foundation for dynamic package management:

```go
// Future package management integration
stateInfo, _ := master.GetWorkerStateInfo("worker-id")
if stateInfo.ErrorCategory == "executable_not_found" {
    // Automatically trigger package installation
    err := packageManager.InstallWorkerPackage(ctx, InstallRequest{
        WorkerType: stateInfo.WorkerID,
        Reason:     "executable_not_found",
    })
}
```

## Conclusion

This implementation provides a robust foundation for error diagnostics while maintaining clean architectural boundaries. The system now properly handles:

✅ **Unmanaged unit detection** through process type identification  
✅ **Optimized error handling** using actual execution errors  
✅ **Clean package dependencies** with Master orchestration  
✅ **Comprehensive failure analysis** with recovery suggestions  
✅ **Type-aware diagnostics** for different worker configurations  

The enhanced error reporting system is ready for integration with dynamic package management and provides excellent visibility into worker failure scenarios.