# HSU Process Management Implementation

---

## Package Structure

```
hsu-procman-go/
├── pkg/
│   ├── process/                    # Low-level process operations
│   │   ├── execute.go             # Common interface
│   │   ├── execute_unix.go        # Unix implementation  
│   │   ├── execute_windows.go     # Windows implementation
│   │   ├── terminate_unix.go
│   │   ├── terminate_windows.go
│   │   ├── attach.go              # Process attachment
│   │   └── validation.go
│   │
│   ├── processstate/               # Process state checks
│   │   ├── is_running_unix.go
│   │   └── is_running_windows.go
│   │
│   ├── processfile/                # State persistence
│   │   └── process_file.go
│   │
│   ├── managedprocess/             # Process abstractions
│   │   ├── process_desc.go        # Process interface
│   │   ├── standard_managed_process.go
│   │   ├── integrated_managed_process.go
│   │   ├── unmanaged_process.go
│   │   └── processcontrol/
│   │       ├── process_control_iface.go
│   │       ├── process_state.go
│   │       └── restart_config.go
│   │   └── processcontrolimpl/
│   │       ├── process_control_impl.go
│   │       └── restart_circuit_breaker.go
│   │
│   ├── processmanagement/          # Main orchestration
│   │   ├── manager.go             # ProcessManager
│   │   ├── config.go              # Configuration
│   │   ├── validation.go
│   │   ├── runner.go              # Execution logic
│   │   ├── processstatemachine/
│   │   │   └── process_state_machine.go
│   │   └── logcollection_integration.go
│   │
│   ├── monitoring/                 # Health checking
│   │   ├── health_check.go
│   │   └── validation.go
│   │
│   ├── resourcelimits/             # Resource management
│   │   ├── interface.go
│   │   ├── enforcer.go            # Common interface
│   │   ├── enforcer_linux.go      # Linux cgroups
│   │   ├── enforcer_darwin.go     # macOS limits
│   │   ├── enforcer_windows.go    # Windows job objects
│   │   ├── monitor.go
│   │   ├── platform_monitor_linux.go
│   │   ├── platform_monitor_darwin.go
│   │   ├── platform_monitor_windows.go
│   │   ├── manager.go
│   │   └── violation_checker.go
│   │
│   └── logcollection/              # Log aggregation
│       ├── interface.go
│       ├── service.go
│       ├── factory.go
│       ├── zap_adapter.go         # Zap logger integration
│       ├── field.go               # Log field types
│       ├── config/
│       │   └── config.go
│       ├── output/                # Output targets
│       └── processing/            # Log processing
│
└── cmd/srv/procman/               # Binary
    ├── main.go
    ├── config-*.yaml              # Example configs
    └── ...
```

## Key Features (All Implemented)

### ✅ Complete Feature Set

**Process Lifecycle:**
- Standard managed processes (full control)
- Integrated managed processes (with gRPC)
- Unmanaged processes (monitoring only)
- Graceful and forceful termination
- Process attachment/reattachment

**Health Monitoring:**
- HTTP health checks
- gRPC health protocol
- Configurable intervals and thresholds
- Failure tracking and recovery

**Resource Management:**
- CPU limit enforcement (cgroups/job objects)
- Memory limit enforcement
- File descriptor limits (Unix)
- Real-time monitoring
- Violation detection and actions

**Restart Policies:**
- Never, OnFailure, Always strategies
- Configurable backoff multipliers
- Circuit breaker pattern
- Maximum attempt limits

**Log Collection:**
- stdout/stderr capture
- Multiple output targets
- Zap logger integration
- Structured log enhancement
- Metadata enrichment

**Configuration:**
- YAML-based configuration
- Comprehensive validation
- Hot-reload support (planned)
- Environment variable support

## Package Details

### `pkg/process`
Low-level cross-platform process primitives. Implements platform-specific functionality with a common interface.

**Key Functions:**
- `Execute(config ProcessControlConfig) (ProcessHandle, error)`
- `TerminateGracefully(pid int, timeout time.Duration) error`
- `TerminateForce(pid int) error`
- `Attach(pid int) (ProcessHandle, error)`

### `pkg/managedprocess`
Process type abstractions with lifecycle management.

**Types:**
- `StandardManagedProcess` - Full lifecycle control
- `IntegratedManagedProcess` - With gRPC health checks
- `UnmanagedProcess` - Monitoring only

**Interface:**
```go
type ProcessOptions interface {
    ID() string
    Metadata() ProcessMetadata
    ProcessControlOptions() processcontrol.ProcessControlOptions
}
```

### `pkg/processmanagement`
Main orchestration with ProcessManager coordinating all processes.

**Core Type:**
```go
type ProcessManager interface {
    ProcessRegistry
    ProcessLifecycle
    LogCollectorIntegration
}

type ProcessLifecycle interface {
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    StartProcess(ctx context.Context, id string) error
    StopProcess(ctx context.Context, id string) error
    GetManagerState() ProcessManagerState
    GetAllProcessStatesWithDiagnostics() map[string]ProcessStateWithDiagnostics
}
```

### `pkg/monitoring`
Health check implementations for HTTP and gRPC protocols.

**Features:**
- Configurable check intervals
- Timeout handling
- Failure threshold tracking
- Multiple protocol support

### `pkg/resourcelimits`
Platform-specific resource enforcement.

**Linux:** cgroups v2 for CPU/memory  
**Windows:** Job objects for limits  
**macOS:** setrlimit for basic limits

### `pkg/logcollection`
Comprehensive log aggregation with multiple outputs.

**Features:**
- Capture stdout/stderr
- Forward to multiple targets
- Add metadata (timestamps, PIDs, etc.)
- Integration with structured logging (Zap)
- Buffer management

## Example Usage

```go
package main

import (
    "context"
    "log"
    "github.com/core-tools/hsu-procman-go/pkg/processmanagement"
)

func main() {
    // Load configuration
    config, err := processmanagement.LoadConfig("config.yaml")
    if err != nil {
        log.Fatalf("Failed to load config: %v", err)
    }

    // Create manager
    manager, err := processmanagement.NewProcessManager(config)
    if err != nil {
        log.Fatalf("Failed to create manager: %v", err)
    }

    // Start all processes
    ctx := context.Background()
    if err := manager.Start(ctx); err != nil {
        log.Fatalf("Failed to start: %v", err)
    }

    // Wait for signal
    // ... signal handling ...

    // Graceful shutdown
    if err := manager.Stop(ctx); err != nil {
        log.Fatalf("Failed to stop: %v", err)
    }
}
```

## Testing

The Go implementation has comprehensive test coverage:

```bash
# Run all tests
go test ./pkg/...

# Run with coverage
go test -cover ./pkg/...

# Run specific package tests
go test ./pkg/processmanagement/...

# Run benchmarks
go test -bench=. ./pkg/...
```

**Test Types:**
- Unit tests for all packages
- Integration tests for full flows
- Benchmark tests for performance
- Concurrency tests for race conditions
- Edge case tests for error handling

## Configuration

Example configuration:

```yaml
process_manager:
  port: 50055
  log_level: "info"
  force_shutdown_timeout: "30s"

managed_processes:
  - id: "my-service"
    type: "standard_managed"
    profile_type: "long_running_service"
    enabled: true
    management:
      control:
        executable: "./my-service"
        arguments: ["--port", "8080"]
        working_directory: "/opt/service"
        environment:
          SERVICE_ENV: "production"
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
        buffer_size: 8192

log_collection:
  enabled: true
  global_aggregation:
    enabled: true
    targets:
      - type: "file"
        path: "/var/log/hsu/processes.log"
        format: "json"
      - type: "process_manager_stdout"
        format: "plain"
```

## Resources

- **Source Code:** `hsu-procman-go/pkg/`
- **Examples:** `hsu-procman-go/cmd/srv/procman/config-*.yaml`
- **Tests:** Run `go test ./pkg/...` for comprehensive examples

## Related Documentation

- **[Architecture](./architecture.md)** - architecture description


---
