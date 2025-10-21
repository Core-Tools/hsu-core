# HSU Service Registry

The service registry package provides dynamic service discovery and registration for HSU modules.

## Overview

The service registry is a lightweight, centralized component that:
- Allows modules to publish their API endpoints at runtime
- Enables clients to discover module endpoints dynamically
- Manages service lifecycle with automatic stale entry cleanup
- Supports multiple transport mechanisms (TCP, Unix Domain Sockets, Named Pipes)

## Architecture

```
┌─────────────┐         ┌──────────────┐         ┌─────────────┐
│   Server    │────────>│   Registry   │<────────│   Client    │
│   Module    │ Publish │              │ Discover│   Module    │
└─────────────┘         └──────────────┘         └─────────────┘
```

## Usage

### Standalone Registry Server

```bash
# Start registry on TCP (for development)
./hsu-registry --transport=tcp --tcp-address=127.0.0.1:17951

# Start registry on Unix socket (for production on Linux/macOS)
./hsu-registry --transport=unix --socket-path=/var/run/hsu/registry.sock
```

### Embedded Registry

```go
package main

import (
    "github.com/core-tools/hsu-core/pkg/serviceregistry"
)

func main() {
    // Create registry
    registry := serviceregistry.NewRegistry(logger)
    
    // Create server with TCP transport
    transport := serviceregistry.TransportConfig{
        TransportType: serviceregistry.TransportTCP,
        TCPAddress:    "127.0.0.1:17951",
    }
    
    server, err := serviceregistry.NewServer(registry, transport, logger)
    if err != nil {
        panic(err)
    }
    
    // Start server
    ctx := context.Background()
    server.Start(ctx)
    
    // Server is now running...
    
    // Stop server when done
    server.Stop(ctx)
}
```

### Publishing APIs

```go
// In your module initialization
client, err := moduleapi.NewServiceRegistryClient("tcp://127.0.0.1:17951", logger)
if err != nil {
    return err
}

err = client.PublishAPIs([]moduleapi.RemoteAPI{
    {
        ModuleID: "mymodule",
        ModuleAPIs: []moduleapi.RemoteModuleAPI{
            {
                ServiceIDs: []moduletypes.ServiceID{"service1", "service2"},
                ServerPort: 50051,
                Protocol:   moduletypes.ProtocolGRPC,
            },
        },
    },
})
```

### Discovering APIs

```go
// In your client module
client, err := moduleapi.NewServiceRegistryClient("tcp://127.0.0.1:17951", logger)
if err != nil {
    return err
}

apis, err := client.FindModuleAPIs("mymodule")
if err != nil {
    return err
}

// Use discovered APIs to connect
for _, api := range apis {
    fmt.Printf("Module available at port %d via %s\n", api.ServerPort, api.Protocol)
}
```

## Transport Options

### TCP (Development)

**Advantages:**
- Easy to debug with standard tools (curl, netcat)
- Works on all platforms
- No special file permissions needed

**Disadvantages:**
- No OS-level security
- Network exposure (even if localhost)

**Configuration:**
```go
transport := serviceregistry.TransportConfig{
    TransportType: serviceregistry.TransportTCP,
    TCPAddress:    "127.0.0.1:17951",
}
```

### Unix Domain Sockets (Production on Linux/macOS)

**Advantages:**
- OS-level security (file permissions)
- No network exposure
- Efficient (kernel-only communication)

**Disadvantages:**
- Not available on Windows
- Requires file system access

**Configuration:**
```go
transport := serviceregistry.TransportConfig{
    TransportType: serviceregistry.TransportUDS,
    SocketPath:    "/var/run/hsu/registry.sock",
    FileMode:      0600, // Owner only
}
```

### Named Pipes (Future: Windows Production)

**Note:** Named pipe support requires the `github.com/Microsoft/go-winio` package, which is not currently included as a dependency. For Windows development, use TCP transport.

## API Reference

### HTTP Endpoints

#### POST /api/v1/publish
Publish module APIs.

**Request:**
```json
{
  "moduleID": "echo",
  "processID": 1234,
  "apis": [
    {
      "serviceIDs": ["service1"],
      "serverPort": 50051,
      "protocol": "grpc"
    }
  ]
}
```

**Response:** 200 OK
```json
{
  "success": true
}
```

#### GET /api/v1/discover/:moduleID
Discover module APIs.

**Response:** 200 OK
```json
{
  "moduleID": "echo",
  "apis": [
    {
      "serviceIDs": ["service1"],
      "serverPort": 50051,
      "protocol": "grpc"
    }
  ]
}
```

**Response:** 404 Not Found
```json
{
  "success": false,
  "error": "module not found"
}
```

#### DELETE /api/v1/unpublish/:moduleID
Unpublish module (optional: with processID query parameter).

**Response:** 200 OK
```json
{
  "success": true
}
```

#### GET /api/v1/health
Health check.

**Response:** 200 OK
```json
{
  "status": "healthy",
  "registeredModules": 5,
  "totalAPIs": 12,
  "uptime": "2h30m15s"
}
```

## Features

### Automatic Stale Entry Cleanup

The registry automatically removes stale entries every minute. An entry is considered stale if:
- Last seen timestamp is older than 10 minutes
- No discovery requests have been made (discovery acts as heartbeat)

### Idempotent Operations

- Publishing the same module multiple times updates the registration
- Unpublishing a non-existent module is not an error
- Discovery updates the last-seen timestamp (acts as heartbeat)

### Thread-Safe

All registry operations are thread-safe and can be called concurrently from multiple goroutines.

## Testing

### Using curl (TCP transport)

```bash
# Health check
curl http://127.0.0.1:17951/api/v1/health

# Publish APIs
curl -X POST http://127.0.0.1:17951/api/v1/publish \
  -H "Content-Type: application/json" \
  -d '{
    "moduleID": "test",
    "processID": 1234,
    "apis": [{
      "serviceIDs": ["service1"],
      "serverPort": 50051,
      "protocol": "grpc"
    }]
  }'

# Discover APIs
curl http://127.0.0.1:17951/api/v1/discover/test

# Unpublish
curl -X DELETE http://127.0.0.1:17951/api/v1/unpublish/test
```

### Using curl with Unix socket

```bash
# Health check
curl --unix-socket /tmp/hsu-registry.sock http://localhost/api/v1/health

# Other operations work the same way
```

## Performance

- **Publish**: < 10ms
- **Discover**: < 1ms (in-memory lookup)
- **Memory**: < 10MB for 1000 modules
- **Startup**: < 100ms

## Security Considerations

### Development (TCP)
- Use localhost binding only (127.0.0.1)
- No authentication required (trusted environment)

### Production (Unix Domain Sockets)
- Set restrictive file permissions (0600 = owner only)
- Socket file location should be protected directory (/var/run/hsu)
- Consider process ID verification for additional security

## Troubleshooting

### Registry won't start
- Check if port/socket is already in use
- Verify directory permissions (for Unix sockets)
- Check firewall settings (for TCP)

### Module not found
- Verify module has published APIs
- Check if registration is stale (> 5 minutes since last seen)
- Use health endpoint to see registered modules

### Permission denied (Unix socket)
- Check socket file permissions
- Verify user has access to socket directory
- Try using /tmp for development

## Future Enhancements

- [ ] Named pipe support for Windows
- [ ] Persistent storage (SQLite backend)
- [ ] Watch API for change notifications
- [ ] Health checking of registered services
- [ ] Registry replication for high availability
- [ ] mTLS support for network transport
- [ ] Rate limiting and quotas

