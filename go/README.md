# HSU Core - Go Implementation

Go implementation of the HSU (Host System Unit) framework for modular system development.

## Quick Start

```bash
# Build process manager
cd cmd/srv/procman
go build

# Run with example config
./procman config-echotest-unix.yaml

# Run tests
cd ../../..
go test ./pkg/...
```

## Project Structure

- **Module Communication** (Universal) - `pkg/modulemanagement/` - Production ready
- **Process Management** (On-Premise) - `pkg/processmanagement/` - Production ready
- **Service Registry** - `pkg/serviceregistry/` - Production ready

## Documentation

**📚 Architecture Documentation:** [`docs/v3/architecture/`](../../docs/v3/architecture/)

- **[Process Management](../../docs/v3/architecture/process-management/)** - Native process orchestration
  - [Overview](../../docs/v3/architecture/process-management/README.md)
  - [Go Implementation Guide](../../docs/v3/architecture/process-management/implementation-go.md)

- **[Service Registry](../../docs/v3/architecture/service-registry/)** - Service discovery
  - [Overview](../../docs/v3/architecture/service-registry/README.md)
  - [Go Implementation](../../docs/v3/architecture/service-registry/implementation-go.md)

- **[Module Communication](../../docs/v3/architecture/module-communication/README.md)** - Universal patterns
- **[Vision & Philosophy](../../docs/v3/VISION.md)** - Framework philosophy

## Status

✅ **Production-Ready**
- All features implemented
- Comprehensive test coverage
- Used in production deployments

## License

Apache-2.0

