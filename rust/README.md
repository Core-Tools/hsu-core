# HSU Process Manager - Rust Implementation

A Rust implementation of the HSU (Host System Unit) process manager, providing comprehensive process lifecycle management, monitoring, and orchestration capabilities.

## Project Status

🚧 **UNDER DEVELOPMENT** 🚧

This is the initial project setup and architecture. The implementation is currently in early development phase.

## Architecture Overview

The HSU Process Manager is built with a modular architecture:

```
src/
├── main.rs              # Entry point and CLI
├── lib.rs               # Library exports
├── config/              # Configuration management
│   ├── mod.rs           # YAML config parsing and validation
│   └── validation.rs    # Configuration validation logic
├── process/             # Core process management
│   ├── mod.rs           # Main process manager
│   ├── state.rs         # Process state machine
│   ├── lifecycle.rs     # Restart policies and lifecycle
│   ├── monitoring.rs    # Resource monitoring
│   └── attachment.rs    # Process reattachment after restart
├── health/              # Health checking
│   ├── mod.rs           # Health check manager
│   ├── http.rs          # HTTP health checks
│   └── grpc.rs          # gRPC health checks
├── logging/             # Log collection and management
├── monitoring/          # System-wide monitoring
├── api/                 # Management APIs
│   ├── grpc.rs          # gRPC management API
│   └── http.rs          # HTTP management API
├── errors.rs            # Comprehensive error types
└── utils.rs             # Utility functions
```

## Features (Planned)

### ✅ Implemented
- [x] Project structure and build system
- [x] Configuration management with YAML support
- [x] Process state machine
- [x] Basic process spawning and lifecycle
- [x] Error handling architecture
- [x] Process attachment/reattachment framework
- [x] Resource monitoring foundation

### 🚧 In Progress
- [ ] Complete process lifecycle management
- [ ] Health checking (HTTP/gRPC)
- [ ] Log collection and aggregation
- [ ] Resource limits enforcement
- [ ] Management APIs

### 📋 Planned
- [ ] Three process types (managed/integrated/unmanaged)
- [ ] Cross-platform process control
- [ ] Policy-based restart and monitoring
- [ ] Performance optimization
- [ ] Production hardening

## Quick Start

### Prerequisites

- Rust 1.70+ (2021 edition)
- Cargo

### Building

```bash
cd hsu-core/rust
cargo build
```

### Running with Example Configuration

```bash
cargo run -- --config config-example.yaml --debug
```

### Running Tests

```bash
cargo test
```

## Configuration

The process manager uses YAML configuration files compatible with the Go implementation. See `config-example.yaml` for a complete example.

### Basic Configuration Structure

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
        executable: "./my-service"
        arguments: ["--port", "8080"]
        environment:
          SERVICE_ENV: "production"
      health_check:
        enabled: true
        interval: "30s"
      resource_limits:
        max_memory_mb: 512
        max_cpu_percent: 80.0
      restart_policy:
        strategy: "on_failure"
        max_attempts: 3
```

## Development

### Learning Path

This project is designed as a Rust learning journey. The implementation progresses through:

1. **Phase 1**: Basic process management (✅ Current)
2. **Phase 2**: Async operations and monitoring
3. **Phase 3**: Advanced features and optimization
4. **Phase 4**: Production hardening

### Code Organization

- **Modular design**: Each feature is in its own module
- **Error-first**: Comprehensive error handling with `thiserror`
- **Async-first**: Built on `tokio` for scalable I/O
- **Type-safe**: Extensive use of Rust's type system for correctness

### Contributing

This is a learning project, but contributions and feedback are welcome:

1. Focus on learning and code quality over speed
2. Follow Rust best practices and idioms
3. Include comprehensive tests
4. Document architectural decisions

## Comparison with Go Implementation

| Feature | Go Implementation | Rust Implementation |
|---------|------------------|-------------------|
| **Memory Safety** | Runtime (GC) | Compile-time (ownership) |
| **Performance** | Good (GC overhead) | Excellent (zero-cost abstractions) |
| **Concurrency** | Goroutines | async/await + tokio |
| **Error Handling** | Explicit returns | Result types + ? operator |
| **Type Safety** | Good | Excellent (borrowing, lifetimes) |
| **Cross-platform** | Excellent | Excellent |

## License

Apache-2.0

## Related Projects

- [HSU Core (Go)](../go/) - Original Go implementation
- [HSU Core (Python)](../python/) - Python implementation
- [HSU Examples](../../) - Example HSU applications

---

*This is part of the HSU (Host System Unit) framework for microservice orchestration.*
