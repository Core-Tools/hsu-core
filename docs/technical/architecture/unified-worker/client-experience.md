# Module Client Experience and Location Transparency

## Overview

This document explores how clients interact with modules deployed in different locations, the current limitations in location transparency, and a proposed solution using domain contract separation.

## Current State: Endpoint Discovery

### Module Endpoint Discovery

Clients can discover module endpoints through the Master API:

```go
// Get worker information including endpoints
state, err := master.GetWorkerStateWithDiagnostics("data-processor-1")
if err != nil {
    return fmt.Errorf("failed to get worker state: %w", err)
}

// Extract endpoint information
if state.ProcessDiagnostics != nil {
    // Process worker endpoint
    endpoint := state.ProcessDiagnostics.Endpoint
} else if state.ModuleDiagnostics != nil {
    // Module worker endpoint  
    endpoint := state.ModuleDiagnostics.ModuleInfo.Endpoints[0]
}
```

### Enhanced Master API for Endpoint Discovery

```go
// Enhanced Master API for easier endpoint discovery
func (m *Master) GetWorkerEndpoint(workerID string, endpointName string) (*WorkerEndpoint, error) {
    state, err := m.GetWorkerStateWithDiagnostics(workerID)
    if err != nil {
        return nil, err
    }
    
    switch state.Type {
    case WorkerTypeProcess:
        return m.getProcessWorkerEndpoint(state, endpointName)
    case WorkerTypeModule:
        return m.getModuleWorkerEndpoint(state, endpointName)
    default:
        return nil, fmt.Errorf("unknown worker type: %s", state.Type)
    }
}

type WorkerEndpoint struct {
    Type     EndpointType
    Address  string
    Metadata map[string]interface{}
}
```

## Client Experience Examples

### Example 1: gRPC/HTTP Endpoint (Process or Module)

#### Module Side - Publishing gRPC Endpoint

```go
// data-processor module publishing gRPC endpoint
type DataProcessor struct {
    server   *grpc.Server
    listener net.Listener
}

func (dp *DataProcessor) Start(ctx context.Context) error {
    // Start gRPC server
    dp.server = grpc.NewServer()
    RegisterDataProcessorService(dp.server, dp)
    
    var err error
    dp.listener, err = net.Listen("tcp", dp.config.Port)
    if err != nil {
        return err
    }
    
    go dp.server.Serve(dp.listener)
    return nil
}

func (dp *DataProcessor) GetInfo() worker.ModuleInfo {
    return worker.ModuleInfo{
        Name: "data-processor",
        Endpoints: []worker.EndpointInfo{
            {
                Name:    "grpc-api",
                Type:    "grpc",
                Address: dp.listener.Addr().String(), // Actual bound address
            },
        },
    }
}
```

#### Client Side - Using gRPC Endpoint

```go
// Client discovering and using gRPC endpoint
func useDataProcessorService(master *Master) error {
    // 1. Discover endpoint
    endpoint, err := master.GetWorkerEndpoint("data-processor-1", "grpc-api")
    if err != nil {
        return err
    }
    
    // 2. Connect to gRPC service
    conn, err := grpc.Dial(endpoint.Address, grpc.WithInsecure())
    if err != nil {
        return err
    }
    defer conn.Close()
    
    // 3. Use service (same code regardless of deployment type!)
    client := NewDataProcessorServiceClient(conn)
    response, err := client.ProcessData(context.Background(), &ProcessDataRequest{
        Data: "some data",
    })
    
    return err
}
```

### Example 2: Function Endpoint (Go Plugin Module)

#### Module Side - Publishing Function Endpoint

```go
// Plugin module exposing function interface
type PluginDataProcessor struct {
    // Internal state
}

// Exported function for plugin loading
func NewDataProcessorPlugin() worker.WorkerModule {
    return &PluginDataProcessor{}
}

func (pdp *PluginDataProcessor) GetInfo() worker.ModuleInfo {
    return worker.ModuleInfo{
        Name: "data-processor",
        Endpoints: []worker.EndpointInfo{
            {
                Name:    "process-function",
                Type:    "function",
                Address: "ProcessData", // Function name
            },
            {
                Name:    "batch-function", 
                Type:    "function",
                Address: "ProcessBatch",
            },
        },
    }
}

// Exposed functions (plugin interface)
func (pdp *PluginDataProcessor) ProcessData(data string) (string, error) {
    // Implementation
    return "processed: " + data, nil
}

func (pdp *PluginDataProcessor) ProcessBatch(batch []string) ([]string, error) {
    // Implementation
    result := make([]string, len(batch))
    for i, data := range batch {
        result[i] = "processed: " + data
    }
    return result, nil
}
```

#### Client Side - Using Function Endpoint

```go
// Client using plugin function endpoint
func usePluginDataProcessor(master *Master) error {
    // 1. Discover function endpoint
    endpoint, err := master.GetWorkerEndpoint("data-processor-plugin", "process-function")
    if err != nil {
        return err
    }
    
    // 2. Get function reference (Master provides this)
    funcRef, err := master.GetModuleFunction("data-processor-plugin", endpoint.Address)
    if err != nil {
        return err
    }
    
    // 3. Type assert and call function
    if processFunc, ok := funcRef.(func(string) (string, error)); ok {
        result, err := processFunc("some data")
        if err != nil {
            return err
        }
        
        fmt.Printf("Result: %s\n", result)
    }
    
    return nil
}
```

### Example 3: Function Endpoint (Native Go Module)

#### Module Side - Publishing Native Function Endpoint

```go
// Native module with direct function access
type NativeDataProcessor struct {
    config Config
}

func (ndp *NativeDataProcessor) GetInfo() worker.ModuleInfo {
    return worker.ModuleInfo{
        Name: "data-processor",
        Endpoints: []worker.EndpointInfo{
            {
                Name:    "direct-function",
                Type:    "function",
                Address: "ProcessData", // Method name
            },
        },
    }
}

// Direct method access (since it's compiled into same binary)
func (ndp *NativeDataProcessor) ProcessData(ctx context.Context, data string) (string, error) {
    return "native processed: " + data, nil
}

// Factory with function exposure
func NewNativeDataProcessor() worker.WorkerModule {
    processor := &NativeDataProcessor{}
    
    // Register function endpoints in Master's function registry
    master.RegisterModuleFunction("data-processor", "ProcessData", processor.ProcessData)
    
    return processor
}
```

#### Client Side - Using Native Function Endpoint

```go
// Client using native function endpoint
func useNativeDataProcessor(master *Master) error {
    // 1. Discover function endpoint
    endpoint, err := master.GetWorkerEndpoint("data-processor-native", "direct-function")
    if err != nil {
        return err
    }
    
    // 2. Get direct function reference
    funcRef, err := master.GetModuleFunction("data-processor-native", endpoint.Address)
    if err != nil {
        return err
    }
    
    // 3. Type assert and call (direct function call, no serialization!)
    if processFunc, ok := funcRef.(func(context.Context, string) (string, error)); ok {
        result, err := processFunc(context.Background(), "some data")
        if err != nil {
            return err
        }
        
        fmt.Printf("Result: %s\n", result)
    }
    
    return nil
}
```

## The Problem: Lack of Client-Side Location Transparency

As these examples show, **clients must handle different endpoint types differently**:

```go
// Client code is different for each deployment type! ❌
switch endpoint.Type {
case "grpc":
    // Use gRPC client
    conn, _ := grpc.Dial(endpoint.Address)
    client := NewServiceClient(conn)
    result, _ := client.Process(req)
    
case "function":
    // Use function call
    funcRef, _ := master.GetModuleFunction(workerID, endpoint.Address)
    result, _ := funcRef.(ProcessFunc)(data)
    
case "http":
    // Use HTTP client
    resp, _ := http.Post(endpoint.Address, "application/json", body)
}
```

**This breaks location transparency from the client perspective!**

## Solution: Domain Contract Separation (Following hsu-example1-go Pattern)

### Step 1: Define Location-Agnostic Domain Contract

```go
// pkg/domain/dataprocessor.go
package domain

import "context"

// Location-agnostic business interface
type DataProcessorContract interface {
    ProcessData(ctx context.Context, data string) (string, error)
    ProcessBatch(ctx context.Context, batch []string) ([]string, error)
    GetStatus(ctx context.Context) (ProcessorStatus, error)
}

type ProcessorStatus struct {
    IsHealthy     bool
    QueueSize     int
    ProcessedCount int64
}
```

### Step 2: Module Implementation (Business Logic)

```go
// pkg/business/dataprocessor.go  
package business

import (
    "context"
    "strings"
    "github.com/my-app/pkg/domain"
)

type DataProcessorImpl struct {
    config      Config
    processedCount int64
    queueSize     int
}

// Implements domain contract with actual business logic
func (dp *DataProcessorImpl) ProcessData(ctx context.Context, data string) (string, error) {
    // Actual business logic (location-independent)
    processed := strings.ToUpper(data) + "-PROCESSED"
    dp.processedCount++
    return processed, nil
}

func (dp *DataProcessorImpl) ProcessBatch(ctx context.Context, batch []string) ([]string, error) {
    result := make([]string, len(batch))
    for i, data := range batch {
        processed, err := dp.ProcessData(ctx, data)
        if err != nil {
            return nil, err
        }
        result[i] = processed
    }
    return result, nil
}

func (dp *DataProcessorImpl) GetStatus(ctx context.Context) (domain.ProcessorStatus, error) {
    return domain.ProcessorStatus{
        IsHealthy:      true,
        QueueSize:      dp.queueSize,
        ProcessedCount: dp.processedCount,
    }, nil
}

func NewDataProcessorImpl(config Config) domain.DataProcessorContract {
    return &DataProcessorImpl{
        config: config,
    }
}
```

### Step 3: Communication Adapters

#### gRPC Handler (for process/module deployment)

```go
// pkg/adapters/grpc_handler.go
package adapters

import (
    "context"
    "github.com/my-app/pkg/domain" 
    "github.com/my-app/pkg/generated/proto"
    "github.com/core-tools/hsu-master/pkg/master"
)

type GRPCDataProcessorHandler struct {
    proto.UnimplementedDataProcessorServiceServer
    businessLogic domain.DataProcessorContract
}

func (handler *GRPCDataProcessorHandler) ProcessData(ctx context.Context, req *proto.ProcessDataRequest) (*proto.ProcessDataResponse, error) {
    // Convert proto to domain
    result, err := handler.businessLogic.ProcessData(ctx, req.Data)
    if err != nil {
        return nil, err
    }
    
    // Convert domain to proto
    return &proto.ProcessDataResponse{Result: result}, nil
}

func (handler *GRPCDataProcessorHandler) ProcessBatch(ctx context.Context, req *proto.ProcessBatchRequest) (*proto.ProcessBatchResponse, error) {
    result, err := handler.businessLogic.ProcessBatch(ctx, req.Data)
    if err != nil {
        return nil, err
    }
    return &proto.ProcessBatchResponse{Results: result}, nil
}

// Register gRPC handler with Master (generic pattern)
func RegisterGRPCDataProcessorHandler(registrar master.EndpointRegistrar, businessLogic domain.DataProcessorContract) error {
    handler := &GRPCDataProcessorHandler{businessLogic: businessLogic}
    
    return registrar.RegisterGRPCHandler("data-processor", "grpc-api", func(server grpc.ServiceRegistrar) {
        proto.RegisterDataProcessorServiceServer(server, handler)
    })
}
```

#### gRPC Gateway (for client-side)

```go
// pkg/adapters/grpc_gateway.go
package adapters

import (
    "context"
    "google.golang.org/grpc"
    "github.com/my-app/pkg/domain"
    "github.com/my-app/pkg/generated/proto" 
    "github.com/core-tools/hsu-master/pkg/master"
)

type GRPCDataProcessorGateway struct {
    grpcClient proto.DataProcessorServiceClient
    conn       *grpc.ClientConn
}

// Implements domain contract but makes gRPC calls
func (gateway *GRPCDataProcessorGateway) ProcessData(ctx context.Context, data string) (string, error) {
    // Convert domain to proto
    req := &proto.ProcessDataRequest{Data: data}
    
    // Make gRPC call
    resp, err := gateway.grpcClient.ProcessData(ctx, req)
    if err != nil {
        return "", err
    }
    
    // Convert proto to domain
    return resp.Result, nil
}

func (gateway *GRPCDataProcessorGateway) ProcessBatch(ctx context.Context, batch []string) ([]string, error) {
    req := &proto.ProcessBatchRequest{Data: batch}
    resp, err := gateway.grpcClient.ProcessBatch(ctx, req)
    if err != nil {
        return nil, err
    }
    return resp.Results, nil
}

func (gateway *GRPCDataProcessorGateway) GetStatus(ctx context.Context) (domain.ProcessorStatus, error) {
    resp, err := gateway.grpcClient.GetStatus(ctx, &proto.GetStatusRequest{})
    if err != nil {
        return domain.ProcessorStatus{}, err
    }
    
    return domain.ProcessorStatus{
        IsHealthy:      resp.IsHealthy,
        QueueSize:      int(resp.QueueSize),
        ProcessedCount: resp.ProcessedCount,
    }, nil
}

func (gateway *GRPCDataProcessorGateway) Close() error {
    if gateway.conn != nil {
        return gateway.conn.Close()
    }
    return nil
}

// Standalone factory function (gRPC pattern)
func NewGRPCDataProcessorGateway(endpoint master.EndpointInfo) (domain.DataProcessorContract, error) {
    conn, err := grpc.Dial(endpoint.Address, grpc.WithInsecure())
    if err != nil {
        return nil, err
    }
    
    grpcClient := proto.NewDataProcessorServiceClient(conn)
    return &GRPCDataProcessorGateway{
        grpcClient: grpcClient,
        conn:       conn,
    }, nil
}
```

#### Direct Function Gateway (for native modules)

```go
// pkg/adapters/direct_gateway.go
package adapters

import (
    "context"
    "github.com/my-app/pkg/domain"
    "github.com/core-tools/hsu-master/pkg/master"
)

type DirectDataProcessorGateway struct {
    businessLogic domain.DataProcessorContract
}

// Direct passthrough (no serialization overhead!)
func (gateway *DirectDataProcessorGateway) ProcessData(ctx context.Context, data string) (string, error) {
    return gateway.businessLogic.ProcessData(ctx, data)
}

func (gateway *DirectDataProcessorGateway) ProcessBatch(ctx context.Context, batch []string) ([]string, error) {
    return gateway.businessLogic.ProcessBatch(ctx, batch)
}

func (gateway *DirectDataProcessorGateway) GetStatus(ctx context.Context) (domain.ProcessorStatus, error) {
    return gateway.businessLogic.GetStatus(ctx)
}

// Standalone factory function for direct access
func NewDirectDataProcessorGateway(endpoint master.EndpointInfo, contractProvider master.ContractProvider) (domain.DataProcessorContract, error) {
    // Get direct reference to business logic
    contract, err := contractProvider.GetContract(endpoint.Metadata["module_id"].(string), "data-processor")
    if err != nil {
        return nil, err
    }
    
    businessLogic := contract.(domain.DataProcessorContract)
    return &DirectDataProcessorGateway{businessLogic: businessLogic}, nil
}
```

### Step 4: Unified Module Implementation with Master Integration

```go
// pkg/modules/dataprocessor.go
package modules

import (
    "context"
    "github.com/core-tools/hsu-master/pkg/worker"
    "github.com/core-tools/hsu-master/pkg/master"
    "github.com/my-app/pkg/business"
    "github.com/my-app/pkg/adapters"
    "github.com/my-app/pkg/domain"
)

type DataProcessorModule struct {
    config        Config
    businessLogic domain.DataProcessorContract
    registrar     master.EndpointRegistrar  // For handler registration
}

func (dp *DataProcessorModule) Initialize(ctx context.Context, configMap map[string]interface{}) error {
    dp.config = parseConfig(configMap)
    
    // Create business logic (same for all deployment types)
    dp.businessLogic = business.NewDataProcessorImpl(dp.config.BusinessConfig)
    
    // Get registrar from context (provided by Master)
    if registrar, ok := ctx.Value("endpoint_registrar").(master.EndpointRegistrar); ok {
        dp.registrar = registrar
    }
    
    return nil
}

func (dp *DataProcessorModule) Start(ctx context.Context) error {
    // Register gRPC handler with Master (works for both in-proc and out-of-proc)
    err := adapters.RegisterGRPCDataProcessorHandler(dp.registrar, dp.businessLogic)
    if err != nil {
        return fmt.Errorf("failed to register gRPC handler: %w", err)
    }
    
    // For native modules, also register direct contract access
    if dp.config.ExposeDirectFunctions {
        err = dp.registrar.RegisterDirectContract("data-processor", "business-contract", dp.businessLogic)
        if err != nil {
            return fmt.Errorf("failed to register direct contract: %w", err)
        }
    }
    
    return nil
}

func (dp *DataProcessorModule) Stop(ctx context.Context) error {
    // Unregister endpoints
    if dp.registrar != nil {
        dp.registrar.UnregisterEndpoints("data-processor")
    }
    return nil
}

func (dp *DataProcessorModule) IsHealthy() bool {
    if dp.businessLogic == nil {
        return false
    }
    
    status, err := dp.businessLogic.GetStatus(context.Background())
    return err == nil && status.IsHealthy
}

func (dp *DataProcessorModule) GetInfo() worker.ModuleInfo {
    return worker.ModuleInfo{
        Name:        "data-processor",
        Version:     "1.0.0",
        Description: "High-performance data processing service",
        Author:      "Data Team",
        // Endpoints are dynamically registered via Master
    }
}

func NewDataProcessorModule() worker.WorkerModule {
    return &DataProcessorModule{}
}
```

### Step 5: Master Enhancement with Interface-Based Registration

```go
// Enhanced Master interfaces for generic endpoint management
type EndpointRegistrar interface {
    RegisterGRPCHandler(serviceName, endpointName string, registerFunc func(grpc.ServiceRegistrar)) error
    RegisterDirectContract(serviceName, contractName string, contract interface{}) error
    UnregisterEndpoints(serviceName string) error
}

type ContractProvider interface {
    GetContract(moduleID, contractName string) (interface{}, error)
    GetEndpoint(serviceName, endpointName string) (EndpointInfo, error)
}

type ServiceRegistry interface {
    RegisterGatewayFactory(serviceName, protocol string, factory GatewayFactory) error
    GetGatewayFactory(serviceName, protocol string) (GatewayFactory, error)
}

type GatewayFactory func(endpoint EndpointInfo, provider ContractProvider) (interface{}, error)

// Enhanced Master implementation
type Master struct {
    // ... existing fields ...
    
    // NEW: Service registry for gateway factories
    gatewayFactories map[string]map[string]GatewayFactory  // [serviceName][protocol] -> factory
    
    // NEW: Contract storage for direct access (in-proc modules)
    directContracts map[string]map[string]interface{}  // [serviceName][contractName] -> contract
    
    // NEW: Endpoint information storage
    serviceEndpoints map[string][]EndpointInfo  // [serviceName] -> endpoints
}

// Generic contract discovery (no reflection-based switches!)
func (m *Master) GetServiceContract(serviceName string, protocol string) (interface{}, error) {
    // Get gateway factory for this service and protocol
    factory, err := m.GetGatewayFactory(serviceName, protocol)
    if err != nil {
        return nil, err
    }
    
    // Get endpoint for this service and protocol
    endpoint, err := m.GetEndpoint(serviceName, protocol)
    if err != nil {
        return nil, err
    }
    
    // Use factory to create gateway
    return factory(endpoint, m)
}

// Implementation of ContractProvider
func (m *Master) GetContract(moduleID, contractName string) (interface{}, error) {
    if contracts, exists := m.directContracts[moduleID]; exists {
        if contract, exists := contracts[contractName]; exists {
            return contract, nil
        }
    }
    return nil, fmt.Errorf("direct contract not found: %s.%s", moduleID, contractName)
}

func (m *Master) GetEndpoint(serviceName, endpointName string) (EndpointInfo, error) {
    endpoints, exists := m.serviceEndpoints[serviceName]
    if !exists {
        return EndpointInfo{}, fmt.Errorf("service not found: %s", serviceName)
    }
    
    for _, endpoint := range endpoints {
        if endpoint.Name == endpointName {
            return endpoint, nil
        }
    }
    
    return EndpointInfo{}, fmt.Errorf("endpoint not found: %s.%s", serviceName, endpointName)
}

// Implementation of EndpointRegistrar  
func (m *Master) RegisterGRPCHandler(serviceName, endpointName string, registerFunc func(grpc.ServiceRegistrar)) error {
    // For in-proc modules: register with Master's gRPC server
    // For out-of-proc modules: register with side-instance Master's gRPC server
    
    grpcServer := m.getGRPCServer() // Gets appropriate gRPC server instance
    registerFunc(grpcServer)
    
    // Record endpoint
    endpoint := EndpointInfo{
        Name:    endpointName,
        Type:    "grpc",
        Address: m.getGRPCServerAddress(),
    }
    
    m.serviceEndpoints[serviceName] = append(m.serviceEndpoints[serviceName], endpoint)
    return nil
}

func (m *Master) RegisterDirectContract(serviceName, contractName string, contract interface{}) error {
    if m.directContracts[serviceName] == nil {
        m.directContracts[serviceName] = make(map[string]interface{})
    }
    
    m.directContracts[serviceName][contractName] = contract
    
    // Record endpoint for direct access
    endpoint := EndpointInfo{
        Name:    contractName,
        Type:    "direct",
        Address: "in-memory",
        Metadata: map[string]interface{}{
            "module_id": serviceName,
        },
    }
    
    m.serviceEndpoints[serviceName] = append(m.serviceEndpoints[serviceName], endpoint)
    return nil
}
```

### Step 6: Symmetric Master Architecture (In-Proc vs Out-of-Proc)

#### For In-Proc Modules (Native)

```go
// Application with in-proc modules
func main() {
    // Main Master instance
    master := master.NewMaster(master.Config{})
    
    // Register gateway factories
    master.RegisterGatewayFactory("data-processor", "grpc", adapters.NewGRPCDataProcessorGateway)
    master.RegisterGatewayFactory("data-processor", "direct", adapters.NewDirectDataProcessorGateway)
    
    // Register native module factory
    master.RegisterNativeModuleFactory("data-processor", modules.NewDataProcessorModule)
    
    // Add worker (module will register handlers during Start)
    config := WorkerConfig{
        ID:         "data-processor-1",
        Type:       WorkerTypeModule,
        ModuleType: ModuleTypeNative,
        ModuleName: "data-processor",
    }
    
    master.AddWorker(config)
    master.StartWorker(context.Background(), "data-processor-1")
}
```

#### For Out-of-Proc Modules (Process)

```go
// Standalone process with side-instance Master
func main() {
    // Side-instance Master for this process
    sideMaster := master.NewSideMaster(master.SideConfig{
        MainMasterAddress: "localhost:9999",  // Connect to main Master
        ProcessID:         "data-processor-1",
    })
    
    // Create and register module with side Master
    module := modules.NewDataProcessorModule()
    
    // Side Master provides EndpointRegistrar via context
    ctx := context.WithValue(context.Background(), "endpoint_registrar", sideMaster)
    
    err := module.Initialize(ctx, loadConfig())
    if err != nil {
        log.Fatal(err)
    }
    
    err = module.Start(ctx)  // Module registers handlers with side Master
    if err != nil {
        log.Fatal(err)
    }
    
    // Side Master communicates endpoint info to main Master
    sideMaster.PublishEndpoints()
    
    // Keep running
    select {}
}
```

### Step 7: True Location-Transparent Client

```go
// Client setup - register gateway factories once
func setupClient(master *Master) {
    // Register gateway factories for all services
    master.RegisterGatewayFactory("data-processor", "grpc", adapters.NewGRPCDataProcessorGateway)
    master.RegisterGatewayFactory("data-processor", "direct", adapters.NewDirectDataProcessorGateway)
    
    // Gateway selection priority: direct > grpc
    master.SetGatewayPriority("data-processor", []string{"direct", "grpc"})
}

// Client code with TRUE location transparency! ✅
func useDataProcessorService(master *Master) error {
    // Get service contract (location-transparent!)
    // Master automatically selects best available protocol (direct for native, grpc for process)
    serviceInterface, err := master.GetServiceContract("data-processor", "auto")
    if err != nil {
        return err
    }
    
    // Type assert to business contract
    processor := serviceInterface.(domain.DataProcessorContract)
    
    // Use service (SAME CODE regardless of deployment type!)
    result, err := processor.ProcessData(context.Background(), "some data")
    if err != nil {
        return err
    }
    
    batch := []string{"data1", "data2", "data3"}
    batchResults, err := processor.ProcessBatch(context.Background(), batch)
    if err != nil {
        return err
    }
    
    status, err := processor.GetStatus(context.Background())
    if err != nil {
        return err
    }
    
    fmt.Printf("Result: %s\n", result)
    fmt.Printf("Batch Results: %v\n", batchResults)
    fmt.Printf("Status: %+v\n", status)
    
    return nil
}

// Advanced: Explicit protocol selection
func useDataProcessorWithSpecificProtocol(master *Master) error {
    // Force gRPC (for testing process communication)
    grpcProcessor, err := master.GetServiceContract("data-processor", "grpc")
    if err != nil {
        return err
    }
    
    // Force direct access (for testing native modules)
    directProcessor, err := master.GetServiceContract("data-processor", "direct")
    if err != nil {
        return err
    }
    
    // Same interface for both!
    processor1 := grpcProcessor.(domain.DataProcessorContract)
    processor2 := directProcessor.(domain.DataProcessorContract)
    
    // Compare performance
    start := time.Now()
    processor1.ProcessData(context.Background(), "test")
    grpcLatency := time.Since(start)
    
    start = time.Now()
    processor2.ProcessData(context.Background(), "test")
    directLatency := time.Since(start)
    
    fmt.Printf("gRPC latency: %v, Direct latency: %v\n", grpcLatency, directLatency)
    return nil
}
```

## Benefits of Domain Contract Separation with Symmetric Master Architecture

### ✅ **True Location Transparency**
```go
// Same client code works for:
// - Native modules (direct function calls, no serialization)
// - Process workers (gRPC with serialization)
// - Plugin modules (function calls with potential serialization)

processor := master.GetServiceContract("data-processor", "auto") // Master handles location details
result := processor.ProcessData(ctx, data) // Same interface!
```

### ✅ **Symmetric Master Architecture**
```go
// In-Proc Modules:
// Main Master -> Module.Initialize() -> RegisterHandler(mainMaster.EndpointRegistrar)

// Out-of-Proc Modules:  
// Side Master -> Module.Initialize() -> RegisterHandler(sideMaster.EndpointRegistrar)
// Side Master -> PublishEndpoints() -> Main Master

// Same registration pattern for both!
```

### ✅ **Performance Optimization**
```go
// Native modules: Direct function calls (fastest)
// - No serialization overhead
// - No network overhead
// - Full type safety

// Process workers: gRPC calls (isolated but slower)
// - Serialization overhead
// - Network overhead
// - Process isolation benefits
```

### ✅ **Clear Separation of Concerns**
- **Domain Layer**: Business logic, location-agnostic
- **Adapter Layer**: Protocol-specific communication
- **Module Layer**: Lifecycle management, endpoint publishing
- **Client Layer**: Business interface usage

### ✅ **Testability**
```go
// Test business logic in isolation
func TestBusinessLogic(t *testing.T) {
    businessLogic := business.NewDataProcessorImpl(config)
    result, err := businessLogic.ProcessData(ctx, "test")
    assert.NoError(t, err)
    assert.Equal(t, "TEST-PROCESSED", result)
}

// Test gRPC adapter
func TestGRPCAdapter(t *testing.T) {
    mockBusiness := &MockDataProcessor{}
    adapter := &GRPCServerAdapter{businessLogic: mockBusiness}
    // Test protocol conversion
}
```

## Implementation Strategy

### Phase 1: Pattern Establishment
1. Define domain contracts for existing services
2. Create adapter templates and code generation tools
3. Implement pattern for one service as proof of concept

### Phase 2: Gradual Migration
1. Migrate existing services one by one
2. Update clients to use contract-based access
3. Maintain backward compatibility during transition

### Phase 3: Platform Enhancement
1. Enhanced Master APIs for contract discovery
2. Code generation tools for adapters
3. Development tooling for pattern enforcement

## Code Generation Support

To reduce manual work, provide code generation tools:

```bash
# Generate gateways and handlers from domain contract
hsu-gen adapters \
  --contract github.com/my-app/pkg/domain.DataProcessorContract \
  --protocols grpc,http \
  --output pkg/adapters/
  
# Generates:
# - grpc_handler.go         (server-side, for modules)
# - grpc_gateway.go         (client-side, for clients)  
# - http_handler.go         (server-side, for modules)
# - http_gateway.go         (client-side, for clients)
# - direct_gateway.go       (client-side, for native modules)

# Also generates registration functions:
# - RegisterGRPCDataProcessorHandler()
# - NewGRPCDataProcessorGateway()
# - NewHTTPDataProcessorGateway()
# - NewDirectDataProcessorGateway()
```

### Generated Gateway Factory Registration

```bash
# Generate gateway factory registration code
hsu-gen factories \
  --contract github.com/my-app/pkg/domain.DataProcessorContract \
  --service-name data-processor \
  --output pkg/client/

# Generates:
# - data_processor_client.go

func RegisterDataProcessorGateways(master *Master) {
    master.RegisterGatewayFactory("data-processor", "grpc", adapters.NewGRPCDataProcessorGateway)
    master.RegisterGatewayFactory("data-processor", "http", adapters.NewHTTPDataProcessorGateway)
    master.RegisterGatewayFactory("data-processor", "direct", adapters.NewDirectDataProcessorGateway)
}
```

## Conclusion

By following the **domain contract separation pattern** from `hsu-example1-go` and implementing **symmetric Master architecture**, we achieve **true location transparency** for clients while maintaining **clear separation of communication concerns**.

### Key Innovations

#### ✅ **Gateway/Handler Pattern (Following Industry Standards)**
- **Gateways** (client-side): Industry-standard pattern for outbound communication
- **Handlers** (server-side): Widely recognized pattern for inbound request processing
- **Standalone Factory Functions**: gRPC-style registration pattern (no reflection switches)

#### ✅ **Symmetric Master Architecture**
- **In-Proc Modules**: Main Master manages lifecycle and endpoint registration
- **Out-of-Proc Modules**: Side-instance Master provides same registration patterns
- **Unified Registration**: Same `EndpointRegistrar` interface for both deployment types

#### ✅ **Interface-Based Service Discovery**
- **Generic Gateway Factories**: No service-specific code in Master
- **Contract Provider Interface**: Clean abstraction for direct contract access
- **Service Registry**: Pluggable gateway factory registration

#### ✅ **True Location Transparency**
```go
// Same client code works for all deployment types:
processor := master.GetServiceContract("data-processor", "auto")
result := processor.ProcessData(ctx, data)

// Master automatically selects optimal protocol:
// - Native modules: Direct function calls (fastest)
// - Process workers: gRPC calls (isolated)
// - Plugin modules: Function calls (flexible)
```

#### ✅ **Performance Optimization with Choice**
```go
// Automatic protocol selection based on availability
master.SetGatewayPriority("data-processor", []string{"direct", "grpc", "http"})

// Or explicit protocol selection for testing/benchmarking
grpcProcessor := master.GetServiceContract("data-processor", "grpc")
directProcessor := master.GetServiceContract("data-processor", "direct")
```

### Architectural Achievements

1. **Clean Abstraction**: No reflection-based switches or service-specific code in core Master
2. **Industry Patterns**: Follows well-established Gateway and Handler patterns
3. **gRPC-Style Registration**: Familiar pattern for service registration and discovery
4. **Symmetric Deployment**: Same patterns work for in-proc and out-of-proc modules
5. **Code Generation Ready**: Structure supports automated gateway/handler generation

This architecture transforms module communication from **deployment-specific client code** to **unified business interfaces with transparent protocol adaptation**.

---

## Related Documentation

- [Module Development Guide](module-development-guide.md) - How to implement modules with domain contracts
- [Master Integration](master-integration.md) - Enhanced Master APIs for contract discovery
- [Configuration and Usage](configuration-and-usage.md) - Deployment configuration examples