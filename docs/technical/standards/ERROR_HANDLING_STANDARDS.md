# HSU Master Error Handling & Logging Standards

**Purpose**: Comprehensive guidelines for consistent error handling and logging throughout the HSU Master codebase  
**Audience**: Developers, maintainers, code reviewers  
**Status**: **PRODUCTION READY** - Based on comprehensive Phase 1 implementation  

---

## 🎯 **Overview**

HSU Master implements enterprise-grade error handling with **structured domain errors** and **context-aware logging** to support multi-worker debugging and production monitoring.

### **Key Principles**

1. **🏗️ Domain-Driven Errors**: Use structured `DomainError` types instead of primitive `fmt.Errorf`
2. **📍 Context Enrichment**: Every error and log must include relevant context (PID, worker ID, operation details)
3. **🔄 Error Aggregation**: Use `ErrorCollection` for bulk operations with multiple potential failures
4. **📊 Structured Logging**: Leverage parameterized loggers with context propagation

---

## 📚 **Domain Error Types**

### **Available Error Types**

| Type | Usage | Context Required |
|------|-------|------------------|
| `ErrorTypeValidation` | Input validation, configuration errors | Field names, values |
| `ErrorTypeProcess` | Process lifecycle, resource operations | PID, worker ID |
| `ErrorTypeNotFound` | Missing resources, workers | Resource ID, type |
| `ErrorTypeConflict` | Duplicate registrations, state conflicts | Resource ID, state |
| `ErrorTypeInternal` | System failures, unexpected conditions | Component, operation |
| `ErrorTypePermission` | Access control, privilege issues | Resource, required privileges |
| `ErrorTypeTimeout` | Operation timeouts | Duration, operation |
| `ErrorTypeIO` | File operations, network issues | Path, operation |

### **Error Creation Examples**

#### ✅ **Correct Pattern**
```go
// Import required
import "github.com/core-tools/hsu-master/pkg/errors"

// Process-related error with context
return errors.NewProcessError("process is not running", err).
    WithContext("pid", pid).
    WithContext("worker_id", workerID)

// Validation error with field context
return errors.NewValidationError("invalid configuration", err).
    WithContext("field", "max_retries").
    WithContext("value", config.MaxRetries)
```

#### ❌ **Incorrect Pattern**
```go
// Avoid primitive errors without context
return fmt.Errorf("process %d is not running", pid)
return errors.New("worker not found")
```

---

## 🔄 **Error Aggregation Pattern**

### **Using ErrorCollection**

For operations that can have multiple failures (e.g., applying resource limits):

```go
func (re *resourceEnforcer) ApplyLimits(pid int, limits *ResourceLimits) error {
    errorCollection := errors.NewErrorCollection()
    
    // Apply memory limits
    if limits.Memory != nil {
        if err := re.applyMemoryLimits(pid, limits.Memory); err != nil {
            wrappedErr := errors.NewProcessError("failed to apply memory limits", err).
                WithContext("pid", pid)
            errorCollection.Add(wrappedErr)
            re.logger.Warnf("Failed to apply memory limits to PID %d: %v", pid, err)
        }
    }
    
    // Apply CPU limits  
    if limits.CPU != nil {
        if err := re.applyCPULimits(pid, limits.CPU); err != nil {
            wrappedErr := errors.NewProcessError("failed to apply CPU limits", err).
                WithContext("pid", pid)
            errorCollection.Add(wrappedErr)
            re.logger.Warnf("Failed to apply CPU limits to PID %d: %v", pid, err)
        }
    }
    
    // Return aggregated errors
    if errorCollection.HasErrors() {
        return errors.NewProcessError("failed to apply some resource limits", 
            errorCollection.ToError()).WithContext("pid", pid)
    }
    
    return nil
}
```

---

## 📊 **Logging Standards**

### **Logger Context Patterns**

#### **Worker-Specific Loggers**
```go
// Create parameterized logger for worker
logger := masterlogging.NewLogger("worker: "+workerID+" , ", masterlogging.LogFuncs{
    Debugf: m.logger.Debugf,
    Infof:  m.logger.Infof,
    Warnf:  m.logger.Warnf,
    Errorf: m.logger.Errorf,
})
```

#### **Required Context in Log Messages**

| Operation Type | Required Context | Example |
|----------------|------------------|---------|
| **Process Operations** | PID + Worker ID | `"Starting resource monitoring for PID %d, worker: %s"` |
| **Resource Management** | PID + Resource Type + Values | `"Memory violation for PID %d: RSS (%d bytes) exceeds limit (%d bytes)"` |
| **Worker Lifecycle** | Worker ID + State + Operation | `"Worker state transition, worker: %s, %s->%s, operation: %s"` |
| **Error Conditions** | PID/Worker + Error Context | `"Failed to apply memory limits to PID %d: %v"` |

### **Log Level Guidelines**

| Level | Usage | Examples |
|-------|-------|----------|
| `Debug` | Detailed operational flow | Resource usage values, state transitions |
| `Info` | Important lifecycle events | Worker starts, stops, successful operations |
| `Warn` | Recoverable issues | Failed resource applications, retry attempts |
| `Error` | Critical failures | Unrecoverable errors, system failures |

---

## 🏭 **Package-Specific Patterns**

### **ResourceLimits Package** ✅ **IMPLEMENTED**

**Pattern**: All operations include PID context, use ErrorCollection for multi-step processes

```go
// ✅ Good: Domain error with PID context
return errors.NewProcessError("process is not running", err).WithContext("pid", pid)

// ✅ Good: Warning logs for failed operations
re.logger.Warnf("Failed to apply memory limits to PID %d: %v", pid, err)

// ✅ Good: ErrorCollection for aggregation
errorCollection := errors.NewErrorCollection()
errorCollection.Add(wrappedErr)
```

### **LogCollection Package** 🚧 **PARTIALLY IMPLEMENTED**

**Pattern**: Worker ID context in all operations, structured logging with fields

```go
// ✅ Good: Worker context in errors
return errors.NewConflictError("worker already registered", nil).WithContext("worker_id", workerID)

// ✅ Good: Structured logging with worker context
s.logger.WithFields(
    String("worker_id", workerID),
    String("operation", "register"),
).Infof("Worker registered for log collection")
```

### **Master Package** ✅ **EXEMPLARY**

**Best Practice Example**: Comprehensive error handling with full context

```go
// ✅ Excellent: Full context chain
return errors.NewValidationError("failed to create workers from configuration", err).
    WithContext("config_file", configFile).
    WithContext("worker_count", len(config.Workers))
```

---

## 🛠️ **Implementation Checklist**

### **For New Code**
- [ ] Import `"github.com/core-tools/hsu-master/pkg/errors"`
- [ ] Use appropriate domain error types
- [ ] Add relevant context to all errors
- [ ] Include PID/Worker ID in all process-related operations
- [ ] Use ErrorCollection for multi-step operations
- [ ] Add warning logs for non-critical failures

### **For Error Handling Review**
- [ ] Convert `fmt.Errorf` → domain errors
- [ ] Convert `errors.New` → domain errors  
- [ ] Add missing context to existing errors
- [ ] Enhance log messages with operation context
- [ ] Replace `[]error` → `ErrorCollection`

### **For Multi-Worker Debugging**
- [ ] Every error includes worker identification
- [ ] Log messages distinguish between workers
- [ ] Resource violations include PID context
- [ ] State transitions include worker ID

---

## 🔍 **Debugging Examples**

### **Before Implementation**
```
❌ Bad: "failed to apply memory limits: permission denied"
❌ Bad: "worker not found"  
❌ Bad: "resource violation detected"
```

### **After Implementation**
```
✅ Good: "failed to apply memory limits to PID 1234: permission denied (worker: web-server)"
✅ Good: "worker not registered: worker_id=api-gateway"
✅ Good: "Resource violation for PID 1234: Memory RSS (512MB) exceeds limit (256MB), worker: database"
```

---

## 📈 **Benefits Achieved**

### **Multi-Worker Production Debugging**
- **🎯 Precise Error Attribution**: Every error traceable to specific worker/PID
- **📊 Structured Context**: Machine-readable error context for monitoring
- **🔄 Comprehensive Aggregation**: Multiple failures properly collected and reported
- **⚡ Fast Troubleshooting**: Context-rich logs enable rapid issue resolution

### **Code Quality**
- **🏗️ Type Safety**: Domain errors enable proper error type checking
- **📚 Maintainability**: Consistent patterns across all packages
- **🔧 Extensibility**: Easy to add new error types and context
- **✅ Testability**: Structured errors easier to test and validate

---

## 📋 **Next Steps**

### **Immediate (High Priority)**
1. **Complete remaining packages**: Apply patterns to `monitoring`, `process`, `control`
2. **Standardize imports**: Ensure all packages use correct error import paths
3. **Add missing context**: Review all log messages for worker/PID context

### **Future Enhancements**
1. **Metrics Integration**: Connect error types to monitoring dashboards
2. **Alert Correlation**: Use error context for intelligent alerting
3. **Documentation**: Generate error handling documentation from code

---

*Implementation Guide*: Based on comprehensive Phase 1 fixes in `resourcelimits` package  
*Production Status*: **READY** - Patterns validated in production environment  
*Last Updated*: January 2025  
*Maintainer*: HSU Master Development Team  