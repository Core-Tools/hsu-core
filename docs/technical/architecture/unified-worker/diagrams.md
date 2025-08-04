# Architecture Diagrams

## Overview

This document contains visual representations of the unified worker architecture, including component relationships, state machines, and data flow.

## High-Level Component Architecture

### Unified Worker Hierarchy

```mermaid
graph TD
    A["WorkerUnit (Base Interface)"] --> B["ProcessWorker<br/>(Out-of-Proc)"]
    A --> C["ModuleWorker<br/>(In-Proc)"]
    
    B --> D["ManagedWorker"]
    B --> E["IntegratedWorker"]
    B --> F["UnmanagedWorker"]
    
    C --> G["IntegratedModule<br/>(Always Integrated)"]
    
    H["HSU Master"] --> A
    I["Unified Lifecycle"] --> A
    J["Package Manager"] --> A
    K["Same APIs/Protocols"] --> A
    
    style C fill:#e1f5fe
    style G fill:#e1f5fe
```

### Enhanced Application Architecture

```mermaid
graph TD
    A["Application Binary"] --> B["HSU Master"]
    A --> C["Native Modules<br/>(Compiled In)"]
    A --> D["External Dependencies"]
    
    B --> E["ModuleRegistry"]
    B --> F["ProcessControl"]
    B --> G["WorkerLifecycle"]
    
    C --> H["data-processor<br/>module"]
    C --> I["auth-service<br/>module"] 
    C --> J["log-processor<br/>module"]
    
    E --> K["Native Factory Map"]
    E --> L["Plugin Loader"]
    E --> M["Process Launcher"]
    
    K --> H
    K --> I
    K --> J
    
    style A fill:#e8f5e8
    style C fill:#e1f5fe
    style H fill:#f3e5f5
    style I fill:#f3e5f5
    style J fill:#f3e5f5
```

## State Machines

### Unified Worker State Machine

```mermaid
stateDiagram-v2
    [*] --> Uninitialized
    Uninitialized --> Initializing: Initialize()
    Initializing --> Initialized: Success
    Initializing --> Failed: Error
    
    Initialized --> Starting: Start()
    Starting --> Running: Success
    Starting --> Failed: Error
    
    Running --> Stopping: Stop()
    Running --> Failed: Crash/Error
    
    Stopping --> Stopped: Success
    Stopped --> Starting: Restart
    
    Failed --> Starting: Retry
    Failed --> Stopped: Give Up
```

### Master Worker Management State Flow

```mermaid
stateDiagram-v2
    [*] --> WorkerRegistered: AddWorker()
    WorkerRegistered --> WorkerStarting: StartWorker()
    WorkerStarting --> WorkerRunning: Start Success
    WorkerStarting --> WorkerFailedStart: Start Failed
    
    WorkerRunning --> WorkerStopping: StopWorker()
    WorkerRunning --> WorkerFailedStart: Runtime Error
    
    WorkerStopping --> WorkerStopped: Stop Success
    WorkerStopped --> WorkerStarting: Restart
    
    WorkerFailedStart --> WorkerStarting: Retry
    WorkerFailedStart --> WorkerRemoved: RemoveWorker()
    WorkerStopped --> WorkerRemoved: RemoveWorker()
```

## Component Interaction Flows

### Module Registration and Startup Flow

```mermaid
sequenceDiagram
    participant App as Application
    participant Master as HSU Master
    participant Registry as Module Registry
    participant Controller as Module Controller
    participant Runtime as Module Runtime
    participant Module as Worker Module
    
    App->>Master: RegisterNativeModuleFactory("service", factory)
    Master->>Registry: Register factory
    
    App->>Master: AddWorker(config)
    Master->>Registry: GetFactory("service")
    Registry-->>Master: factory function
    Master->>Master: module = factory()
    Master->>Module: Initialize(config)
    Module-->>Master: success
    Master->>Controller: NewModuleWorkerController(module)
    
    App->>Master: StartWorker("service-1")
    Master->>Controller: Start()
    Controller->>Runtime: StartModule(module)
    Runtime->>Module: Start()
    Module-->>Runtime: success
    Runtime-->>Controller: success
    Controller-->>Master: success
    Master-->>App: success
```

### Process vs Module Worker Comparison

```mermaid
graph TB
    subgraph "Process Worker Flow"
        PA[AddWorker] --> PB[ProcessControl.Start]
        PB --> PC[exec.Command]
        PC --> PD[External Process]
        PD --> PE[IPC Communication]
    end
    
    subgraph "Module Worker Flow" 
        MA[AddWorker] --> MB[ModuleController.Start]
        MB --> MC[GoroutineManager.Start]
        MC --> MD[Module.Start in Goroutine]
        MD --> ME[Direct Function Calls]
    end
    
    subgraph "Unified Master API"
        UA[StartWorker] --> UB{Worker Type?}
        UB -->|Process| PA
        UB -->|Module| MA
    end
    
    style PA fill:#ffebee
    style MA fill:#e8f5e8
    style UA fill:#fff3e0
```

## Data Flow Diagrams

### Configuration to Runtime Flow

```mermaid
flowchart TD
    A[workers.yaml] --> B[Config Parser]
    B --> C{Worker Type?}
    
    C -->|process| D[Process Config]
    C -->|module| E[Module Config]
    
    D --> F[ProcessWorkerController]
    E --> G[ModuleWorkerController]
    
    F --> H[ProcessControl]
    G --> I[ModuleRuntime]
    
    H --> J[OS Process]
    I --> K[Go Goroutine]
    
    J --> L[Network/IPC]
    K --> M[Function Calls]
    
    L --> N[Service Endpoint]
    M --> N
    
    style A fill:#e3f2fd
    style N fill:#e8f5e8
```

### Error Handling and Recovery Flow

```mermaid
flowchart TD
    A[Worker Error] --> B{Worker Type?}
    
    B -->|Process| C[Process Died]
    B -->|Module| D[Module Panic/Error]
    
    C --> E[ProcessControl Recovery]
    D --> F[Panic Recovery]
    
    E --> G[Restart Process]
    F --> H[Restart Module]
    
    G --> I{Success?}
    H --> J{Success?}
    
    I -->|Yes| K[Running]
    I -->|No| L[Failed State]
    J -->|Yes| K
    J -->|No| M[Degraded State]
    
    L --> N[Alert Operations]
    M --> O[Graceful Degradation]
    
    style C fill:#ffebee
    style D fill:#fff3e0
    style K fill:#e8f5e8
```

## Package Management Architecture

### Package Installation Flow

```mermaid
sequenceDiagram
    participant Client as Client
    participant PM as Package Manager
    participant Repo as Repository
    participant FS as File System
    participant Master as Master
    
    Client->>PM: InstallWorkerPackage(request)
    PM->>Repo: GetPackageInfo(workerType, version)
    Repo-->>PM: PackageInfo
    
    alt Process Package
        PM->>Repo: DownloadExecutable()
        Repo-->>PM: executable binary
        PM->>FS: Install to /opt/workers/
    else Module Package
        PM->>Repo: DownloadModule()
        Repo-->>PM: module files
        PM->>FS: Install to /opt/modules/
        PM->>Master: ValidateModuleFactory()
    end
    
    PM->>PM: ValidateInstallation()
    PM-->>Client: Installation Success
```

### Unified Package Support

```mermaid
graph TD
    A[Package Definition] --> B{Supported Types}
    
    B --> C[Process Only]
    B --> D[Module Only]  
    B --> E[Both Process & Module]
    
    C --> F[Executable Binary]
    D --> G[Module Library]
    E --> H[Executable Binary]
    E --> I[Module Library]
    
    F --> J[Process Worker]
    G --> K[Module Worker]
    H --> J
    I --> K
    
    J --> L[Same Service Interface]
    K --> L
    
    style E fill:#e8f5e8
    style L fill:#fff3e0
```

## Runtime Architecture

### Master Internal Architecture

```mermaid
graph TB
    subgraph "Master Core"
        A[Worker Registry]
        B[Lifecycle Manager]
        C[Health Monitor]
        D[Package Manager]
    end
    
    subgraph "Process Workers"
        E[ProcessWorkerController]
        F[ProcessControl]
        G[OS Process]
    end
    
    subgraph "Module Workers"
        H[ModuleWorkerController]
        I[GoroutineManager]
        J[Module Runtime]
        K[Worker Modules]
    end
    
    A --> E
    A --> H
    B --> E
    B --> H
    C --> E
    C --> H
    
    E --> F
    F --> G
    
    H --> I
    I --> J
    J --> K
    
    style A fill:#e3f2fd
    style E fill:#ffebee
    style H fill:#e8f5e8
```

### Module Runtime Components

```mermaid
graph TD
    A[ModuleWorkerController] --> B[GoroutineManager]
    B --> C[ModuleExecution]
    C --> D[Worker Module]
    C --> E[Context & Cancellation]
    C --> F[Panic Recovery]
    
    B --> G[Module Registry]
    G --> H[Factory Functions]
    G --> I[Plugin Loader]
    
    A --> J[Diagnostics Collector]
    J --> K[Health Status]
    J --> L[Performance Metrics]
    J --> M[Error History]
    
    style B fill:#e8f5e8
    style F fill:#fff3e0
    style J fill:#e3f2fd
```

## Deployment Scenarios

### Development vs Production Deployment

```mermaid
graph TB
    subgraph "Development Environment"
        DA[Fast Iteration] --> DB[Module Deployment]
        DB --> DC[In-Process Execution]
        DC --> DD[Shared Memory]
        DD --> DE[Fast Debugging]
    end
    
    subgraph "Production Environment"
        PA[Fault Isolation] --> PB[Process Deployment]
        PB --> PC[Separate Processes]
        PC --> PD[Network Communication]
        PD --> PE[High Availability]
    end
    
    subgraph "Hybrid Environment"
        HA[Critical Services] --> HB[Process Deployment]
        HC[Supporting Services] --> HD[Module Deployment]
        HB --> HE[Maximum Reliability]
        HD --> HF[Resource Efficiency]
    end
    
    style DA fill:#e8f5e8
    style PA fill:#ffebee
    style HA fill:#fff3e0
```

### Scaling Patterns

```mermaid
graph LR
    A[Single Module Instance] --> B[Multiple Module Instances]
    B --> C[Mixed Module/Process]
    C --> D[Process Farm]
    
    A1[High Performance<br/>Shared Memory] --> A
    B1[Horizontal Scale<br/>Same Binary] --> B
    C1[Selective Isolation<br/>Critical Services] --> C
    D1[Maximum Isolation<br/>Independent Processes] --> D
    
    style A fill:#e8f5e8
    style B fill:#fff3e0
    style C fill:#ffebee
    style D fill:#f3e5f5
```

## Error and Recovery Patterns

### Failure Impact Comparison

```mermaid
graph TB
    subgraph "Process Worker Failure"
        PA[Process Crash] --> PB[Isolated Impact]
        PB --> PC[Other Workers Continue]
        PC --> PD[Master Stable]
        PD --> PE[Automatic Restart]
    end
    
    subgraph "Module Worker Failure"
        MA[Module Panic] --> MB[Panic Recovery]
        MB --> MC{Recovery Success?}
        MC -->|Yes| MD[Module Restarts]
        MC -->|No| ME[Shared Impact Risk]
        ME --> MF[Master May Be Affected]
    end
    
    style PB fill:#e8f5e8
    style MB fill:#fff3e0
    style ME fill:#ffebee
```

### Recovery Strategies

```mermaid
flowchart TD
    A[Worker Failure Detected] --> B{Failure Type}
    
    B -->|Process Exit| C[Standard Restart]
    B -->|Module Panic| D[Panic Recovery]
    B -->|Health Check Fail| E[Health-based Recovery]
    
    C --> F[ProcessControl.Restart]
    D --> G[GoroutineManager.Restart]
    E --> H{Worker Type}
    
    H -->|Process| F
    H -->|Module| G
    
    F --> I[New Process]
    G --> J[New Goroutine]
    
    I --> K[Success Check]
    J --> L[Success Check]
    
    K --> M{Success?}
    L --> N{Success?}
    
    M -->|Yes| O[Running State]
    M -->|No| P[Failed State]
    N -->|Yes| O
    N -->|No| Q[Degraded State]
    
    style C fill:#e8f5e8
    style D fill:#fff3e0
    style Q fill:#ffebee
```

---

## Diagram Legend

### Colors
- 🟢 **Green (#e8f5e8)**: Module/In-process components
- 🔵 **Blue (#e3f2fd)**: Master/Core components  
- 🟡 **Yellow (#fff3e0)**: Shared/Common components
- 🔴 **Red (#ffebee)**: Process/External components
- 🟣 **Purple (#f3e5f5)**: Advanced/Optional components

### Shapes
- **Rectangles**: Components/Services
- **Diamonds**: Decision Points
- **Circles**: States
- **Rounded Rectangles**: Processes/Operations

---

## Related Documentation

- [Master Integration](master-integration.md) - Technical implementation details
- [Module Development Guide](module-development-guide.md) - How to create modules
- [Configuration and Usage](configuration-and-usage.md) - Application setup and usage
- [Implementation Guide](implementation-guide.md) - Development phases and considerations