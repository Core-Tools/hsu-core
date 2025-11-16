# HSU Core - Rust Implementation

Rust implementation of the HSU (Host System Unit) framework for modular system development.

## Quick Start

```bash
# Build all crates
cargo build --workspace

# Run process manager
cd crates/hsu-process-manager
cargo run -- --config config-example.yaml --debug

# Run tests
cargo test --workspace
```

## Project Structure

- **Module Communication** (Universal) - `crates/hsu-module-*` - Production ready
- **Process Management** (On-Premise) - `crates/hsu-process-*` - Under development

## Documentation

**📚 Architecture Documentation:** [`docs/v3/architecture/`](../../docs/v3/architecture/)

- **[Process Management](../../docs/v3/architecture/process-management/)** - Architecture, implementation details
  - [Overview](../../docs/v3/architecture/process-management/README.md)
  - [Rust Implementation Guide](../../docs/v3/architecture/process-management/implementation-rust.md)
  - [Restructuring Notes](../../docs/v3/architecture/process-management/restructuring-notes.md)

- **[Module Communication](../../docs/v3/architecture/module-communication/README.md)** - Universal patterns
- **[Service Registry](../../docs/v3/architecture/service-registry/README.md)** - Service discovery
- **[Vision & Philosophy](../../docs/v3/VISION.md)** - Framework philosophy

## Status

### Module Communication ✅
- Production-ready
- Full parity with Go

### Process Management 🚧
- Restructured November 2025
- Foundation complete
- Advanced features in progress

## License

Apache-2.0
