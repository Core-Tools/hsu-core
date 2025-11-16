# HSU Core

Modular system development framework with universal module communication and native process management.

## Implementations

- **[Go](go/)** - ✅ Production-ready
- **[Rust](rust/)** - 🚧 Under development  
- **[Python](python/)** - 📋 Planned

## Documentation

**📚 Complete Architecture Documentation:** [`docs/v3/`](../docs/v3/)

### Main Documentation
- **[VISION](../docs/v3/VISION.md)** - Framework philosophy and scope
- **[INDEX](../docs/v3/INDEX.md)** - Documentation navigation
- **[Architecture Overview](../docs/v3/architecture/README.md)**

### Core Components
- **[Process Management](../docs/v3/architecture/process-management/README.md)** - Native process orchestration (on-premise)
- **[Module Communication](../docs/v3/architecture/module-communication/README.md)** - Location-transparent communication (universal)
- **[Service Registry](../docs/v3/architecture/service-registry/README.md)** - Service discovery
- **[Code Organization](../docs/v3/architecture/code-organization/README.md)** - Repository patterns

## Quick Start

### Go
```bash
cd go/cmd/srv/procman
go build
./procman config-example.yaml
```

### Rust
```bash
cd rust
cargo build --workspace
cd crates/hsu-process-manager
cargo run -- --config config-example.yaml
```

## License

Apache-2.0

---

*Part of: Modular System Development Framework*
