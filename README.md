# HSU Process Manager - Go Implementation

Go implementation of the HSU process management.

## Quick Start

```bash
# Build test process
cd cmd/test/echotest
go build
# Build process manager
cd cmd/srv/procman
go build

# Run with example config
.\procman.exe --config config-echotest-unix.yaml
or
./procman --config config-echotest-unix.yaml
```

## Documentation

- **[Architecture](./architecture.md)** - architecture description
- **[Implementation](./implementation.md)** - implementation details
