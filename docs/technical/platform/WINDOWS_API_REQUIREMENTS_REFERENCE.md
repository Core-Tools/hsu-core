# Windows API Requirements Reference

**Project**: HSU Master Process Manager  
**Document Version**: 1.0  
**Date**: January 2025  
**Author**: Technical Analysis  
**Scope**: Complete Windows API requirements and compatibility analysis

---

## 🎯 **Executive Summary**

This document provides a comprehensive analysis of Windows APIs required for implementing process management, resource limits, and console signal handling in the HSU Master process manager. It covers API availability across Windows versions, implementation strategies, and practical code examples for production deployment.

**Key Finding**: All required APIs are available in Windows 7+ with full functionality, providing robust process management and resource monitoring capabilities on Windows platforms.

---

## 📋 **Complete Windows API Requirements Analysis**

### **🔧 Category 1: Process Management APIs**

**Availability**: Windows XP+ (Universal Support)

```c
// Core process management APIs from kernel32.dll
#include <windows.h>
#include <processthreadsapi.h>

// Process lifecycle management
HANDLE OpenProcess(DWORD dwDesiredAccess, BOOL bInheritHandle, DWORD dwProcessId);
BOOL TerminateProcess(HANDLE hProcess, UINT uExitCode);
BOOL GetExitCodeProcess(HANDLE hProcess, LPDWORD lpExitCode);
BOOL CreateProcess(/* ... parameters ... */);

// Process access rights
#define PROCESS_TERMINATE                 0x0001
#define PROCESS_QUERY_INFORMATION         0x0400
#define PROCESS_QUERY_LIMITED_INFORMATION 0x1000
#define PROCESS_SET_QUOTA                 0x0100
#define PROCESS_VM_READ                   0x0010

// Process creation flags
#define CREATE_NEW_PROCESS_GROUP          0x00000200
#define CREATE_SUSPENDED                  0x00000004
#define DETACHED_PROCESS                  0x00000008
```

#### **Process Creation Configuration**

**Implementation**: `pkg/process/execute_windows.go`

```go
// Windows-specific process creation with signal handling isolation
func setupProcessAttributes(cmd *exec.Cmd) {
    cmd.SysProcAttr = &syscall.SysProcAttr{
        CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
    }
}
```

**Key Features**:
- ✅ **CREATE_NEW_PROCESS_GROUP**: Isolates child processes for independent termination
- ✅ **Console Signal Preservation**: Master process retains Ctrl+C handling capabilities
- ✅ **Graceful Termination Support**: Enables sendCtrlBreak functionality for child processes
- ✅ **Signal Routing Optimization**: Prevents interference during process restart cycles

#### **Process State Monitoring**

**Implementation**: `pkg/processstate/is_running_windows.go`

```go
// Process liveness checking using minimal access rights
func IsProcessRunning(pid int) bool {
    handle, err := syscall.OpenProcess(
        PROCESS_QUERY_LIMITED_INFORMATION, // Minimal access rights
        false,                             // Don't inherit handle
        uint32(pid),
    )
    if err != nil {
        return false // Process doesn't exist or access denied
    }
    defer syscall.CloseHandle(handle)

    var exitCode uint32
    err = syscall.GetExitCodeProcess(handle, &exitCode)
    if err != nil {
        return false // Can't get exit code, assume dead
    }

    return exitCode == STILL_ACTIVE // 259 = still running
}
```

**Security Model**:
- ✅ **Minimal Permissions**: Uses `PROCESS_QUERY_LIMITED_INFORMATION` for security
- ✅ **Non-Destructive**: Read-only process state checking
- ✅ **Cross-User Compatible**: Works across different user contexts where permitted
- ✅ **Handle Management**: Proper handle cleanup to prevent resource leaks

### **⚡ Category 2: Console Signal Management APIs**

**Availability**: Windows 2000+ (Universal Support)

```c
// Console control APIs from kernel32.dll
#include <wincon.h>

// Console attachment and detachment
BOOL AttachConsole(DWORD dwProcessId);
BOOL FreeConsole(void);
BOOL AllocConsole(void);

// Console signal generation
BOOL GenerateConsoleCtrlEvent(DWORD dwCtrlEvent, DWORD dwProcessGroupId);

// Console control events
#define CTRL_C_EVENT        0
#define CTRL_BREAK_EVENT    1
#define CTRL_CLOSE_EVENT    2
#define CTRL_LOGOFF_EVENT   5
#define CTRL_SHUTDOWN_EVENT 6

// Special PID for current console
#define ATTACH_PARENT_PROCESS ((DWORD)-1)
```

#### **The AttachConsole Dead PID Hack**

**Implementation**: `pkg/process/terminate_windows.go`

This is a **critical Windows-specific workaround** for console signal handling issues:

```go
// consoleSignalFix implements the AttachConsole dead PID hack
func consoleSignalFix(dll *syscall.DLL, deadPID int) error {
    // The magic: AttachConsole to a dead process triggers console state reset
    err := attachConsole(dll, deadPID)
    if err != nil {
        // Expected to fail for dead process - that's the magic!
        return nil
    }
    // Unexpected success - should not happen with dead PID
    return fmt.Errorf("warning: AttachConsole unexpectedly succeeded for PID %d", deadPID)
}

func attachConsole(dll *syscall.DLL, pid int) error {
    attachConsole, err := dll.FindProc("AttachConsole")
    if err != nil {
        return err
    }

    result, _, err := attachConsole.Call(uintptr(pid))
    if result == 0 {
        return err // Expected failure for dead PIDs
    }
    return nil
}
```

**The Mystery Explained**:
1. **Problem**: After child process termination, master process loses Ctrl+C responsiveness
2. **Root Cause**: Windows console handle state becomes inconsistent
3. **Solution**: `AttachConsole(deadPID)` mysteriously resets console signal routing
4. **Magic**: The API call **must fail** (dead PID) to trigger the internal console state reset

#### **Safe Signal Transmission**

```go
// sendCtrlBreakToProcessSafe sends signals with timeout protection
func sendCtrlBreakToProcessSafe(dll *syscall.DLL, pid int, timeout time.Duration) error {
    done := make(chan error, 1)
    go func() {
        done <- generateConsoleCtrlEvent(dll, pid)
    }()

    select {
    case err := <-done:
        return err
    case <-time.After(timeout):
        return fmt.Errorf("timeout sending Ctrl+Break to PID %d after %v", pid, timeout)
    }
}

func generateConsoleCtrlEvent(dll *syscall.DLL, pid int) error {
    generateConsoleCtrlEvent, err := dll.FindProc("GenerateConsoleCtrlEvent")
    if err != nil {
        return err
    }

    result, _, err := generateConsoleCtrlEvent.Call(
        uintptr(syscall.CTRL_BREAK_EVENT), // More reliable than CTRL_C_EVENT
        uintptr(pid),
    )
    if result == 0 {
        return err
    }
    return nil
}
```

**Advanced Console Management**:
```go
// Emergency console reset for severe signal handling corruption
func resetConsoleSession() error {
    dll, err := syscall.LoadDLL("kernel32.dll")
    if err != nil {
        return err
    }
    defer dll.Release()

    // Free current console
    freeConsole(dll) // Ignore errors - might be expected

    // Allocate new console
    return allocConsole(dll)
}
```

### **🚀 Category 3: Windows Job Objects (Resource Management)**

**Availability**: Windows 2000+ (Universal Support)

```c
// Job Object APIs from kernel32.dll
#include <winnt.h>
#include <jobapi.h>

// Job Object management
HANDLE CreateJobObjectA(LPSECURITY_ATTRIBUTES lpJobAttributes, LPCSTR lpName);
BOOL AssignProcessToJobObject(HANDLE hJob, HANDLE hProcess);
BOOL SetInformationJobObject(HANDLE hJob, JOBOBJECTINFOCLASS JobObjectInformationClass, 
                            LPVOID lpJobObjectInformation, DWORD cbJobObjectInformationLength);
BOOL QueryInformationJobObject(HANDLE hJob, JOBOBJECTINFOCLASS JobObjectInformationClass, 
                              LPVOID lpJobObjectInformation, DWORD cbJobObjectInformationLength, 
                              LPDWORD lpReturnLength);

// Job Object limit flags
#define JOB_OBJECT_LIMIT_WORKINGSET                 0x00000001
#define JOB_OBJECT_LIMIT_PROCESS_TIME               0x00000002
#define JOB_OBJECT_LIMIT_JOB_TIME                   0x00000004
#define JOB_OBJECT_LIMIT_ACTIVE_PROCESS             0x00000008
#define JOB_OBJECT_LIMIT_AFFINITY                   0x00000010
#define JOB_OBJECT_LIMIT_PRIORITY_CLASS             0x00000020
#define JOB_OBJECT_LIMIT_PROCESS_MEMORY             0x00000100
#define JOB_OBJECT_LIMIT_JOB_MEMORY                 0x00000200
#define JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE          0x00002000

// Information classes
typedef enum _JOBOBJECTINFOCLASS {
    JobObjectBasicLimitInformation = 2,
    JobObjectExtendedLimitInformation = 9,
    JobObjectBasicUIRestrictions = 4,
    JobObjectSecurityLimitInformation = 5
} JOBOBJECTINFOCLASS;
```

#### **Memory Limits Enforcement**

**Implementation**: `pkg/resourcelimits/enforcer_windows.go`

```go
// Windows-specific memory limits using Job Objects
type jobObjectExtendedLimitInformation struct {
    BasicLimitInformation jobObjectBasicLimitInformation
    IoInfo                ioCounters
    ProcessMemoryLimit    uintptr
    JobMemoryLimit        uintptr
    PeakProcessMemoryUsed uintptr
    PeakJobMemoryUsed     uintptr
}

func applyWindowsMemoryLimitsImpl(pid int, limits *MemoryLimits, logger logging.Logger) error {
    enforcer := newWindowsResourceEnforcer(logger)

    // Create or get existing job object
    jobHandle, err := enforcer.getOrCreateJobObject(pid)
    if err != nil {
        return fmt.Errorf("failed to create job object: %v", err)
    }

    var extendedInfo jobObjectExtendedLimitInformation

    // Set working set limits (RSS equivalent)
    if limits.MaxRSS > 0 {
        extendedInfo.BasicLimitInformation.LimitFlags |= JOB_OBJECT_LIMIT_WORKINGSET
        extendedInfo.BasicLimitInformation.MaximumWorkingSetSize = uintptr(limits.MaxRSS)
    }

    // Set process memory limit (virtual memory)
    if limits.MaxVirtual > 0 {
        extendedInfo.BasicLimitInformation.LimitFlags |= JOB_OBJECT_LIMIT_PROCESS_MEMORY
        extendedInfo.ProcessMemoryLimit = uintptr(limits.MaxVirtual)
    }

    // Apply the limits
    ret, _, err := enforcer.setInformationJobObject.Call(
        uintptr(jobHandle),
        JobObjectExtendedLimitInformation,
        uintptr(unsafe.Pointer(&extendedInfo)),
        unsafe.Sizeof(extendedInfo),
    )

    if ret == 0 {
        return fmt.Errorf("SetInformationJobObject failed: %v", err)
    }

    return nil
}
```

#### **CPU Limits Enforcement**

```go
// CPU time limits using Job Objects
func applyWindowsCPULimitsImpl(pid int, limits *CPULimits, logger logging.Logger) error {
    var basicInfo jobObjectBasicLimitInformation

    // Set CPU time limit
    if limits.MaxTime > 0 {
        basicInfo.LimitFlags |= JOB_OBJECT_LIMIT_PROCESS_TIME
        // Convert to 100-nanosecond intervals (Windows FILETIME units)
        basicInfo.PerProcessUserTimeLimit = uint64(limits.MaxTime.Nanoseconds() / 100)
    }

    // Note: CPU percentage limits require additional implementation
    // Options: CPU Rate Control or monitoring-based throttling
    if limits.MaxPercent > 0 {
        logger.Warnf("CPU percentage limits require additional implementation beyond basic Job Objects")
    }

    return applyJobObjectLimits(jobHandle, &basicInfo)
}
```

#### **Process Assignment to Job Objects**

```go
// Secure process-to-job assignment with proper error handling
func (we *windowsResourceEnforcer) getOrCreateJobObject(pid int) (syscall.Handle, error) {
    // Create anonymous job object
    ret, _, err := we.createJobObject.Call(
        0, // NULL security attributes
        0, // Anonymous job object (no name)
    )
    if ret == 0 {
        return 0, fmt.Errorf("CreateJobObject failed: %v", err)
    }
    jobHandle := syscall.Handle(ret)

    // Open process with necessary rights
    procHandle, err := syscall.OpenProcess(
        PROCESS_SET_QUOTA|syscall.PROCESS_TERMINATE,
        false,
        uint32(pid),
    )
    if err != nil {
        syscall.CloseHandle(jobHandle)
        return 0, fmt.Errorf("failed to open process %d: %v", pid, err)
    }
    defer syscall.CloseHandle(procHandle)

    // Assign process to job object
    ret, _, err = we.assignProcessToJob.Call(
        uintptr(jobHandle),
        uintptr(procHandle),
    )
    if ret == 0 {
        syscall.CloseHandle(jobHandle)
        return 0, fmt.Errorf("AssignProcessToJobObject failed: %v", err)
    }

    return jobHandle, nil
}
```

### **📊 Category 4: Process Monitoring APIs**

**Availability**: Windows 2000+ (psapi.dll), Windows Vista+ (kernel32.dll)

```c
// Process information APIs
#include <psapi.h>

// Memory information
BOOL GetProcessMemoryInfo(HANDLE Process, PPROCESS_MEMORY_COUNTERS ppsmemCounters, DWORD cb);

typedef struct _PROCESS_MEMORY_COUNTERS {
    DWORD  cb;
    DWORD  PageFaultCount;
    SIZE_T PeakWorkingSetSize;
    SIZE_T WorkingSetSize;
    SIZE_T QuotaPeakPagedPoolUsage;
    SIZE_T QuotaPagedPoolUsage;
    SIZE_T QuotaPeakNonPagedPoolUsage;
    SIZE_T QuotaNonPagedPoolUsage;
    SIZE_T PagefileUsage;
    SIZE_T PeakPagefileUsage;
} PROCESS_MEMORY_COUNTERS;

// Process timing information
BOOL GetProcessTimes(HANDLE hProcess, LPFILETIME lpCreationTime, LPFILETIME lpExitTime, 
                    LPFILETIME lpKernelTime, LPFILETIME lpUserTime);

// Handle count information
BOOL GetProcessHandleCount(HANDLE hProcess, PDWORD pdwHandleCount);
```

#### **Comprehensive Resource Monitoring**

**Implementation**: `pkg/resourcelimits/platform_monitor_windows.go`

```go
// Windows-specific resource monitoring implementation
type windowsResourceMonitor struct {
    logger                logging.Logger
    kernel32              *syscall.LazyDLL
    psapi                 *syscall.LazyDLL
    getProcessMemoryInfo  *syscall.LazyProc
    getProcessTimes       *syscall.LazyProc
    getProcessHandleCount *syscall.LazyProc
}

func (w *windowsResourceMonitor) GetProcessUsage(pid int) (*ResourceUsage, error) {
    // Open process handle with monitoring rights
    handle, err := syscall.OpenProcess(
        syscall.PROCESS_QUERY_INFORMATION|PROCESS_VM_READ,
        false,
        uint32(pid),
    )
    if err != nil {
        return nil, fmt.Errorf("failed to open process %d: %v", pid, err)
    }
    defer syscall.CloseHandle(handle)

    usage := &ResourceUsage{
        Timestamp: time.Now(),
    }

    // Collect comprehensive resource information
    w.getMemoryUsage(handle, usage)   // Working set, page file usage
    w.getCPUUsage(handle, usage)      // Kernel/user time, CPU percentage
    w.getHandleCount(handle, usage)   // Handle count (file descriptors equivalent)
    w.getIOUsage(handle, usage)       // I/O counters (future enhancement)

    return usage, nil
}
```

#### **Memory Usage Monitoring**

```go
// Detailed memory information using GetProcessMemoryInfo
func (w *windowsResourceMonitor) getMemoryUsage(handle syscall.Handle, usage *ResourceUsage) error {
    var pmc processMemoryCounters
    pmc.Size = uint32(unsafe.Sizeof(pmc))

    ret, _, err := w.getProcessMemoryInfo.Call(
        uintptr(handle),
        uintptr(unsafe.Pointer(&pmc)),
        uintptr(pmc.Size),
    )

    if ret == 0 {
        return fmt.Errorf("GetProcessMemoryInfo failed: %v", err)
    }

    usage.MemoryRSS = int64(pmc.WorkingSetSize)     // Physical memory usage
    usage.MemoryVirtual = int64(pmc.PagefileUsage)  // Virtual memory usage

    return nil
}
```

#### **CPU Usage Monitoring**

```go
// CPU timing and percentage calculation
func (w *windowsResourceMonitor) getCPUUsage(handle syscall.Handle, usage *ResourceUsage) error {
    var creationTime, exitTime, kernelTime, userTime fileTime

    ret, _, err := w.getProcessTimes.Call(
        uintptr(handle),
        uintptr(unsafe.Pointer(&creationTime)),
        uintptr(unsafe.Pointer(&exitTime)),
        uintptr(unsafe.Pointer(&kernelTime)),
        uintptr(unsafe.Pointer(&userTime)),
    )

    if ret == 0 {
        return fmt.Errorf("GetProcessTimes failed: %v", err)
    }

    // Convert FILETIME to seconds (100-nanosecond intervals)
    kernelSeconds := fileTimeToSeconds(kernelTime)
    userSeconds := fileTimeToSeconds(userTime)
    usage.CPUTime = kernelSeconds + userSeconds

    return nil
}

// Enhanced CPU percentage calculation with temporal tracking
type windowsPerformanceMonitor struct {
    *windowsResourceMonitor
    lastCPUTime     float64
    lastMeasurement time.Time
}

func (w *windowsPerformanceMonitor) GetProcessUsage(pid int) (*ResourceUsage, error) {
    usage, err := w.windowsResourceMonitor.GetProcessUsage(pid)
    if err != nil {
        return nil, err
    }

    // Calculate CPU percentage based on time difference
    now := time.Now()
    if !w.lastMeasurement.IsZero() {
        timeDiff := now.Sub(w.lastMeasurement).Seconds()
        cpuTimeDiff := usage.CPUTime - w.lastCPUTime

        if timeDiff > 0 {
            usage.CPUPercent = (cpuTimeDiff / timeDiff) * 100
            // Cap at reasonable values
            if usage.CPUPercent > 100 {
                usage.CPUPercent = 100
            }
        }
    }

    w.lastCPUTime = usage.CPUTime
    w.lastMeasurement = now

    return usage, nil
}
```

#### **Handle Count Monitoring**

```go
// Windows handle count (equivalent to file descriptor count on Unix)
func (w *windowsResourceMonitor) getHandleCount(handle syscall.Handle, usage *ResourceUsage) error {
    var handleCount uint32

    ret, _, err := w.getProcessHandleCount.Call(
        uintptr(handle),
        uintptr(unsafe.Pointer(&handleCount)),
    )

    if ret == 0 {
        return fmt.Errorf("GetProcessHandleCount failed: %v", err)
    }

    usage.OpenFileDescriptors = int(handleCount)
    return nil
}
```

---

## 🔒 **Windows Security Model & Permissions**

### **User Account Control (UAC) Considerations**

**Impact on Process Management**:
- ✅ **Standard User Rights**: Basic process monitoring and management work without elevation
- ⚠️ **Cross-User Process Access**: Requires appropriate permissions or elevation
- ⚠️ **System Process Access**: Requires administrative privileges for some system processes
- ✅ **Job Object Creation**: Available to standard users for their own processes

### **Process Access Rights Matrix**

| Operation | Required Rights | Standard User | Admin Required |
|-----------|----------------|---------------|----------------|
| **OpenProcess (Query Info)** | PROCESS_QUERY_INFORMATION | ✅ Same user | ⚠️ Cross-user |
| **OpenProcess (Limited Query)** | PROCESS_QUERY_LIMITED_INFORMATION | ✅ Cross-user (Win Vista+) | ❌ Never |
| **Process Termination** | PROCESS_TERMINATE | ✅ Same user | ⚠️ Cross-user |
| **Job Object Assignment** | PROCESS_SET_QUOTA | ✅ Same user | ⚠️ Cross-user |
| **Console Signal Send** | Same process group | ✅ Always | ❌ Never |
| **Handle Count Query** | PROCESS_QUERY_INFORMATION | ✅ Same user | ⚠️ Cross-user |

### **Security Best Practices**

```go
// Use minimal required permissions
const MINIMAL_QUERY_RIGHTS = PROCESS_QUERY_LIMITED_INFORMATION

// Graceful permission fallback
func openProcessWithFallback(pid int) (syscall.Handle, error) {
    // Try limited rights first (Vista+)
    handle, err := syscall.OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
    if err == nil {
        return handle, nil
    }

    // Fallback to full query rights
    handle, err = syscall.OpenProcess(PROCESS_QUERY_INFORMATION, false, uint32(pid))
    if err == nil {
        return handle, nil
    }

    return 0, fmt.Errorf("insufficient permissions to open process %d", pid)
}
```

---

## 📊 **Windows Version Compatibility Matrix**

| API Category | Windows 7 | Windows 10 | Windows 11 | Notes |
|--------------|-----------|------------|------------|-------|
| **Process Management** | ✅ **Full Support** | ✅ **Full Support** | ✅ **Full Support** | Core APIs available since Windows XP |
| **Console Signal Handling** | ✅ **Full Support** | ✅ **Full Support** | ✅ **Full Support** | AttachConsole hack works across all versions |
| **Job Objects** | ✅ **Full Support** | ✅ **Enhanced** | ✅ **Enhanced** | Basic support since Windows 2000, improvements in newer versions |
| **Process Monitoring** | ✅ **Full Support** | ✅ **Enhanced** | ✅ **Enhanced** | psapi.dll APIs, some moved to kernel32.dll in Vista+ |
| **Limited Query Rights** | ✅ **Available** | ✅ **Available** | ✅ **Available** | PROCESS_QUERY_LIMITED_INFORMATION since Vista |
| **Security Model** | ✅ **UAC** | ✅ **Enhanced UAC** | ✅ **Enhanced UAC** | Consistent behavior across versions |

### **Version-Specific Enhancements**

#### **Windows 7 (Baseline)**
- ✅ **All core APIs functional** - Complete process management and monitoring
- ✅ **Job Objects fully supported** - Resource limits enforcement
- ✅ **Console signal handling** - AttachConsole hack functional
- ✅ **UAC compatibility** - Proper permission handling

#### **Windows 10 (Enhanced)**
- 🚀 **Improved Job Objects** - Additional limit types and better performance
- 🚀 **Enhanced security** - Better isolation and permission checking
- 🚀 **Performance improvements** - Optimized API implementations
- 🚀 **Better error reporting** - More detailed error messages

#### **Windows 11 (Latest)**
- 🚀 **All Windows 10 features** - Full backward compatibility
- 🚀 **Enhanced security model** - Additional security features
- 🚀 **Performance optimizations** - Further improved API performance
- 🚀 **Modern development support** - Better debugging and profiling tools

---

## ⚖️ **Windows vs Cross-Platform Considerations**

### **✅ Windows-Specific Advantages**

- 🚀 **Job Objects**: Powerful, centralized resource management unavailable on other platforms
- 🚀 **Atomic Resource Limits**: OS-level enforcement with immediate effect
- 🚀 **Handle Management**: Comprehensive handle tracking and monitoring
- 🚀 **Process Groups**: Clean process isolation and signal routing
- 🚀 **Console Integration**: Rich console signal handling capabilities

### **⚠️ Windows-Specific Challenges**

- ⚠️ **Console Signal Complexity**: Requires workarounds like AttachConsole hack
- ⚠️ **Permission Model**: More complex than Unix process permissions
- ⚠️ **API Proliferation**: Multiple DLLs (kernel32, psapi) required for complete functionality
- ⚠️ **Platform-Specific Code**: Significant Windows-specific implementation required

### **🔄 Cross-Platform Abstraction Strategy**

```go
// Platform abstraction interface
type ResourceManager interface {
    Initialize() error
    GetUsage() (*ResourceUsage, error)
    ApplyLimits(*ResourceLimits) error
    CheckViolations() ([]*ResourceViolation, error)
    Cleanup() error
}

// Windows-specific implementation
type windowsResourceManager struct {
    monitor  *windowsResourceMonitor
    enforcer *windowsResourceEnforcer
    jobObjects map[int]syscall.Handle
}

// Factory function for platform-specific creation
func NewResourceManager(platform string, config interface{}) (ResourceManager, error) {
    switch platform {
    case "windows":
        return NewWindowsResourceManager(config.(*WindowsConfig)), nil
    case "darwin":
        return NewMacOSResourceManager(config.(*MacOSConfig)), nil
    case "linux":
        return NewLinuxResourceManager(config.(*LinuxConfig)), nil
    default:
        return nil, fmt.Errorf("unsupported platform: %s", platform)
    }
}
```

---

## 🚨 **Critical Windows-Specific Behaviors**

### **1. AttachConsole Dead PID Hack**

**Problem**: Windows console signal handling becomes corrupted after child process termination
**Solution**: Call `AttachConsole(deadPID)` to reset console state
**Critical Detail**: The API call **must fail** for the hack to work

```go
// This is THE critical Windows workaround
func consoleSignalFix(deadPID int) error {
    err := AttachConsole(deadPID)
    if err != nil {
        // SUCCESS: Error means PID is dead, console state reset worked
        return nil
    }
    // FAILURE: PID wasn't actually dead, hack didn't work
    return fmt.Errorf("AttachConsole unexpectedly succeeded for PID %d", deadPID)
}
```

**Operational Flow**:
```
Child Process Dies → Get Dead PID → AttachConsole(deadPID) → Expected Failure → Console Reset → Ctrl+C Restored
```

### **2. Process Group Signal Isolation**

**CREATE_NEW_PROCESS_GROUP Strategy**:
```go
// Children get separate process group for independent termination
cmd.SysProcAttr = &syscall.SysProcAttr{
    CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
}
```

**Benefits**:
- ✅ Child processes can be terminated independently via `GenerateConsoleCtrlEvent`
- ✅ Master process retains normal console signal handling
- ✅ Prevents signal cascade during process restart cycles
- ✅ Enables graceful shutdown sequences

### **3. Job Object Resource Enforcement**

**Atomic Resource Application**:
```go
// Job Objects provide immediate, OS-level resource enforcement
extendedInfo.BasicLimitInformation.LimitFlags |= JOB_OBJECT_LIMIT_WORKINGSET
extendedInfo.BasicLimitInformation.MaximumWorkingSetSize = uintptr(maxMemory)

// OS immediately enforces these limits - no polling required
SetInformationJobObject(jobHandle, JobObjectExtendedLimitInformation, &extendedInfo)
```

**Enforcement Characteristics**:
- ⚡ **Immediate Effect**: Limits applied instantly by Windows kernel
- 🛡️ **Tamper Proof**: Cannot be bypassed by target processes
- 🔄 **Persistent**: Limits remain active until job object destruction
- 📊 **Queryable**: Current usage and limit status available via QueryInformationJobObject

---

## 💡 **Implementation Recommendations**

### **🎯 Production Deployment Strategy**

**Phase 1: Core Implementation (Windows 7+)**
- ✅ Implement basic process management with Job Objects
- ✅ Deploy AttachConsole console signal fix
- ✅ Use PROCESS_QUERY_LIMITED_INFORMATION for security
- ✅ Implement resource monitoring with psapi.dll APIs

**Phase 2: Enhanced Features (Windows 10+)**
- 🚀 Leverage enhanced Job Object capabilities
- 🚀 Implement advanced CPU percentage limiting
- 🚀 Add comprehensive I/O monitoring
- 🚀 Utilize improved error reporting

**Phase 3: Modern Features (Windows 11+)**
- 🌟 Integrate with modern Windows security model
- 🌟 Utilize latest performance optimizations
- 🌟 Implement advanced debugging and profiling integration

### **🛠️ Development Best Practices**

```go
// Always check API availability
func checkWindowsAPIAvailability() error {
    kernel32 := syscall.NewLazyDLL("kernel32.dll")
    
    // Check critical functions
    if err := kernel32.NewProc("CreateJobObjectW").Find(); err != nil {
        return fmt.Errorf("Job Objects not available: %v", err)
    }
    
    if err := kernel32.NewProc("AttachConsole").Find(); err != nil {
        return fmt.Errorf("Console APIs not available: %v", err)
    }
    
    return nil
}

// Graceful degradation strategy
func createResourceManager() ResourceManager {
    if checkWindowsAPIAvailability() == nil {
        return newFullWindowsResourceManager()
    } else {
        return newBasicWindowsResourceManager() // Fallback implementation
    }
}
```

### **🔧 Error Handling Strategy**

```go
// Comprehensive Windows error handling
func handleWindowsAPIError(apiName string, err error) error {
    if err == nil {
        return nil
    }
    
    // Extract Windows error code
    if errno, ok := err.(syscall.Errno); ok {
        switch errno {
        case syscall.ERROR_ACCESS_DENIED:
            return fmt.Errorf("%s failed: insufficient permissions", apiName)
        case syscall.ERROR_INVALID_HANDLE:
            return fmt.Errorf("%s failed: invalid handle", apiName)
        case syscall.ERROR_NOT_FOUND:
            return fmt.Errorf("%s failed: process not found", apiName)
        default:
            return fmt.Errorf("%s failed: Windows error %d", apiName, errno)
        }
    }
    
    return fmt.Errorf("%s failed: %v", apiName, err)
}
```

### **📊 Performance Optimization**

```go
// Efficient handle management
type WindowsHandlePool struct {
    processes map[int]syscall.Handle
    mutex     sync.RWMutex
}

func (p *WindowsHandlePool) GetHandle(pid int) (syscall.Handle, error) {
    p.mutex.RLock()
    if handle, exists := p.processes[pid]; exists {
        p.mutex.RUnlock()
        return handle, nil
    }
    p.mutex.RUnlock()

    // Create new handle with minimal permissions
    handle, err := syscall.OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
    if err != nil {
        return 0, err
    }

    p.mutex.Lock()
    p.processes[pid] = handle
    p.mutex.Unlock()

    return handle, nil
}
```

---

## 📚 **References**

### **Microsoft Documentation**
- [Process and Thread APIs](https://docs.microsoft.com/en-us/windows/win32/procthread/)
- [Job Objects](https://docs.microsoft.com/en-us/windows/win32/procthread/job-objects)
- [Console APIs](https://docs.microsoft.com/en-us/windows/console/console-apis)
- [Performance Monitoring](https://docs.microsoft.com/en-us/windows/win32/perfctrs/performance-counters-portal)

### **Windows Headers**
- `windows.h` - Core Windows API definitions
- `processthreadsapi.h` - Process and thread management
- `jobapi.h` - Job Object APIs
- `psapi.h` - Process Status API
- `wincon.h` - Console API definitions

### **Critical Windows-Specific Knowledge**
- **AttachConsole Dead PID Hack**: Undocumented Windows console signal fix
- **Job Objects**: Powerful Windows-specific resource management
- **Process Groups**: Windows signal isolation mechanism
- **Handle Management**: Windows resource management patterns

---

**Assessment**: Windows provides **excellent process management capabilities** with unique features like Job Objects that exceed Unix/Linux capabilities in some areas. The AttachConsole dead PID hack represents critical Windows-specific knowledge for robust console application development.

**Recommendation**: Windows is a **first-class platform** for HSU Master with all required APIs available and functional across Windows 7+. The platform-specific optimizations (Job Objects, console handling) provide significant advantages for process management applications.

---

**Document Status**: Complete  
**Next Review**: After Windows implementation enhancement  
**Maintainer**: HSU Master Development Team 