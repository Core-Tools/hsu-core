# HSU Process Management Architecture

---

## Overview

This document describes the architectural patterns and design principles of the HSU Process Management system. The architecture emphasizes separation of concerns, interface-based design, and platform abstraction.

## Core Architecture Pattern

Both implementations follow a **layered architecture** with trait/interface-based separation:

```
┌─────────────────────────────────────────┐
│         ProcessManager                  │  Layer 5: Orchestration
│    (Coordinates all processes)          │
└──────────────┬──────────────────────────┘
               │ uses
               ▼
┌─────────────────────────────────────────┐
│      ProcessControl Interface           │  Layer 4: Abstraction
│  ProcessControlIface (interface)        │
└──────────────┬──────────────────────────┘
               │ implemented by
               ▼
┌─────────────────────────────────────────┐
│     ProcessControlImpl                  │  Layer 3: Implementation
│  • Process spawning & termination       │
│  • Health check integration             │
│  • Resource monitoring                  │
│  • Restart policy & circuit breaker     │
└──────────────┬──────────────────────────┘
               │ uses
               ▼
┌─────────────────────────────────────────┐
│     Cross-Cutting Concerns              │  Layer 2: Services
│  • State Machine                        │
│  • Health Monitoring                    │
│  • Resource Limits                      │
│  • Log Collection                       │
│  • PID File Management                  │
└──────────────┬──────────────────────────┘
               │ uses
               ▼
┌─────────────────────────────────────────┐
│     Platform Primitives                 │  Layer 1: Low-level
│  • process_spawn / Execute              │
│  • process_kill / Terminate             │
│  • process_exists / IsRunning           │
└─────────────────────────────────────────┘
```

## Design Principles

### 1. Separation of Concerns

**Orchestration vs Implementation**

- **ProcessManager**: High-level coordination, configuration, API exposure
- **ProcessControl**: Lifecycle operations interface/trait
- **ProcessControlImpl**: Actual implementation of complex logic

**Go Example:**
```go
// Orchestration layer
type ProcessManager struct {
    processes map[string]*ManagedProcessInstance
}

// Interface layer
type ProcessControlIface interface {
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
}

// Implementation layer
type ProcessControlImpl struct {
    // Complex logic here
}
```

### 2. Interface-Based Design

**Benefits:**
- ✅ **Testability**: Can mock for unit tests
- ✅ **Extensibility**: Add new implementations easily
- ✅ **Decoupling**: Manager doesn't know about implementation details
- ✅ **Flexibility**: Swap implementations at runtime

**Go Interface:**
```go
type ProcessControlIface interface {
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    Restart(ctx context.Context, force bool) error
    GetState() ProcessState
    GetDiagnostics() ProcessDiagnostics
}
```

### 3. Process Type Hierarchy

Three process types with different capabilities:

| Type | Spawn | Monitor | Restart | Terminate | Use Case |
|------|-------|---------|---------|-----------|----------|
| **StandardManaged** | ✅ | ✅ | ✅ | ✅ | Custom apps, services |
| **IntegratedManaged** | ✅ | ✅ (gRPC) | ✅ | ✅ | HSU-aware services |
| **Unmanaged** | ❌ | ✅ | ❌ | ❌ | External systems (DB, etc) |

**Configuration Differences:**

```yaml
# StandardManaged
- id: "api-server"
  type: "standard_managed"      # Full control
  management:
    control:
      executable: "./server"
    health_check:
      http_endpoint: "http://localhost:8080/health"
    restart_policy:
      strategy: "on_failure"

# IntegratedManaged
- id: "llm-service"
  type: "integrated_managed"    # + gRPC integration
  management:
    health_check:
      grpc_service: "localhost:50051"
      grpc_health_check: true

# Unmanaged
- id: "postgres"
  type: "unmanaged"             # Monitoring only
  # No restart_policy, no control section
  # Can only attach and monitor
```

### 4. State Machine

All processes follow a validated state machine:

```
     ┌──────────────────────────────────┐
     │                                  │
     ▼                                  │
  Stopped ──────► Starting ──────► Running
     ▲               │                │
     │               ▼                ▼
     │            Failed         Stopping
     │               │                │
     └───────────────┴────────────────┘
```

**State Transitions:**
- `Stopped` → `Starting`: Process spawn initiated
- `Starting` → `Running`: Process successfully started
- `Running` → `Stopping`: Graceful shutdown initiated
- `Stopping` → `Stopped`: Process terminated
- `Starting/Running` → `Failed`: Error occurred
- `Failed` → `Starting`: Restart policy triggered

**Implementation:**

Both implementations use a state machine with:
- Validated transitions (invalid transitions return errors)
- State history tracking
- Transition timestamps
- Current state queries

### 5. Platform Abstraction

Cross-platform operations with platform-specific implementations:

**Go:**
```go
// Common interface
func Execute(config ProcessControlConfig) (ProcessHandle, error)

// Platform-specific files
execute_unix.go      // Unix/Linux/macOS
execute_windows.go   // Windows
```

## Component Responsibilities

### ProcessManager (Orchestration)

**Responsibilities:**
- Load and validate configuration
- Initialize processes from config
- Coordinate process lifecycle
- Expose management APIs
- Handle reattachment after restart
- Aggregate process information

**Does NOT:**
- Spawn processes directly
- Manage health checks directly
- Implement restart logic
- Handle platform-specific details

### ProcessControl (Interface/Trait)

**Defines:**
- Lifecycle operations (start/stop/restart)
- State queries
- Diagnostics access
- Error handling contract

**Benefits:**
- Clear contract between layers
- Testability via mocking
- Extensibility for new types

### ProcessControlImpl (Implementation)

**Responsibilities:**
- Process spawning and termination
- Health check background tasks
- Resource monitoring background tasks
- Restart policy enforcement
- Circuit breaker logic
- State machine integration

**This is where the complexity lives!**

### State Machine

**Responsibilities:**
- Validate state transitions
- Track state history
- Enforce valid operations per state
- Provide state queries

### Health Monitoring

**Responsibilities:**
- Execute health checks (HTTP/gRPC)
- Track consecutive failures
- Trigger restart on threshold
- Report health status

### Resource Monitoring

**Responsibilities:**
- Collect CPU/memory/FD usage
- Check against limits
- Trigger actions on violations
- Platform-specific collection

### Log Collection

**Responsibilities:**
- Capture stdout/stderr
- Route to multiple targets
- Enhance with metadata
- Buffer management

## Key Patterns

### 1. Background Task Management

Both implementations spawn background tasks for monitoring:

**Go:**
```go
// Spawn goroutine
go func() {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    
    for range ticker.C {
        // Perform health check
    }
}()
```

### 2. Restart Policy Enforcement

**Never:** Process never restarts automatically
```yaml
restart_policy:
  strategy: "never"
```

**OnFailure:** Restart only on non-zero exit
```yaml
restart_policy:
  strategy: "on_failure"
  max_attempts: 3
  restart_delay: "5s"
  backoff_multiplier: 1.5
```

**Always:** Restart regardless of exit code
```yaml
restart_policy:
  strategy: "always"
  max_attempts: 10
  restart_delay: "2s"
```

**Circuit Breaker:**
- Tracks failures in time window (60s)
- Trips after threshold (5 failures)
- Cooldown period (5 minutes)
- Prevents restart loops

### 3. Graceful Shutdown

Both implementations follow the same pattern:

1. **Send termination signal** (SIGTERM/Ctrl+C)
2. **Wait for graceful timeout** (configurable, default 10s)
3. **Force kill if needed** (SIGKILL/TerminateProcess)
4. **Clean up resources** (PID files, background tasks)

### 4. Process Reattachment

Survives manager restarts:

1. **Before shutdown**: Write PID file with state
2. **On startup**: Scan for PID files
3. **For each PID**: Check if process exists
4. **If exists**: Reattach and resume monitoring
5. **If not exists**: Clean up stale PID file

## Configuration Schema

Implementations uses YAML:

```yaml
process_manager:
  port: 50055
  log_level: "info"
  force_shutdown_timeout: "30s"

managed_processes:
  - id: "my-service"
    type: "standard_managed"
    enabled: true
    management:
      control:
        executable: "./service"
        arguments: ["--config", "prod.yaml"]
        working_directory: "/opt/service"
        environment:
          ENV: "production"
        startup_timeout: "60s"
        shutdown_timeout: "10s"
      
      health_check:
        enabled: true
        interval: "30s"
        timeout: "5s"
        failure_threshold: 3
        http_endpoint: "http://localhost:8080/health"
      
      resource_limits:
        max_memory_mb: 512
        max_cpu_percent: 80.0
        max_file_descriptors: 1024
      
      restart_policy:
        strategy: "on_failure"
        max_attempts: 3
        restart_delay: "5s"
        backoff_multiplier: 1.5
      
      logging:
        capture_stdout: true
        capture_stderr: true
```

## Future Enhancements

### Planned Features
- **Process Migration**: Move processes between managers
- **Distributed Coordination**: Multi-manager sync
- **Hot Config Reload**: Update without restart
- **Enhanced Metrics**: Prometheus export
- **Process Groups**: Hierarchical management

### Research Areas
- **Container Integration**: Optional containerization
- **Resource Quotas**: Advanced cgroup/job object usage
- **Network Management**: Port allocation, firewall integration

## Related Documentation

- **[Implementation](./implementation.md)** - implementation details

---
