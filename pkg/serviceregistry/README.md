# HSU Service Registry - Go Implementation

Dynamic service discovery and registration for HSU modules.

## Quick Start

```bash
# Build standalone server
cd cmd/srv/svcreg
go build

# Run with TCP transport (development)
./svcreg --transport=tcp --tcp-address=127.0.0.1:17951

# Run with Unix socket (production)
./svcreg --transport=unix --socket-path=/var/run/hsu/registry.sock
```

## Usage

```go
import "github.com/core-tools/hsu-core/pkg/serviceregistry"

// Create registry
registry := serviceregistry.NewRegistry(logger)

// Create server
transport := serviceregistry.TransportConfig{
    TransportType: serviceregistry.TransportTCP,
    TCPAddress:    "127.0.0.1:17951",
}
server, err := serviceregistry.NewServer(registry, transport, logger)

// Start server
server.Start(ctx)
```

## Documentation

**📚 Complete documentation:** [`docs/v3/architecture/service-registry/`](../../../../docs/v3/architecture/service-registry/)

- **[Architecture Overview](../../../../docs/v3/architecture/service-registry/README.md)**
- **[Go Implementation Guide](../../../../docs/v3/architecture/service-registry/implementation-go.md)**
- **[API Reference](../../../../docs/v3/architecture/service-registry/implementation-go.md#api-reference)**

## Features

- ✅ HTTP API for publish/discover/unpublish
- ✅ Multiple transports (TCP, Unix sockets, Named Pipes)
- ✅ Automatic stale entry cleanup
- ✅ Thread-safe operations
- ✅ Health check endpoint

---

*Part of HSU Core - Modular System Development*
