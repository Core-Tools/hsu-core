# Module Development Guide

## Overview

This guide shows module authors how to create location-transparent services that can be deployed either as in-process modules or out-of-process workers with the same code.

## The WorkerModule Interface

Module authors implement **one unified interface** that is used throughout the entire system:

```go
package worker

// Module authors implement this interface
// This SAME interface is used by:
// - Module authors (to implement their modules)
// - ModuleWorkerController (to manage modules)
// - ModuleRuntime (to load/unload modules)  
// - Master (to register and control modules)
type WorkerModule interface {
    // Lifecycle methods
    Initialize(ctx context.Context, config map[string]interface{}) error
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    
    // Health and metadata
    IsHealthy() bool
    GetInfo() ModuleInfo
}

type ModuleInfo struct {
    Name        string
    Version     string
    Description string
    Author      string
    Endpoints   []EndpointInfo  // What services this module exposes
}

type EndpointInfo struct {
    Name     string
    Type     string  // "grpc", "http", "function"
    Address  string  // ":8080", "unix:/tmp/socket", or function name
}
```

## Complete Example Implementation

```go
package dataprocessor

import (
    "context"
    "net"
    "google.golang.org/grpc"
    "github.com/core-tools/hsu-master/pkg/worker"
)

type DataProcessor struct {
    config    Config
    server    *grpc.Server
    running   bool
}

func (dp *DataProcessor) Initialize(ctx context.Context, config map[string]interface{}) error {
    dp.config = parseConfig(config)
    dp.server = grpc.NewServer()
    // Register gRPC services
    RegisterDataProcessorService(dp.server, dp)
    return nil
}

func (dp *DataProcessor) Start(ctx context.Context) error {
    listener, err := net.Listen("tcp", dp.config.Port)
    if err != nil {
        return err
    }
    
    go dp.server.Serve(listener)
    dp.running = true
    return nil
}

func (dp *DataProcessor) Stop(ctx context.Context) error {
    dp.server.GracefulStop()
    dp.running = false
    return nil
}

func (dp *DataProcessor) IsHealthy() bool {
    return dp.running
}

func (dp *DataProcessor) GetInfo() worker.ModuleInfo {
    return worker.ModuleInfo{
        Name:        "data-processor",
        Version:     "1.2.3", 
        Description: "High-performance data processing module",
        Author:      "Data Team",
        Endpoints: []worker.EndpointInfo{
            {
                Name:    "grpc-api",
                Type:    "grpc",
                Address: dp.config.Port,
            },
        },
    }
}

// Factory function for module creation
func NewDataProcessor() worker.WorkerModule {
    return &DataProcessor{}
}
```

## Configuration Handling

Your module receives configuration as `map[string]interface{}`. Create a helper to parse it:

```go
type Config struct {
    Port      string `yaml:"port"`
    BatchSize int    `yaml:"batch_size"`
    Timeout   string `yaml:"timeout"`
    Mode      string `yaml:"mode"`
}

func parseConfig(configMap map[string]interface{}) Config {
    config := Config{
        Port:      ":8080",  // defaults
        BatchSize: 100,
        Timeout:   "30s",
        Mode:      "production",
    }
    
    if port, ok := configMap["port"].(string); ok {
        config.Port = port
    }
    if batchSize, ok := configMap["batch_size"].(int); ok {
        config.BatchSize = batchSize
    }
    // ... handle other fields
    
    return config
}
```

## Module Compilation Models

### Approach A: Native Go Modules (Recommended)

**Modules compiled directly into the application binary**

#### Module Project Structure:
```
my-data-processor/
├── go.mod                 # Standard Go module
├── module.go              # WorkerModule implementation
├── processor.go           # Business logic
├── config.go              # Configuration handling
├── proto/                 # Protocol definitions
│   └── dataprocessor.proto
└── cmd/
    └── standalone/        # For process deployment
        └── main.go
```

#### Module as Package:
```go
// module.go
package dataprocessor

// Implements worker.WorkerModule interface
func NewDataProcessor() worker.WorkerModule {
    return &DataProcessor{}
}
```

#### Standalone Process (with Side-Instance Master):
```go
// cmd/standalone/main.go
package main

import (
    "context"
    "log"
    "my-company/data-processor"
    "github.com/core-tools/hsu-master/pkg/master"
)

func main() {
    // Side-instance Master for this process
    sideMaster := master.NewSideMaster(master.SideConfig{
        MainMasterAddress: "localhost:9999",  // Connect to main Master
        ProcessID:         "data-processor-1",
    })
    
    // Create module
    processor := dataprocessor.NewDataProcessor()
    
    // Side Master provides EndpointRegistrar via context
    ctx := context.WithValue(context.Background(), "endpoint_registrar", sideMaster)
    
    // Initialize and start with side Master support
    err := processor.Initialize(ctx, loadConfig())
    if err != nil {
        log.Fatal(err)
    }
    
    err = processor.Start(ctx)  // Module registers handlers with side Master
    if err != nil {
        log.Fatal(err)
    }
    
    // Side Master communicates endpoint info to main Master
    sideMaster.PublishEndpoints()
    
    // Keep running
    select {}
}
```

### Approach B: Go Plugins (Advanced)

**For runtime loading scenarios**

```go
// plugin.go - separate from main module
// +build plugin

package main

import "my-company/data-processor"

// Exported symbol for plugin loading
var NewModule = dataprocessor.NewDataProcessor
```

Build as plugin:
```bash
go build -buildmode=plugin -o data-processor.so plugin.go
```

## Location-Transparent Development

### Same Protocol, Different Deployment

```go
// Same gRPC service works for both deployments
type DataProcessor struct {
    server *grpc.Server
}

// Process Worker: Runs as separate process
func main() {
    processor := &DataProcessor{}
    processor.server = grpc.NewServer()
    
    // Listen on external port for process deployment
    listener, _ := net.Listen("tcp", ":8080")
    processor.server.Serve(listener)
}

// Module Worker: Runs in-proc in HSU Master  
func (dp *DataProcessor) Start(ctx context.Context) error {
    dp.server = grpc.NewServer()
    
    // Same gRPC server, but on different listener for module deployment
    // Could be localhost, unix socket, or in-memory
    listener, err := net.Listen("tcp", dp.config.Port)
    if err != nil {
        return err
    }
    
    go dp.server.Serve(listener)  // Same server code!
    return nil
}
```

### Context Handling

Always respect the context parameter:

```go
func (dp *DataProcessor) Start(ctx context.Context) error {
    // Use context for cancellation
    go func() {
        <-ctx.Done()
        dp.server.GracefulStop()  // Graceful shutdown when context cancelled
    }()
    
    listener, err := net.Listen("tcp", dp.config.Port)
    if err != nil {
        return err
    }
    
    go dp.server.Serve(listener)
    return nil
}
```

## Best Practices

### 1. Resource Management

```go
func (dp *DataProcessor) Stop(ctx context.Context) error {
    // Graceful shutdown with timeout
    done := make(chan struct{})
    go func() {
        dp.server.GracefulStop()
        close(done)
    }()
    
    select {
    case <-done:
        return nil
    case <-time.After(30 * time.Second):
        dp.server.Stop()  // Force stop
        return nil
    }
}
```

### 2. Health Checking

```go
func (dp *DataProcessor) IsHealthy() bool {
    // Check multiple health indicators
    return dp.running && 
           dp.server != nil && 
           dp.checkDependencies() &&
           dp.checkResourceUsage()
}

func (dp *DataProcessor) checkDependencies() bool {
    // Check database connectivity, external APIs, etc.
    return true
}
```

### 3. Error Handling

```go
func (dp *DataProcessor) Initialize(ctx context.Context, config map[string]interface{}) error {
    // Validate configuration early
    dp.config = parseConfig(config)
    if err := dp.config.Validate(); err != nil {
        return fmt.Errorf("invalid configuration: %w", err)
    }
    
    // Initialize dependencies
    if err := dp.initializeDependencies(); err != nil {
        return fmt.Errorf("failed to initialize dependencies: %w", err)
    }
    
    return nil
}
```

### 4. Logging Integration

```go
import "github.com/core-tools/hsu-master/pkg/logging"

type DataProcessor struct {
    logger logging.Logger
}

func (dp *DataProcessor) Initialize(ctx context.Context, config map[string]interface{}) error {
    // Get logger from context or create one
    dp.logger = logging.FromContext(ctx)
    if dp.logger == nil {
        dp.logger = logging.NewLogger("data-processor")
    }
    
    dp.logger.Infof("Initializing data processor with config: %+v", config)
    return nil
}
```

## Testing Modules

### Unit Testing

```go
func TestDataProcessor(t *testing.T) {
    processor := NewDataProcessor().(*DataProcessor)
    
    config := map[string]interface{}{
        "port": ":9999",
        "batch_size": 10,
    }
    
    ctx := context.Background()
    
    // Test initialization
    err := processor.Initialize(ctx, config)
    assert.NoError(t, err)
    
    // Test startup
    err = processor.Start(ctx)
    assert.NoError(t, err)
    assert.True(t, processor.IsHealthy())
    
    // Test shutdown
    err = processor.Stop(ctx)
    assert.NoError(t, err)
}
```

### Integration Testing

```go
func TestDataProcessorIntegration(t *testing.T) {
    // Test as module
    testAsModule(t)
    
    // Test as process (if applicable)
    testAsProcess(t)
}

func testAsModule(t *testing.T) {
    master := master.NewMaster(master.Config{})
    master.RegisterNativeModuleFactory("data-processor", NewDataProcessor)
    
    config := WorkerConfig{
        ID:         "test-processor",
        Type:       WorkerTypeModule,
        ModuleType: ModuleTypeNative,
        ModuleName: "data-processor",
        Config: map[string]interface{}{
            "port": ":9999",
        },
    }
    
    err := master.AddWorker(config)
    assert.NoError(t, err)
    
    err = master.StartWorker(context.Background(), "test-processor")
    assert.NoError(t, err)
    
    // Test service functionality
    // ...
}
```

## Common Patterns

### HTTP Service Module

```go
type HTTPService struct {
    server *http.Server
    mux    *http.ServeMux
}

func (hs *HTTPService) Initialize(ctx context.Context, config map[string]interface{}) error {
    hs.mux = http.NewServeMux()
    hs.mux.HandleFunc("/health", hs.healthHandler)
    hs.mux.HandleFunc("/api/process", hs.processHandler)
    
    port := config["port"].(string)
    hs.server = &http.Server{
        Addr:    port,
        Handler: hs.mux,
    }
    return nil
}

func (hs *HTTPService) Start(ctx context.Context) error {
    go func() {
        if err := hs.server.ListenAndServe(); err != http.ErrServerClosed {
            // log error
        }
    }()
    return nil
}

func (hs *HTTPService) Stop(ctx context.Context) error {
    return hs.server.Shutdown(ctx)
}
```

### Background Worker Module

```go
type BackgroundWorker struct {
    done   chan struct{}
    cancel context.CancelFunc
}

func (bw *BackgroundWorker) Start(ctx context.Context) error {
    workerCtx, cancel := context.WithCancel(ctx)
    bw.cancel = cancel
    bw.done = make(chan struct{})
    
    go bw.runWorker(workerCtx)
    return nil
}

func (bw *BackgroundWorker) runWorker(ctx context.Context) {
    defer close(bw.done)
    
    ticker := time.NewTicker(time.Minute)
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            bw.doWork()
        }
    }
}

func (bw *BackgroundWorker) Stop(ctx context.Context) error {
    if bw.cancel != nil {
        bw.cancel()
    }
    
    select {
    case <-bw.done:
        return nil
    case <-time.After(30 * time.Second):
        return errors.New("worker shutdown timeout")
    }
}
```

## Deployment Considerations

### Development vs Production

```go
func (dp *DataProcessor) Initialize(ctx context.Context, config map[string]interface{}) error {
    dp.config = parseConfig(config)
    
    // Adjust behavior based on deployment
    if dp.config.Mode == "development" {
        // Enable debug logging, relaxed timeouts, etc.
        dp.setupDevelopmentMode()
    } else {
        // Production settings
        dp.setupProductionMode()
    }
    
    return nil
}
```

### Resource Usage

Since in-process modules share resources with the Master:

```go
func (dp *DataProcessor) Initialize(ctx context.Context, config map[string]interface{}) error {
    // Be conservative with resource usage for module deployment
    maxWorkers := config["max_workers"].(int)
    if maxWorkers == 0 {
        // Default based on deployment type
        maxWorkers = runtime.NumCPU() / 2  // Conservative for modules
    }
    
    dp.workerPool = workerpool.New(maxWorkers)
    return nil
}
```

## Migration Guide

### From Standalone Service to Module

1. **Extract service logic** into a struct that implements `WorkerModule`
2. **Move main() logic** into `Start()` method
3. **Add configuration parsing** for `Initialize()`
4. **Add health checking** for `IsHealthy()`
5. **Add graceful shutdown** for `Stop()`
6. **Create factory function**

### Example Migration

**Before (standalone service):**
```go
func main() {
    config := loadConfig()
    server := grpc.NewServer()
    RegisterMyService(server, NewMyService(config))
    
    listener, _ := net.Listen("tcp", config.Port)
    server.Serve(listener)
}
```

**After (module):**
```go
type MyServiceModule struct {
    config Config
    server *grpc.Server
}

func (m *MyServiceModule) Initialize(ctx context.Context, configMap map[string]interface{}) error {
    m.config = parseConfig(configMap)
    m.server = grpc.NewServer()
    RegisterMyService(m.server, NewMyService(m.config))
    return nil
}

func (m *MyServiceModule) Start(ctx context.Context) error {
    listener, err := net.Listen("tcp", m.config.Port)
    if err != nil {
        return err
    }
    go m.server.Serve(listener)
    return nil
}

// ... implement other methods

func NewMyServiceModule() worker.WorkerModule {
    return &MyServiceModule{}
}
```

## Next Steps

- See [Client Experience](client-experience.md) for location-transparent service communication
- See [Configuration and Usage](configuration-and-usage.md) for how applications use your modules
- See [Developer Experience](developer-experience.md) for benefits and best practices
- See [Diagrams](diagrams.md) for visual architecture overview