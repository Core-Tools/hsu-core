# Configuration and Usage

## Overview

This guide shows application developers how to configure and use the unified worker system to deploy services as either process workers or in-process modules.

## Application Setup

### Basic Application Structure

```
my-application/
├── main.go                    # Application entry point with HSU Master
├── go.mod                     # Dependencies including hsu-master
├── workers.yaml               # Worker configuration
├── modules/                   # Local modules
│   ├── data-processor/
│   │   ├── module.go         # Implements WorkerModule interface
│   │   ├── processor.go      # Business logic
│   │   └── config.go         # Configuration handling
│   └── auth-service/
│       ├── module.go
│       ├── auth.go
│       └── handlers.go
└── vendor/                    # External modules
    └── github.com/company/log-processor/
```

### Main Application Code

```go
// main.go - Application entry point
package main

import (
    "context"
    "log"
    "github.com/core-tools/hsu-master/pkg/master"
    "github.com/core-tools/hsu-master/pkg/worker"
    
    // Import your modules as regular Go packages
    "my-app/modules/data-processor"
    "my-app/modules/auth-service"
    "github.com/company/log-processor"  // External module from dependency
)

func main() {
    // Create HSU Master
    master := master.NewMaster(master.Config{
        LogLevel: "info",
        // other master configuration
    })
    
    // Register native modules (compiled into binary)
    master.RegisterNativeModuleFactory("data-processor", dataprocessor.NewDataProcessor)
    master.RegisterNativeModuleFactory("auth-service", authservice.NewAuthService)
    master.RegisterNativeModuleFactory("log-processor", logprocessor.NewLogProcessor)
    
    // Configure workers from config file
    config := loadWorkerConfig("workers.yaml")
    for _, workerConfig := range config.Workers {
        err := master.AddWorker(workerConfig)
        if err != nil {
            log.Fatal(err)
        }
    }
    
    // Start master
    err := master.Start(context.Background())
    if err != nil {
        log.Fatal(err)
    }
    
    // Keep running
    select {}
}
```

## Worker Configuration

### YAML Configuration Examples

```yaml
# workers.yaml
workers:
  # Out-of-process deployment (existing)
  - id: "data-processor-1"
    type: "process"
    package: "data-processor:1.2.3"
    execution:
      executable_path: "/opt/workers/data-processor"
      args: ["--config", "config.yaml"]
    
  # Native module deployment (RECOMMENDED)
  - id: "data-processor-2"  
    type: "module"
    module_type: "native"
    module_name: "data-processor"  # References registered factory
    config:
      port: ":8080"
      batch_size: 1000
      timeout: "30s"
      
  # Another instance of same native module with different config
  - id: "data-processor-3" 
    type: "module"
    module_type: "native"
    module_name: "data-processor"  # Same module, different config
    config:
      port: ":8081"
      batch_size: 500
      mode: "development"
      
  # Plugin module deployment (advanced)
  - id: "external-processor"
    type: "module"
    module_type: "plugin"
    module_name: "/opt/plugins/external-processor.so"
    config:
      endpoint: "/api/process"
      workers: 4
      
  # Authentication service instances
  - id: "auth-service-public"
    type: "module"
    module_type: "native"
    module_name: "auth-service"
    config:
      port: ":9090"
      audience: "public-api"
      jwt_secret: "${JWT_SECRET}"
      
  - id: "auth-service-internal"
    type: "module" 
    module_type: "native"
    module_name: "auth-service"
    config:
      port: ":9091"
      audience: "internal-api"
      jwt_secret: "${JWT_SECRET}"
```

### Configuration Structure

```go
type WorkerConfig struct {
    ID     string      `yaml:"id"`
    Type   WorkerType  `yaml:"type"`  // "process" or "module"
    
    // For process workers
    Package        string   `yaml:"package,omitempty"`
    ExecutablePath string   `yaml:"executable_path,omitempty"`
    Args           []string `yaml:"args,omitempty"`
    
    // For module workers  
    ModuleType ModuleType              `yaml:"module_type,omitempty"`  // "native" or "plugin"
    ModuleName string                  `yaml:"module_name,omitempty"`  // Factory name or plugin path
    Config     map[string]interface{}  `yaml:"config,omitempty"`
    
    // Common configuration
    HealthCheck *HealthCheckConfig     `yaml:"health_check,omitempty"`
    Resources   *ResourceLimits        `yaml:"resources,omitempty"`    // Only for process workers
}
```

## Runtime Management

### Programmatic Worker Management

```go
// Application startup - register native modules
master := master.NewMaster(master.Config{})
master.RegisterNativeModuleFactory("data-processor", dataprocessor.NewDataProcessor)
master.RegisterNativeModuleFactory("auth-service", authservice.NewAuthService)

// Same API for all worker types!
processConfig := WorkerConfig{
    ID:           "data-processor-1",
    Type:         WorkerTypeProcess,
    Package:      "data-processor:1.2.3",
    ExecutablePath: "/opt/workers/data-processor",
}
err := master.AddWorker(processConfig)

nativeModuleConfig := WorkerConfig{
    ID:         "data-processor-2",
    Type:       WorkerTypeModule,
    ModuleType: ModuleTypeNative,
    ModuleName: "data-processor",  // References registered factory
    Config: map[string]interface{}{
        "port":       ":8080",
        "batch_size": 1000,
    },
}
err := master.AddWorker(nativeModuleConfig)

// Same monitoring for all types
state1 := master.GetWorkerStateWithDiagnostics("data-processor-1")  // Process worker
state2 := master.GetWorkerStateWithDiagnostics("data-processor-2")  // Native module worker

// Same lifecycle operations
err = master.StartWorker(ctx, "data-processor-1")
err = master.StartWorker(ctx, "data-processor-2")  // Same API!

// Multiple instances of same module with different configs
authConfig1 := WorkerConfig{
    ID:         "auth-service-public",
    Type:       WorkerTypeModule,
    ModuleType: ModuleTypeNative,
    ModuleName: "auth-service",
    Config: map[string]interface{}{
        "port":     ":9090",
        "audience": "public-api",
    },
}

authConfig2 := WorkerConfig{
    ID:         "auth-service-internal",
    Type:       WorkerTypeModule,
    ModuleType: ModuleTypeNative,
    ModuleName: "auth-service",  // Same module type
    Config: map[string]interface{}{
        "port":     ":9091",
        "audience": "internal-api",
    },
}

err = master.AddWorker(authConfig1)
err = master.AddWorker(authConfig2)
```

### Dynamic Configuration

```go
// Load and reload configuration
func (app *Application) reloadConfig() error {
    newConfig := loadWorkerConfig("workers.yaml")
    
    // Compare with current configuration
    for _, workerConfig := range newConfig.Workers {
        currentWorker, exists := app.master.GetWorker(workerConfig.ID)
        
        if !exists {
            // Add new worker
            err := app.master.AddWorker(workerConfig)
            if err != nil {
                return fmt.Errorf("failed to add worker %s: %w", workerConfig.ID, err)
            }
        } else if configChanged(currentWorker.Config, workerConfig) {
            // Restart worker with new config (requires restart for modules)
            err := app.master.RemoveWorker(workerConfig.ID)
            if err != nil {
                return fmt.Errorf("failed to remove worker %s: %w", workerConfig.ID, err)
            }
            
            err = app.master.AddWorker(workerConfig)
            if err != nil {
                return fmt.Errorf("failed to re-add worker %s: %w", workerConfig.ID, err)
            }
        }
    }
    
    return nil
}
```

## Package Management

### Enhanced Package Metadata

```yaml
# package.yaml - supports both deployment types
name: "data-processor"
version: "1.2.3"
description: "High-performance data processing service"

# Deployment options
supported_types: ["process", "module"]

# Process deployment
process:
  executable_path: "bin/data-processor"
  args: ["--config", "config.yaml"]
  
# Module deployment  
module:
  module_path: "lib/data-processor.so"
  entry_point: "CreateDataProcessor"
  
dependencies:
  - name: "libprocessor"
    version: ">=2.1.0"
    
health_check:
  endpoint: "/health"
  interval: "30s"
```

### Package Installation

```go
// Install for different deployment types
err := master.InstallWorkerPackage(ctx, InstallRequest{
    WorkerType: "data-processor",
    TargetType: WorkerTypeProcess,  // Install as executable
})

err := master.InstallWorkerPackage(ctx, InstallRequest{
    WorkerType: "data-processor", 
    TargetType: WorkerTypeModule,   // Install as module
})
```

## Usage Patterns

### Environment-Based Configuration

```yaml
# workers-development.yaml
workers:
  - id: "data-processor"
    type: "module"          # Fast startup for development
    module_type: "native"
    module_name: "data-processor"
    config:
      port: ":8080"
      debug: true
      batch_size: 10        # Small batches for testing

# workers-production.yaml  
workers:
  - id: "data-processor"
    type: "process"         # Fault isolation for production
    package: "data-processor:1.2.3"
    execution:
      executable_path: "/opt/workers/data-processor"
    config:
      port: ":8080"
      batch_size: 1000      # Large batches for efficiency
```

Load different configurations:

```go
func main() {
    env := os.Getenv("ENVIRONMENT")
    configFile := fmt.Sprintf("workers-%s.yaml", env)
    
    master := master.NewMaster(master.Config{})
    
    // Register modules (same for all environments)
    master.RegisterNativeModuleFactory("data-processor", dataprocessor.NewDataProcessor)
    
    // Load environment-specific configuration
    config := loadWorkerConfig(configFile)
    for _, workerConfig := range config.Workers {
        err := master.AddWorker(workerConfig)
        if err != nil {
            log.Fatal(err)
        }
    }
    
    err := master.Start(context.Background())
    if err != nil {
        log.Fatal(err)
    }
}
```

### Scaling Patterns

```yaml
# Horizontal scaling with modules
workers:
  # Multiple instances of the same service
  - id: "data-processor-1"
    type: "module"
    module_type: "native"
    module_name: "data-processor"
    config:
      port: ":8080"
      
  - id: "data-processor-2"
    type: "module"
    module_type: "native"
    module_name: "data-processor"
    config:
      port: ":8081"
      
  - id: "data-processor-3"
    type: "module"
    module_type: "native"
    module_name: "data-processor"
    config:
      port: ":8082"
```

### Mixed Deployment

```yaml
# Critical services as processes, others as modules
workers:
  # Critical service - separate process for fault isolation
  - id: "payment-service"
    type: "process"
    package: "payment-service:2.1.0"
    execution:
      executable_path: "/opt/workers/payment-service"
    
  # Supporting services - modules for efficiency
  - id: "user-cache"
    type: "module"
    module_type: "native"
    module_name: "user-cache"
    
  - id: "notification-service"
    type: "module"
    module_type: "native" 
    module_name: "notification-service"
    
  - id: "audit-logger"
    type: "module"
    module_type: "native"
    module_name: "audit-logger"
```

## Monitoring and Operations

### Health Checking

```go
// Check health of all workers (same API for all types)
func (app *Application) healthCheck() error {
    workers := app.master.GetAllWorkerStatesWithDiagnostics()
    
    for id, state := range workers {
        if state.State != WorkerStateRunning {
            return fmt.Errorf("worker %s not running: %s", id, state.State)
        }
        
        if state.ProcessDiagnostics != nil && state.ProcessDiagnostics.LastError != nil {
            return fmt.Errorf("worker %s has error: %v", id, state.ProcessDiagnostics.LastError)
        }
    }
    
    return nil
}
```

### Metrics Collection

```go
// Collect metrics from all worker types
func (app *Application) collectMetrics() {
    workers := app.master.GetAllWorkerStatesWithDiagnostics()
    
    for id, state := range workers {
        // Common metrics
        app.metrics.WorkerUptime.WithLabelValues(id, string(state.Type)).Set(
            time.Since(*state.StartTime).Seconds())
        
        // Type-specific metrics
        switch state.Type {
        case WorkerTypeProcess:
            if state.ProcessDiagnostics != nil {
                app.metrics.ProcessMemory.WithLabelValues(id).Set(
                    float64(state.ProcessDiagnostics.MemoryUsage))
            }
        case WorkerTypeModule:
            // Module-specific metrics
            app.metrics.ModuleInstances.WithLabelValues(id).Inc()
        }
    }
}
```

### Logging Integration

```go
// Configure logging for modules
func (app *Application) configureLogging() {
    logger := logging.NewStructuredLogger(logging.Config{
        Level:  "info",
        Format: "json",
        Fields: map[string]interface{}{
            "app":     "my-application",
            "version": app.version,
        },
    })
    
    master := master.NewMaster(master.Config{
        Logger: logger,
    })
    
    // Modules inherit this logger configuration
    master.RegisterNativeModuleFactory("data-processor", dataprocessor.NewDataProcessor)
}
```

## Error Handling

### Graceful Degradation

```go
func (app *Application) handleWorkerFailure(workerID string, err error) {
    logger := app.logger.WithField("worker", workerID)
    logger.Errorf("Worker failed: %v", err)
    
    // Get worker configuration
    state, diagErr := app.master.GetWorkerStateWithDiagnostics(workerID)
    if diagErr != nil {
        logger.Errorf("Failed to get worker diagnostics: %v", diagErr)
        return
    }
    
    // Handle based on worker type
    switch state.Type {
    case WorkerTypeProcess:
        // Process workers can be restarted
        logger.Infof("Restarting process worker: %s", workerID)
        if restartErr := app.master.RestartWorker(context.Background(), workerID); restartErr != nil {
            logger.Errorf("Failed to restart process worker: %v", restartErr)
        }
        
    case WorkerTypeModule:
        // Module failures may require application restart
        logger.Warnf("Module worker failed - may require application restart: %s", workerID)
        
        // Try restart first
        if restartErr := app.master.RestartWorker(context.Background(), workerID); restartErr != nil {
            logger.Errorf("Failed to restart module worker: %v", restartErr)
            
            // For critical modules, consider application restart
            if app.isCriticalWorker(workerID) {
                logger.Fatalf("Critical module failed - restarting application")
            }
        }
    }
}
```

### Configuration Validation

```go
func validateWorkerConfig(config WorkerConfig) error {
    if config.ID == "" {
        return errors.New("worker ID is required")
    }
    
    switch config.Type {
    case WorkerTypeProcess:
        if config.Package == "" && config.ExecutablePath == "" {
            return errors.New("process workers require package or executable_path")
        }
        
    case WorkerTypeModule:
        if config.ModuleName == "" {
            return errors.New("module workers require module_name")
        }
        
        if config.ModuleType == "" {
            config.ModuleType = ModuleTypeNative  // default
        }
        
        if config.ModuleType == ModuleTypeNative && config.Config == nil {
            config.Config = make(map[string]interface{})  // default empty config
        }
        
    default:
        return fmt.Errorf("unknown worker type: %s", config.Type)
    }
    
    return nil
}
```

## Migration Strategies

### Gradual Migration from Process to Module

```yaml
# Phase 1: Start with process workers
workers:
  - id: "auth-service"
    type: "process"
    package: "auth-service:1.0.0"
    
  - id: "user-service" 
    type: "process"
    package: "user-service:1.0.0"

# Phase 2: Migrate non-critical services to modules
workers:
  - id: "auth-service"
    type: "process"          # Keep critical service as process
    package: "auth-service:1.0.0"
    
  - id: "user-service"
    type: "module"           # Migrate to module
    module_type: "native"
    module_name: "user-service"

# Phase 3: Migrate remaining services based on experience
workers:
  - id: "auth-service"
    type: "module"           # Migrate when confident
    module_type: "native"
    module_name: "auth-service"
    
  - id: "user-service"
    type: "module"
    module_type: "native"
    module_name: "user-service"
```

### A/B Testing Deployment Models

```go
func (app *Application) experimentalDeployment() error {
    // Deploy same service in both modes for comparison
    processConfig := WorkerConfig{
        ID:           "service-process",
        Type:         WorkerTypeProcess,
        Package:      "my-service:1.0.0",
    }
    
    moduleConfig := WorkerConfig{
        ID:         "service-module",
        Type:       WorkerTypeModule,
        ModuleType: ModuleTypeNative,
        ModuleName: "my-service",
    }
    
    // Add both workers
    if err := app.master.AddWorker(processConfig); err != nil {
        return err
    }
    
    if err := app.master.AddWorker(moduleConfig); err != nil {
        return err
    }
    
    // Route traffic to both and compare performance
    app.setupTrafficSplitting("service-process", "service-module")
    
    return nil
}
```

## Best Practices

### 1. Module Selection Criteria

**Use Modules For:**
- Non-critical services that can tolerate shared fate
- High-frequency, low-latency services
- Services with simple resource requirements
- Development and testing environments

**Use Processes For:**
- Critical services requiring fault isolation
- Services with specific resource limits
- Legacy services not designed for module deployment
- Production environments requiring maximum stability

### 2. Configuration Management

```go
// Use environment-specific defaults
func loadWorkerConfig(env string) (*Config, error) {
    config := &Config{}
    
    // Load base configuration
    if err := yaml.Unmarshal(baseConfig, config); err != nil {
        return nil, err
    }
    
    // Override with environment-specific settings
    envConfig := fmt.Sprintf("workers-%s.yaml", env)
    if envData, err := ioutil.ReadFile(envConfig); err == nil {
        if err := yaml.Unmarshal(envData, config); err != nil {
            return nil, err
        }
    }
    
    // Apply environment variable overrides
    config.applyEnvironmentOverrides()
    
    return config, nil
}
```

### 3. Resource Planning

```yaml
# Consider resource requirements when choosing deployment type
workers:
  # Memory-intensive service - separate process for isolation
  - id: "ml-processor"
    type: "process"
    package: "ml-processor:1.0.0"
    resources:
      memory_limit: "2GB"
      
  # Lightweight services - modules for efficiency  
  - id: "cache-warmer"
    type: "module"
    module_type: "native"
    module_name: "cache-warmer"
    
  - id: "health-checker"
    type: "module"
    module_type: "native"
    module_name: "health-checker"
```

## Next Steps

- See [Module Development Guide](module-development-guide.md) for creating modules
- See [Master Integration](master-integration.md) for platform implementation details
- See [Developer Experience](developer-experience.md) for benefits and migration strategies