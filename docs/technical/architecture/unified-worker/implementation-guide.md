# Implementation Guide

## Overview

This guide provides detailed implementation phases, technical considerations, and constraints for building the unified worker architecture.

## Implementation Phases

### Phase 1: Foundation (Weeks 1-2)

#### 1.1 Interface Definitions

**Create Core Interfaces:**

```go
// pkg/worker/interfaces.go
package worker

type WorkerModule interface {
    Initialize(ctx context.Context, config map[string]interface{}) error
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    IsHealthy() bool
    GetInfo() ModuleInfo
}

type WorkerController interface {
    Start(ctx context.Context) error
    Stop(ctx context.Context) error  
    Restart(ctx context.Context, force bool) error
    GetState() WorkerState
    GetDiagnostics() WorkerDiagnostics
}

type ModuleRuntime interface {
    LoadModule(path string, entryPoint string) (WorkerModule, error)
    UnloadModule(module WorkerModule) error
    ListLoadedModules() []ModuleInfo
}
```

**Extend Existing Types:**

```go
// pkg/workers/processcontrol/process_state.go
type WorkerType string
const (
    WorkerTypeProcess WorkerType = "process"
    WorkerTypeModule  WorkerType = "module"
)

type ModuleType string
const (
    ModuleTypeNative ModuleType = "native"
    ModuleTypePlugin ModuleType = "plugin"
)
```

#### 1.2 Basic Module Runtime

**GoroutineManager Implementation:**

```go
// pkg/worker/runtime/goroutine_manager.go
package runtime

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
    panics    int
    lastPanic time.Time
}

func NewGoroutineManager(logger logging.Logger) *GoroutineManager {
    return &GoroutineManager{
        modules: make(map[string]*ModuleExecution),
        logger:  logger,
    }
}
```

**Panic Recovery Implementation:**

```go
func (gm *GoroutineManager) StartModule(ctx context.Context, module worker.WorkerModule) error {
    moduleCtx, cancel := context.WithCancel(ctx)
    done := make(chan error, 1)
    
    moduleInfo := module.GetInfo()
    
    // Start module in isolated goroutine with panic recovery
    go func() {
        defer func() {
            if r := recover(); r != nil {
                gm.handleModulePanic(moduleInfo.Name, r)
                done <- fmt.Errorf("module panic: %v", r)
            }
        }()
        
        err := module.Start(moduleCtx)
        done <- err
    }()
    
    // Wait for startup with timeout
    return gm.waitForStartup(moduleInfo.Name, done, cancel)
}

func (gm *GoroutineManager) handleModulePanic(moduleName string, panicValue interface{}) {
    gm.mutex.Lock()
    defer gm.mutex.Unlock()
    
    if execution, exists := gm.modules[moduleName]; exists {
        execution.panics++
        execution.lastPanic = time.Now()
        
        // Log panic with stack trace
        gm.logger.Errorf("Module panic: %s - %v\nStack: %s", 
            moduleName, panicValue, debug.Stack())
        
        // Implement panic threshold logic
        if execution.panics > 5 {
            gm.logger.Errorf("Module %s exceeded panic threshold, marking unstable", moduleName)
        }
    }
}
```

#### 1.3 Master Integration Points

**Enhanced Master Structure:**

```go
// pkg/master/master.go - additions
type Master struct {
    // Existing fields...
    
    // NEW: Native module support
    nativeModuleFactories map[string]ModuleFactory
    pluginModuleLoader    PluginLoader
    moduleRuntime         ModuleRuntime
    workerControllers     map[string]WorkerController
}

type ModuleFactory func() worker.WorkerModule

func (m *Master) RegisterNativeModuleFactory(name string, factory ModuleFactory) {
    if m.nativeModuleFactories == nil {
        m.nativeModuleFactories = make(map[string]ModuleFactory)
    }
    m.nativeModuleFactories[name] = factory
    m.logger.Infof("Registered native module factory: %s", name)
}
```

**Configuration Parsing:**

```go
// pkg/master/config.go - enhancements
type WorkerConfig struct {
    ID     string      `yaml:"id"`
    Type   WorkerType  `yaml:"type"`
    
    // For process workers
    Package        string   `yaml:"package,omitempty"`
    ExecutablePath string   `yaml:"executable_path,omitempty"`
    Args           []string `yaml:"args,omitempty"`
    
    // For module workers
    ModuleType ModuleType              `yaml:"module_type,omitempty"`
    ModuleName string                  `yaml:"module_name,omitempty"`
    Config     map[string]interface{}  `yaml:"config,omitempty"`
    
    // Common
    HealthCheck *HealthCheckConfig `yaml:"health_check,omitempty"`
    Resources   *ResourceLimits    `yaml:"resources,omitempty"`
}

func (config *WorkerConfig) Validate() error {
    if config.ID == "" {
        return errors.New("worker ID is required")
    }
    
    switch config.Type {
    case WorkerTypeProcess:
        return config.validateProcessConfig()
    case WorkerTypeModule:
        return config.validateModuleConfig()
    default:
        return fmt.Errorf("unknown worker type: %s", config.Type)
    }
}
```

### Phase 2: Core Implementation (Weeks 3-4)

#### 2.1 ModuleWorkerController

**Complete Implementation:**

```go
// pkg/worker/controller/module_controller.go
package controller

type ModuleWorkerController struct {
    module      worker.WorkerModule
    runtime     runtime.ModuleRuntime
    state       WorkerState
    startTime   time.Time
    lastError   error
    mutex       sync.RWMutex
    logger      logging.Logger
    
    // Diagnostics
    healthHistory []HealthCheckResult
    startCount    int
    lastRestart   time.Time
}

func NewModuleWorkerController(module worker.WorkerModule, runtime runtime.ModuleRuntime, logger logging.Logger) *ModuleWorkerController {
    return &ModuleWorkerController{
        module:        module,
        runtime:       runtime,
        state:         WorkerStateIdle,
        logger:        logger.WithField("module", module.GetInfo().Name),
        healthHistory: make([]HealthCheckResult, 0, 10),
    }
}

func (m *ModuleWorkerController) Start(ctx context.Context) error {
    m.mutex.Lock()
    defer m.mutex.Unlock()
    
    if !m.canStartFromState(m.state) {
        return fmt.Errorf("cannot start module in state: %s", m.state)
    }
    
    m.state = WorkerStateStarting
    m.lastError = nil
    
    // Start module via runtime
    err := m.runtime.StartModule(ctx, m.module)
    if err != nil {
        m.state = WorkerStateFailedStart
        m.lastError = err
        m.logger.Errorf("Module start failed: %v", err)
        return err
    }
    
    m.state = WorkerStateRunning
    m.startTime = time.Now()
    m.startCount++
    m.logger.Infof("Module started successfully")
    
    return nil
}

func (m *ModuleWorkerController) Stop(ctx context.Context) error {
    m.mutex.Lock()
    defer m.mutex.Unlock()
    
    if !m.canStopFromState(m.state) {
        return fmt.Errorf("cannot stop module in state: %s", m.state)
    }
    
    m.state = WorkerStateStopping
    
    err := m.runtime.StopModule(m.module.GetInfo().Name)
    if err != nil {
        m.state = WorkerStateRunning // Revert on failure
        m.lastError = err
        m.logger.Errorf("Module stop failed: %v", err)
        return err
    }
    
    m.state = WorkerStateIdle
    m.logger.Infof("Module stopped successfully")
    
    return nil
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
        ResourceUsage:   nil, // Not supported for modules
        ProcessID:       0,   // N/A for modules
        ExecutablePath:  fmt.Sprintf("in-proc-module:%s", moduleInfo.Name),
        ExecutableExists: true,
        HealthStatus:    m.getCurrentHealthStatus(),
        
        // Module-specific diagnostics
        ModuleDiagnostics: &ModuleDiagnostics{
            ModuleInfo:      moduleInfo,
            StartCount:      m.startCount,
            LastRestart:     m.lastRestart,
            HealthHistory:   m.healthHistory,
        },
    }
}
```

#### 2.2 Package Management Extension

**Enhanced Package Info:**

```go
// pkg/packagemanager/types.go - enhancements
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
    
    // NEW: Module metadata
    ModuleMetadata  *ModulePackageMetadata `json:"module_metadata,omitempty"`
}

type ModulePackageMetadata struct {
    RequiredInterfaces []string `json:"required_interfaces"`
    ProvidedServices   []string `json:"provided_services"`
    ResourceHints      *ResourceHints `json:"resource_hints,omitempty"`
}

type ResourceHints struct {
    MinMemoryMB     int `json:"min_memory_mb"`
    MinCPUPercent   int `json:"min_cpu_percent"`
    MaxGoroutines   int `json:"max_goroutines"`
}
```

**Installation Strategy:**

```go
// pkg/packagemanager/installer.go - enhancements
func (pm *PackageManager) installModule(ctx context.Context, pkg PackageInfo) error {
    pm.logger.Infof("Installing module package: %s", pkg.WorkerType)
    
    // Validate module package
    if err := pm.validateModulePackage(pkg); err != nil {
        return fmt.Errorf("module package validation failed: %w", err)
    }
    
    switch pkg.ModuleType {
    case ModuleTypeNative:
        // For native modules, just validate the factory is registered
        return pm.validateNativeModule(pkg)
    case ModuleTypePlugin:
        // Download and install plugin files
        return pm.installPluginModule(ctx, pkg)
    default:
        return fmt.Errorf("unsupported module type: %s", pkg.ModuleType)
    }
}

func (pm *PackageManager) validateNativeModule(pkg PackageInfo) error {
    // Check if factory is registered in the master
    if pm.master.HasNativeModuleFactory(pkg.WorkerType) {
        pm.logger.Infof("Native module factory validated: %s", pkg.WorkerType)
        return nil
    }
    
    return fmt.Errorf("native module factory not found: %s", pkg.WorkerType)
}
```

#### 2.3 Configuration Support

**YAML Configuration Loading:**

```go
// pkg/master/config_loader.go
func LoadWorkerConfig(configPath string) (*WorkerConfigSet, error) {
    data, err := ioutil.ReadFile(configPath)
    if err != nil {
        return nil, fmt.Errorf("failed to read config file: %w", err)
    }
    
    var config WorkerConfigSet
    if err := yaml.Unmarshal(data, &config); err != nil {
        return nil, fmt.Errorf("failed to parse config: %w", err)
    }
    
    // Validate all worker configurations
    for i, workerConfig := range config.Workers {
        if err := workerConfig.Validate(); err != nil {
            return nil, fmt.Errorf("invalid worker config at index %d: %w", i, err)
        }
    }
    
    return &config, nil
}

type WorkerConfigSet struct {
    Workers []WorkerConfig `yaml:"workers"`
    Global  GlobalConfig   `yaml:"global,omitempty"`
}

type GlobalConfig struct {
    ModuleDefaults map[string]interface{} `yaml:"module_defaults,omitempty"`
    ProcessDefaults map[string]interface{} `yaml:"process_defaults,omitempty"`
}
```

### Phase 3: Testing and Validation (Weeks 5-6)

#### 3.1 Comprehensive Testing

**Module Lifecycle Tests:**

```go
// pkg/worker/controller/module_controller_test.go
func TestModuleWorkerController_Lifecycle(t *testing.T) {
    logger := &MockLogger{}
    runtime := &MockModuleRuntime{}
    module := &MockWorkerModule{
        info: worker.ModuleInfo{Name: "test-module"},
    }
    
    controller := NewModuleWorkerController(module, runtime, logger)
    
    ctx := context.Background()
    
    // Test initial state
    assert.Equal(t, WorkerStateIdle, controller.GetState())
    
    // Test start
    err := controller.Start(ctx)
    assert.NoError(t, err)
    assert.Equal(t, WorkerStateRunning, controller.GetState())
    
    // Test diagnostics
    diagnostics := controller.GetDiagnostics()
    assert.Equal(t, WorkerTypeModule, diagnostics.Type)
    assert.NotNil(t, diagnostics.StartTime)
    
    // Test stop
    err = controller.Stop(ctx)
    assert.NoError(t, err)
    assert.Equal(t, WorkerStateIdle, controller.GetState())
}

func TestModuleWorkerController_PanicRecovery(t *testing.T) {
    logger := &MockLogger{}
    runtime := &MockModuleRuntime{}
    module := &PanicWorkerModule{} // Module that panics on start
    
    controller := NewModuleWorkerController(module, runtime, logger)
    
    ctx := context.Background()
    
    // Test panic handling
    err := controller.Start(ctx)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "module panic")
    assert.Equal(t, WorkerStateFailedStart, controller.GetState())
}
```

**Integration Tests:**

```go
// pkg/master/integration_test.go
func TestMaster_ModuleIntegration(t *testing.T) {
    master := NewMaster(Config{
        Logger: &MockLogger{},
    })
    
    // Register test module factory
    master.RegisterNativeModuleFactory("test-module", func() worker.WorkerModule {
        return &TestModule{}
    })
    
    // Add module worker
    config := WorkerConfig{
        ID:         "test-worker",
        Type:       WorkerTypeModule,
        ModuleType: ModuleTypeNative,
        ModuleName: "test-module",
        Config: map[string]interface{}{
            "port": ":9999",
        },
    }
    
    err := master.AddWorker(config)
    assert.NoError(t, err)
    
    // Start worker
    ctx := context.Background()
    err = master.StartWorker(ctx, "test-worker")
    assert.NoError(t, err)
    
    // Check state
    state, err := master.GetWorkerStateWithDiagnostics("test-worker")
    assert.NoError(t, err)
    assert.Equal(t, WorkerStateRunning, state.State)
    assert.Equal(t, WorkerTypeModule, state.Type)
    
    // Stop worker
    err = master.StopWorker(ctx, "test-worker")
    assert.NoError(t, err)
}
```

#### 3.2 Example Implementations

**Sample HTTP Module:**

```go
// examples/http-module/module.go
package httpmodule

import (
    "context"
    "net/http"
    "github.com/core-tools/hsu-master/pkg/worker"
)

type HTTPModule struct {
    server *http.Server
    config Config
}

type Config struct {
    Port string `yaml:"port"`
    Host string `yaml:"host"`
}

func (h *HTTPModule) Initialize(ctx context.Context, configMap map[string]interface{}) error {
    h.config = parseConfig(configMap)
    
    mux := http.NewServeMux()
    mux.HandleFunc("/health", h.healthHandler)
    mux.HandleFunc("/api/hello", h.helloHandler)
    
    h.server = &http.Server{
        Addr:    h.config.Host + h.config.Port,
        Handler: mux,
    }
    
    return nil
}

func (h *HTTPModule) Start(ctx context.Context) error {
    go func() {
        if err := h.server.ListenAndServe(); err != http.ErrServerClosed {
            // Log error but don't panic - let health check handle it
        }
    }()
    return nil
}

func (h *HTTPModule) Stop(ctx context.Context) error {
    return h.server.Shutdown(ctx)
}

func (h *HTTPModule) IsHealthy() bool {
    // Simple health check
    return h.server != nil
}

func (h *HTTPModule) GetInfo() worker.ModuleInfo {
    return worker.ModuleInfo{
        Name:        "http-module",
        Version:     "1.0.0",
        Description: "Example HTTP service module",
        Author:      "HSU Team",
        Endpoints: []worker.EndpointInfo{
            {Name: "http-api", Type: "http", Address: h.config.Port},
        },
    }
}

func NewHTTPModule() worker.WorkerModule {
    return &HTTPModule{}
}
```

#### 3.3 Performance Validation

**Benchmarking Tests:**

```go
// pkg/worker/runtime/benchmark_test.go
func BenchmarkGoroutineManager_StartStop(b *testing.B) {
    logger := &MockLogger{}
    gm := NewGoroutineManager(logger)
    
    b.ResetTimer()
    
    for i := 0; i < b.N; i++ {
        module := &BenchmarkModule{id: fmt.Sprintf("module-%d", i)}
        
        err := gm.StartModule(context.Background(), module)
        if err != nil {
            b.Fatalf("Failed to start module: %v", err)
        }
        
        err = gm.StopModule(module.GetInfo().Name)
        if err != nil {
            b.Fatalf("Failed to stop module: %v", err)
        }
    }
}

func BenchmarkModuleVsProcess_Latency(b *testing.B) {
    // Compare module vs process startup latency
    b.Run("Module", func(b *testing.B) {
        for i := 0; i < b.N; i++ {
            // Time module startup
        }
    })
    
    b.Run("Process", func(b *testing.B) {
        for i := 0; i < b.N; i++ {
            // Time process startup
        }
    })
}
```

### Phase 4: Production Readiness (Weeks 7-8)

#### 4.1 Stability and Performance

**Memory Leak Detection:**

```go
// pkg/worker/runtime/monitoring.go
type MemoryMonitor struct {
    baseline    runtime.MemStats
    checkInterval time.Duration
    logger      logging.Logger
    alerts      chan MemoryAlert
}

type MemoryAlert struct {
    Timestamp     time.Time
    AllocDelta    uint64
    HeapSizeDelta uint64
    GoroutineCount int
}

func (mm *MemoryMonitor) StartMonitoring(ctx context.Context) {
    ticker := time.NewTicker(mm.checkInterval)
    defer ticker.Stop()
    
    runtime.ReadMemStats(&mm.baseline)
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            mm.checkMemoryUsage()
        }
    }
}

func (mm *MemoryMonitor) checkMemoryUsage() {
    var current runtime.MemStats
    runtime.ReadMemStats(&current)
    
    allocDelta := current.Alloc - mm.baseline.Alloc
    heapDelta := current.HeapSys - mm.baseline.HeapSys
    goroutines := runtime.NumGoroutine()
    
    // Alert on significant increases
    if allocDelta > 100*1024*1024 || heapDelta > 100*1024*1024 { // 100MB threshold
        alert := MemoryAlert{
            Timestamp:     time.Now(),
            AllocDelta:    allocDelta,
            HeapSizeDelta: heapDelta,
            GoroutineCount: goroutines,
        }
        
        select {
        case mm.alerts <- alert:
        default:
            mm.logger.Warnf("Memory alert channel full, dropping alert")
        }
    }
}
```

**Goroutine Leak Prevention:**

```go
// pkg/worker/runtime/goroutine_tracker.go
type GoroutineTracker struct {
    baselines    map[string]int
    mutex        sync.RWMutex
    logger       logging.Logger
    alertThreshold int
}

func (gt *GoroutineTracker) TrackModule(moduleName string) {
    gt.mutex.Lock()
    defer gt.mutex.Unlock()
    
    gt.baselines[moduleName] = runtime.NumGoroutine()
    gt.logger.Debugf("Tracking goroutines for module %s: baseline %d", 
        moduleName, gt.baselines[moduleName])
}

func (gt *GoroutineTracker) CheckModule(moduleName string) {
    gt.mutex.RLock()
    baseline, exists := gt.baselines[moduleName]
    gt.mutex.RUnlock()
    
    if !exists {
        return
    }
    
    current := runtime.NumGoroutine()
    delta := current - baseline
    
    if delta > gt.alertThreshold {
        gt.logger.Warnf("Potential goroutine leak in module %s: %d new goroutines", 
            moduleName, delta)
    }
}
```

#### 4.2 Security Considerations

**Module Validation:**

```go
// pkg/worker/validation/module_validator.go
type ModuleValidator struct {
    trustedSources []string
    checksumDB     ChecksumDatabase
    logger         logging.Logger
}

func (mv *ModuleValidator) ValidateModule(module worker.WorkerModule) error {
    info := module.GetInfo()
    
    // Validate module metadata
    if err := mv.validateModuleInfo(info); err != nil {
        return fmt.Errorf("module info validation failed: %w", err)
    }
    
    // Check against known good checksums
    if err := mv.validateChecksum(info); err != nil {
        return fmt.Errorf("checksum validation failed: %w", err)
    }
    
    // Validate author/source
    if err := mv.validateSource(info); err != nil {
        return fmt.Errorf("source validation failed: %w", err)
    }
    
    return nil
}

func (mv *ModuleValidator) validateModuleInfo(info worker.ModuleInfo) error {
    if info.Name == "" {
        return errors.New("module name is required")
    }
    
    if info.Version == "" {
        return errors.New("module version is required")
    }
    
    // Validate version format
    if !semver.IsValid(info.Version) {
        return fmt.Errorf("invalid version format: %s", info.Version)
    }
    
    return nil
}
```

**Access Control:**

```go
// pkg/worker/security/access_control.go
type ModuleAccessControl struct {
    allowedOperations map[string][]string
    modulePermissions map[string]ModulePermissions
    logger           logging.Logger
}

type ModulePermissions struct {
    AllowNetworkAccess bool
    AllowFileAccess    bool
    AllowedPorts       []int
    MaxGoroutines      int
}

func (mac *ModuleAccessControl) CheckPermission(moduleName, operation string) error {
    allowed, exists := mac.allowedOperations[moduleName]
    if !exists {
        return fmt.Errorf("no permissions defined for module: %s", moduleName)
    }
    
    for _, allowedOp := range allowed {
        if allowedOp == operation || allowedOp == "*" {
            return nil
        }
    }
    
    return fmt.Errorf("operation %s not allowed for module %s", operation, moduleName)
}
```

#### 4.3 Documentation and Training

**API Documentation Generation:**

```go
// cmd/docs/generate.go
func generateAPIDocumentation() {
    // Generate documentation from code annotations
    interfaces := []interface{}{
        (*worker.WorkerModule)(nil),
        (*WorkerController)(nil),
        (*ModuleRuntime)(nil),
    }
    
    for _, iface := range interfaces {
        doc := extractDocumentation(iface)
        writeMarkdownDoc(doc)
    }
}
```

## Constraints and Limitations

### Documented Constraints

#### 1. Shared Fate for In-Proc Modules
```go
// This is a documented architectural decision
// Modules can crash the Master process
//
// Mitigation strategies:
// - Panic recovery in goroutine manager
// - Health monitoring with restart
// - Graceful degradation where possible
```

#### 2. No Resource Limits for Modules
```go
// Modules share Master process resources
// Cannot enforce CPU/memory limits like processes
//
// Recommendations:
// - Use modules for trusted, well-behaved services
// - Monitor resource usage at application level
// - Consider process workers for resource-intensive services
```

#### 3. No Hot-Swapping
```go
// Module updates require application restart
// Configuration changes require restart
//
// Benefits:
// - Simpler implementation
// - No version conflicts
// - Consistent state
```

### Technical Limitations

#### 1. Platform Dependencies
```go
// Go plugin support varies by platform
// Native modules recommended for maximum compatibility
//
// Plugin support:
// ✅ Linux (amd64, arm64)
// ✅ macOS (amd64, arm64)  
// ❌ Windows (limited support)
```

#### 2. Goroutine Isolation
```go
// Modules run in goroutines, not separate processes
// Shared memory space with Master
//
// Implications:
// - Global state can be affected
// - Panic can crash entire application
// - Resource contention possible
```

## Error Handling Strategies

### Module Failure Recovery

```go
// pkg/worker/recovery/module_recovery.go
type ModuleRecoveryManager struct {
    maxRetries    int
    retryDelay    time.Duration
    backoffFactor float64
    logger        logging.Logger
}

func (mrm *ModuleRecoveryManager) HandleModuleFailure(
    moduleName string, 
    err error, 
    restartFunc func() error,
) error {
    
    for attempt := 1; attempt <= mrm.maxRetries; attempt++ {
        mrm.logger.Warnf("Module %s failed (attempt %d/%d): %v", 
            moduleName, attempt, mrm.maxRetries, err)
        
        // Exponential backoff
        delay := time.Duration(float64(mrm.retryDelay) * math.Pow(mrm.backoffFactor, float64(attempt-1)))
        time.Sleep(delay)
        
        if restartErr := restartFunc(); restartErr == nil {
            mrm.logger.Infof("Module %s recovered after %d attempts", moduleName, attempt)
            return nil
        }
    }
    
    return fmt.Errorf("module %s failed to recover after %d attempts", moduleName, mrm.maxRetries)
}
```

### Graceful Degradation

```go
// pkg/master/degradation.go
func (m *Master) handleWorkerFailure(workerID string, err error) {
    entry, exists := m.workers[workerID]
    if !exists {
        return
    }
    
    switch entry.Type {
    case WorkerTypeModule:
        // For modules, try restart first, then degradation
        if restartErr := m.restartModuleWorker(workerID); restartErr != nil {
            m.degradeModuleWorker(workerID, err)
        }
    case WorkerTypeProcess:
        // For processes, standard restart logic
        m.restartProcessWorker(workerID)
    }
}

func (m *Master) degradeModuleWorker(workerID string, originalErr error) {
    m.logger.Warnf("Degrading module worker %s due to failure: %v", workerID, originalErr)
    
    // Mark as degraded but keep in worker list
    entry := m.workers[workerID]
    entry.State = WorkerStateDegraded
    entry.DegradationReason = originalErr.Error()
    
    // Notify dependent services
    m.notifyWorkerDegradation(workerID, originalErr)
}
```

## Performance Optimization

### Module Pool Management

```go
// pkg/worker/pool/module_pool.go
type ModulePool struct {
    factories map[string]ModuleFactory
    instances map[string][]worker.WorkerModule
    maxInstances int
    mutex     sync.RWMutex
}

func (mp *ModulePool) GetModule(moduleName string) (worker.WorkerModule, error) {
    mp.mutex.Lock()
    defer mp.mutex.Unlock()
    
    instances, exists := mp.instances[moduleName]
    if exists && len(instances) > 0 {
        // Reuse existing instance
        module := instances[len(instances)-1]
        mp.instances[moduleName] = instances[:len(instances)-1]
        return module, nil
    }
    
    // Create new instance
    factory, exists := mp.factories[moduleName]
    if !exists {
        return nil, fmt.Errorf("module factory not found: %s", moduleName)
    }
    
    return factory(), nil
}

func (mp *ModulePool) ReturnModule(moduleName string, module worker.WorkerModule) {
    mp.mutex.Lock()
    defer mp.mutex.Unlock()
    
    instances := mp.instances[moduleName]
    if len(instances) < mp.maxInstances {
        mp.instances[moduleName] = append(instances, module)
    }
    // If pool is full, let module be garbage collected
}
```

## Next Steps

- See [Configuration and Usage](configuration-and-usage.md) for application integration
- See [Module Development Guide](module-development-guide.md) for module creation
- See [Developer Experience](developer-experience.md) for benefits and migration strategies