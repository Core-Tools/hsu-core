# Master Integration

## Overview

This document details the enhanced Master interface and runtime components that enable unified worker management for both process workers and in-process modules.

## Core Worker Interface

```go
package worker

// Same interface for ALL worker types - in-proc or out-of-proc
type Worker interface {
    // Identity and capabilities
    GetID() string
    GetType() WorkerType
    GetCapabilities() WorkerCapabilities
    
    // Lifecycle (HSU Master managed)
    Initialize(ctx context.Context, config WorkerConfig) error
    GetHealth() HealthStatus
    
    // Communication endpoint (location-agnostic)
    GetEndpoint() WorkerEndpoint  // Could be URL, function reference, etc.
}

// Deployment type - simple binary choice
type WorkerType string
const (
    WorkerTypeProcess WorkerType = "process"  // Current workers (managed/integrated/unmanaged)
    WorkerTypeModule  WorkerType = "module"   // NEW: In-proc modules (always integrated)
)

// Location-agnostic endpoint
type WorkerEndpoint struct {
    Type     EndpointType  // "url", "function", "ipc"
    Address  string        // URL, function name, IPC path
    Metadata map[string]interface{}
}

type EndpointType string
const (
    EndpointTypeURL      EndpointType = "url"      // HTTP/gRPC endpoint
    EndpointTypeFunction EndpointType = "function" // Direct function call
    EndpointTypeIPC      EndpointType = "ipc"      // Unix socket, named pipe
)
```

## Unified Worker Controller

The `WorkerController` abstraction allows the Master to manage all worker types uniformly:

```go
// Unified interface for lifecycle management
type WorkerController interface {
    Start(ctx context.Context) error
    Stop(ctx context.Context) error  
    Restart(ctx context.Context, force bool) error
    GetState() WorkerState           // Same states for all
    GetDiagnostics() WorkerDiagnostics
}

// Process Worker Controller (existing logic - reused)
type ProcessWorkerController struct {
    processControl processcontrol.ProcessControl  // Reuse existing infrastructure!
}

// Module Worker Controller (NEW - simplified)
type ModuleWorkerController struct {
    module     worker.WorkerModule  // Uses same interface as module authors implement
    goroutine  *GoroutineManager
    state      WorkerState
    startTime  time.Time
    lastError  error
}

func (m *ModuleWorkerController) Start(ctx context.Context) error {
    m.state = WorkerStateStarting
    
    // Start module in isolated goroutine
    err := m.goroutine.StartModule(ctx, m.module)
    if err != nil {
        m.state = WorkerStateFailedStart
        m.lastError = err
        return err
    }
    
    m.state = WorkerStateRunning
    m.startTime = time.Now()
    return nil
}

func (m *ModuleWorkerController) GetDiagnostics() WorkerDiagnostics {
    return WorkerDiagnostics{
        State:           m.state,
        Type:            WorkerTypeModule,
        LastError:       m.lastError,
        StartTime:       &m.startTime,
        ResourceUsage:   nil,  // Not supported (like unmanaged workers)
        ProcessID:       0,    // N/A for modules
        ExecutablePath:  "in-proc-module",
        ExecutableExists: true,
    }
}
```

## Enhanced Master Interface

The Master is enhanced to support both process and module workers with a unified API:

```go
// Enhanced Master - treats all workers uniformly
type Master struct {
    // Unified worker storage
    workers map[string]WorkerEntry  // Same for process + module workers
    
    // Existing infrastructure (reused)
    packageManager packagemanager.WorkerPackageManager
    logger         logging.Logger
    
    // NEW: Native module support
    nativeModuleFactories map[string]ModuleFactory
    pluginModuleLoader    PluginLoader
    moduleInstances       map[string]worker.WorkerModule
    moduleRuntime         ModuleRuntime
}

type WorkerEntry struct {
    Worker      Worker
    Type        WorkerType        // "process" or "module"
    Controller  WorkerController  // Unified lifecycle control
    Config      WorkerConfig
}

type ModuleFactory func() worker.WorkerModule

type ModuleType string
const (
    ModuleTypeNative ModuleType = "native"  // Compiled into binary
    ModuleTypePlugin ModuleType = "plugin"  // .so/.dll files
)

// Enhanced WorkerConfig
type WorkerConfig struct {
    ID     string
    Type   WorkerType  // "process" or "module"
    
    // For process workers
    Package        string
    ExecutablePath string
    
    // For module workers
    ModuleType ModuleType  // "native" or "plugin"
    ModuleName string      // Factory name or plugin path
    Config     map[string]interface{}
}
```

## Master API Methods

### Native Module Registration

```go
// Native module registration
func (m *Master) RegisterNativeModuleFactory(name string, factory ModuleFactory) {
    if m.nativeModuleFactories == nil {
        m.nativeModuleFactories = make(map[string]ModuleFactory)
    }
    m.nativeModuleFactories[name] = factory
    m.logger.Infof("Registered native module factory: %s", name)
}
```

### Unified Worker Management

```go
// Enhanced Master methods (unified interface)
func (m *Master) AddWorker(config WorkerConfig) error
func (m *Master) GetWorkerStateWithDiagnostics(id string) (WorkerStateWithDiagnostics, error)
func (m *Master) GetAllWorkerStatesWithDiagnostics() map[string]WorkerStateWithDiagnostics
func (m *Master) StartWorker(ctx context.Context, id string) error
func (m *Master) StopWorker(ctx context.Context, id string) error

// Same API regardless of worker type!
```

### Enhanced AddWorker Implementation

```go
// Enhanced AddWorker implementation
func (m *Master) AddWorker(config WorkerConfig) error {
    // Validate worker ID
    if err := ValidateWorkerID(config.ID); err != nil {
        return errors.NewValidationError("invalid worker ID", err).WithContext("worker_id", config.ID)
    }
    
    // Check if worker already exists
    if _, exists := m.workers[config.ID]; exists {
        return errors.NewConflictError("worker already exists", nil).WithContext("worker_id", config.ID)
    }
    
    switch config.Type {
    case WorkerTypeProcess:
        return m.addProcessWorker(config)
    case WorkerTypeModule:
        return m.addModuleWorker(config)
    default:
        return fmt.Errorf("unsupported worker type: %s", config.Type)
    }
}

func (m *Master) addModuleWorker(config WorkerConfig) error {
    var module worker.WorkerModule
    var err error
    
    switch config.ModuleType {
    case ModuleTypeNative:
        // Create from registered factory
        factory, exists := m.nativeModuleFactories[config.ModuleName]
        if !exists {
            return fmt.Errorf("native module factory not found: %s", config.ModuleName)
        }
        module = factory()
        
    case ModuleTypePlugin:
        // Load from plugin file
        module, err = m.pluginModuleLoader.LoadModule(config.ModuleName)
        if err != nil {
            return err
        }
        
    default:
        return fmt.Errorf("unsupported module type: %s", config.ModuleType)
    }
    
    // Initialize module
    err = module.Initialize(context.Background(), config.Config)
    if err != nil {
        return fmt.Errorf("failed to initialize module: %w", err)
    }
    
    // Create controller
    controller := NewModuleWorkerController(module, m.logger)
    
    // Register worker
    m.workers[config.ID] = WorkerEntry{
        Worker:     module,
        Type:       WorkerTypeModule,
        Controller: controller,
        Config:     config,
    }
    
    m.logger.Infof("Added module worker: %s (type: %s, module: %s)", 
        config.ID, config.ModuleType, config.ModuleName)
    
    return nil
}
```

## Module Runtime Components

### Module Runtime Interface

```go
// NEW: Module runtime for in-proc workers
type ModuleRuntime interface {
    LoadModule(path string, entryPoint string) (worker.WorkerModule, error)
    UnloadModule(module worker.WorkerModule) error
    ListLoadedModules() []worker.ModuleInfo
}

// Note: WorkerModule interface is defined in the worker package (see Module Development section)
// Module authors implement worker.WorkerModule interface - same interface used throughout system
```

### Goroutine Management with Isolation

```go
type GoroutineManager struct {
    modules   map[string]*ModuleExecution
    mutex     sync.RWMutex
    logger    logging.Logger
}

type ModuleExecution struct {
    module    worker.WorkerModule
    cancel    context.CancelFunc
    done      chan error
    started   time.Time
}

func NewGoroutineManager(logger logging.Logger) *GoroutineManager {
    return &GoroutineManager{
        modules: make(map[string]*ModuleExecution),
        logger:  logger,
    }
}

func (gm *GoroutineManager) StartModule(ctx context.Context, module worker.WorkerModule) error {
    moduleCtx, cancel := context.WithCancel(ctx)
    done := make(chan error, 1)
    
    moduleInfo := module.GetInfo()
    gm.logger.Infof("Starting module: %s", moduleInfo.Name)
    
    // Start module in isolated goroutine with panic recovery
    go func() {
        defer func() {
            if r := recover(); r != nil {
                gm.logger.Errorf("Module panic: %s - %v", moduleInfo.Name, r)
                done <- fmt.Errorf("module panic: %v", r)
            }
        }()
        
        err := module.Start(moduleCtx)
        done <- err
    }()
    
    // Wait for startup or timeout
    select {
    case err := <-done:
        if err != nil {
            cancel()
            gm.logger.Errorf("Module start failed: %s - %v", moduleInfo.Name, err)
            return err
        }
        
        gm.mutex.Lock()
        gm.modules[moduleInfo.Name] = &ModuleExecution{
            module:  module,
            cancel:  cancel,
            done:    done,
            started: time.Now(),
        }
        gm.mutex.Unlock()
        
        gm.logger.Infof("Module started successfully: %s", moduleInfo.Name)
        return nil
        
    case <-time.After(30 * time.Second):
        cancel()
        gm.logger.Errorf("Module start timeout: %s", moduleInfo.Name)
        return errors.New("module startup timeout")
    }
}

func (gm *GoroutineManager) StopModule(moduleID string) error {
    gm.mutex.Lock()
    execution, exists := gm.modules[moduleID]
    if !exists {
        gm.mutex.Unlock()
        return errors.New("module not found")
    }
    delete(gm.modules, moduleID)
    gm.mutex.Unlock()
    
    gm.logger.Infof("Stopping module: %s", moduleID)
    
    // Cancel module context
    execution.cancel()
    
    // Wait for graceful shutdown
    select {
    case <-execution.done:
        gm.logger.Infof("Module stopped gracefully: %s", moduleID)
        return nil
    case <-time.After(30 * time.Second):
        gm.logger.Warnf("Module shutdown timeout: %s", moduleID)
        return errors.New("module shutdown timeout")
    }
}

func (gm *GoroutineManager) GetRunningModules() []string {
    gm.mutex.RLock()
    defer gm.mutex.RUnlock()
    
    var moduleIDs []string
    for id := range gm.modules {
        moduleIDs = append(moduleIDs, id)
    }
    return moduleIDs
}
```

### ModuleWorkerController Implementation

```go
func NewModuleWorkerController(module worker.WorkerModule, logger logging.Logger) *ModuleWorkerController {
    return &ModuleWorkerController{
        module:    module,
        goroutine: NewGoroutineManager(logger),
        state:     WorkerStateIdle,
        logger:    logger,
    }
}

func (m *ModuleWorkerController) Start(ctx context.Context) error {
    m.mutex.Lock()
    defer m.mutex.Unlock()
    
    if m.state != WorkerStateIdle {
        return fmt.Errorf("cannot start module in state: %s", m.state)
    }
    
    m.state = WorkerStateStarting
    
    // Start module in isolated goroutine
    err := m.goroutine.StartModule(ctx, m.module)
    if err != nil {
        m.state = WorkerStateFailedStart
        m.lastError = err
        return err
    }
    
    m.state = WorkerStateRunning
    m.startTime = time.Now()
    m.lastError = nil
    return nil
}

func (m *ModuleWorkerController) Stop(ctx context.Context) error {
    m.mutex.Lock()
    defer m.mutex.Unlock()
    
    if m.state != WorkerStateRunning {
        return fmt.Errorf("cannot stop module in state: %s", m.state)
    }
    
    m.state = WorkerStateStopping
    
    moduleInfo := m.module.GetInfo()
    err := m.goroutine.StopModule(moduleInfo.Name)
    if err != nil {
        m.state = WorkerStateRunning // Revert state on failure
        m.lastError = err
        return err
    }
    
    m.state = WorkerStateIdle
    return nil
}

func (m *ModuleWorkerController) Restart(ctx context.Context, force bool) error {
    // Stop then start
    if err := m.Stop(ctx); err != nil && !force {
        return fmt.Errorf("failed to stop module for restart: %w", err)
    }
    
    return m.Start(ctx)
}

func (m *ModuleWorkerController) GetState() WorkerState {
    m.mutex.RLock()
    defer m.mutex.RUnlock()
    return m.state
}

func (m *ModuleWorkerController) GetDiagnostics() WorkerDiagnostics {
    m.mutex.RLock()
    defer m.mutex.RUnlock()
    
    moduleInfo := m.module.GetInfo()
    
    return WorkerDiagnostics{
        State:           m.state,
        Type:            WorkerTypeModule,
        LastError:       m.lastError,
        StartTime:       &m.startTime,
        ResourceUsage:   nil,  // Not supported (like unmanaged workers)
        ProcessID:       0,    // N/A for modules
        ExecutablePath:  fmt.Sprintf("in-proc-module:%s", moduleInfo.Name),
        ExecutableExists: true,
        HealthStatus:    m.getHealthStatus(),
    }
}

func (m *ModuleWorkerController) getHealthStatus() HealthStatus {
    if m.state != WorkerStateRunning {
        return HealthStatusUnhealthy
    }
    
    if m.module.IsHealthy() {
        return HealthStatusHealthy
    }
    
    return HealthStatusUnhealthy
}
```

## Enhanced Package Management

### Extended Package Types

```go
// Enhanced package info to support modules
type PackageInfo struct {
    WorkerType    string
    Version       string
    InstallPath   string
    InstallDate   time.Time
    
    // NEW: Deployment support
    SupportedTypes  []WorkerType  // ["process", "module"] or just ["process"]
    
    // Type-specific paths
    ExecutablePath  string        // For process workers
    ModulePath      string        // For module workers (.so, .dll, Go plugin)
    EntryPoint      string        // Function/symbol name for modules
    
    Dependencies    []Dependency
    Checksum        string
}

// Enhanced install request
type InstallRequest struct {
    WorkerType      string
    Version         string
    Repository      string
    
    // NEW: Target deployment type
    TargetType      WorkerType    // "process", "module", or "both"
    
    InstallTimeout  time.Duration
    AllowDowngrade  bool
    Force           bool
}
```

### Installation Strategy

```go
// Package manager handles both types
func (pm *PackageManager) InstallPackage(ctx context.Context, req InstallRequest) error {
    packageInfo := pm.getPackageInfo(req.WorkerType, req.Version)
    
    switch req.TargetType {
    case WorkerTypeProcess:
        return pm.installExecutable(ctx, packageInfo)
    case WorkerTypeModule:
        return pm.installModule(ctx, packageInfo)
    default:
        return errors.New("unsupported target type")
    }
}

func (pm *PackageManager) installModule(ctx context.Context, pkg PackageInfo) error {
    // Download and install module (shared library, Go plugin, etc.)
    // Validate entry points
    // Register with module runtime
    return pm.moduleRuntime.LoadModule(pkg.ModulePath, pkg.EntryPoint)
}
```

## Error Handling and Diagnostics

### Unified Error Reporting

```go
type WorkerDiagnostics struct {
    State            WorkerState
    Type             WorkerType    // "process" or "module"
    LastError        error
    StartTime        *time.Time
    ResourceUsage    *ResourceUsage // nil for modules
    ProcessID        int           // 0 for modules
    ExecutablePath   string        // "in-proc-module:name" for modules
    ExecutableExists bool
    HealthStatus     HealthStatus
    
    // Type-specific diagnostics
    ProcessDiagnostics *ProcessDiagnostics  // Only for process workers
    ModuleDiagnostics  *ModuleDiagnostics   // Only for module workers
}

type ModuleDiagnostics struct {
    ModuleInfo      worker.ModuleInfo
    GoroutineCount  int
    LoadTime        time.Time
    LastHealthCheck time.Time
}
```

### Health Monitoring Integration

```go
func (m *Master) GetWorkerHealth(id string) (HealthStatus, error) {
    entry, exists := m.workers[id]
    if !exists {
        return HealthStatusUnknown, errors.NewNotFoundError("worker not found", nil).WithContext("worker_id", id)
    }
    
    switch entry.Type {
    case WorkerTypeProcess:
        // Use existing process health monitoring
        return m.getProcessWorkerHealth(entry)
    case WorkerTypeModule:
        // Use module health checking
        return m.getModuleWorkerHealth(entry)
    default:
        return HealthStatusUnknown, fmt.Errorf("unknown worker type: %s", entry.Type)
    }
}

func (m *Master) getModuleWorkerHealth(entry WorkerEntry) (HealthStatus, error) {
    module := entry.Worker.(worker.WorkerModule)
    
    if entry.Controller.GetState() != WorkerStateRunning {
        return HealthStatusUnhealthy, nil
    }
    
    if module.IsHealthy() {
        return HealthStatusHealthy, nil
    }
    
    return HealthStatusUnhealthy, nil
}
```

## State Management

The Master maintains the same state machine for all worker types:

```go
type WorkerState string

const (
    WorkerStateIdle         WorkerState = "idle"
    WorkerStateStarting     WorkerState = "starting"
    WorkerStateRunning      WorkerState = "running"
    WorkerStateStopping     WorkerState = "stopping"
    WorkerStateFailedStart  WorkerState = "failed_start"
    WorkerStateTerminating  WorkerState = "terminating"
)

// Same state transitions for all worker types
func (m *Master) transitionWorkerState(id string, newState WorkerState) error {
    entry, exists := m.workers[id]
    if !exists {
        return errors.NewNotFoundError("worker not found", nil).WithContext("worker_id", id)
    }
    
    // Validate state transition (same rules for all worker types)
    if !isValidStateTransition(entry.Controller.GetState(), newState) {
        return fmt.Errorf("invalid state transition: %s -> %s", entry.Controller.GetState(), newState)
    }
    
    m.logger.Infof("Worker state transition: %s %s -> %s", id, entry.Controller.GetState(), newState)
    
    // State is managed by the WorkerController implementation
    return nil
}
```

## Integration with Existing Master Features

### Logging Integration

Modules inherit the Master's logging configuration:

```go
func (m *Master) addModuleWorker(config WorkerConfig) error {
    // ... module creation ...
    
    // Pass logger context to module
    loggerCtx := logging.WithLogger(context.Background(), m.logger.WithField("module", config.ModuleName))
    err = module.Initialize(loggerCtx, config.Config)
    if err != nil {
        return fmt.Errorf("failed to initialize module: %w", err)
    }
    
    // ... rest of setup ...
}
```

### Monitoring Integration

Modules participate in the same monitoring infrastructure:

```go
func (m *Master) collectWorkerMetrics() {
    for id, entry := range m.workers {
        diagnostics := entry.Controller.GetDiagnostics()
        
        // Common metrics for all worker types
        m.metrics.WorkerState.WithLabelValues(id, string(entry.Type)).Set(float64(stateToInt(diagnostics.State)))
        m.metrics.WorkerUptime.WithLabelValues(id, string(entry.Type)).Set(uptimeSeconds(diagnostics.StartTime))
        
        // Type-specific metrics
        switch entry.Type {
        case WorkerTypeProcess:
            if diagnostics.ProcessDiagnostics != nil {
                m.metrics.ProcessMemory.WithLabelValues(id).Set(float64(diagnostics.ProcessDiagnostics.MemoryUsage))
            }
        case WorkerTypeModule:
            if diagnostics.ModuleDiagnostics != nil {
                m.metrics.ModuleGoroutines.WithLabelValues(id).Set(float64(diagnostics.ModuleDiagnostics.GoroutineCount))
            }
        }
    }
}
```

## Next Steps

- See [Configuration and Usage](configuration-and-usage.md) for application integration
- See [Module Development Guide](module-development-guide.md) for module implementation
- See [Implementation Guide](implementation-guide.md) for development phases