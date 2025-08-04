# Worker Package Management Architecture

## Overview

This document outlines the architectural design for dynamic worker package management in the HSU Master system. This enhancement addresses the need for on-demand installation, update, and management of worker executables.

## Problem Statement

Currently, the HSU Master assumes worker executables are pre-installed on the system. When a worker fails to start due to missing executables, there's no mechanism to:
- Detect the specific failure reason
- Automatically install missing worker packages
- Manage worker package versions and updates
- Handle worker package dependencies

## Architecture Principles

### 1. Separation of Concerns
- **HSU Master**: Remains focused on process lifecycle management
- **Package Manager**: Handles package installation, updates, and dependency management
- **Clear interfaces**: Well-defined boundaries between components

### 2. Modular Design
- Package management is a separate, pluggable component
- Multiple package manager implementations (apt, yum, docker, custom)
- Master can operate with or without package management capabilities

### 3. Progressive Enhancement
- Existing functionality remains unchanged
- Package management is an optional enhancement
- Backward compatibility with current worker deployment methods

## Component Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    HSU Master System                        │
├─────────────────────────────────────────────────────────────┤
│  ┌───────────────┐    ┌─────────────────┐    ┌────────────┐ │
│  │   HSU Master  │    │  Worker Package │    │  Package   │ │
│  │ (Process Mgr) │◄──►│    Manager      │◄──►│ Repository │ │
│  │               │    │                 │    │            │ │
│  └───────────────┘    └─────────────────┘    └────────────┘ │
│           │                     │                           │
│           ▼                     ▼                           │
│  ┌───────────────┐    ┌─────────────────┐                  │
│  │ProcessControl │    │ Installation    │                  │
│  │   Enhanced    │    │ Status Tracker  │                  │
│  └───────────────┘    └─────────────────┘                  │
└─────────────────────────────────────────────────────────────┘
```

## Interface Definitions

### 1. Worker Package Manager Interface

```go
package packagemanager

import (
    "context"
    "time"
)

// WorkerPackageManager handles worker package lifecycle
type WorkerPackageManager interface {
    // Package availability
    IsPackageInstalled(workerType string, version string) (bool, error)
    GetInstalledPackages() ([]PackageInfo, error)
    GetAvailableVersions(workerType string) ([]string, error)
    
    // Installation management
    InstallPackage(ctx context.Context, req InstallRequest) error
    UpdatePackage(ctx context.Context, req UpdateRequest) error
    UninstallPackage(ctx context.Context, workerType string) error
    
    // Status tracking
    GetInstallationStatus(workerType string) (InstallationStatus, error)
    
    // Health and validation
    ValidatePackage(workerType string, version string) error
    GetPackageHealth(workerType string) (PackageHealth, error)
}

// PackageInfo describes an installed package
type PackageInfo struct {
    WorkerType    string
    Version       string
    InstallPath   string
    InstallDate   time.Time
    Dependencies  []Dependency
    Checksum      string
}

// InstallRequest contains package installation parameters
type InstallRequest struct {
    WorkerType      string
    Version         string  // empty for latest
    Repository      string  // optional custom repository
    InstallTimeout  time.Duration
    AllowDowngrade  bool
    Force           bool    // bypass safety checks
}

// UpdateRequest contains package update parameters  
type UpdateRequest struct {
    WorkerType     string
    TargetVersion  string  // empty for latest
    UpdateTimeout  time.Duration
    BackupCurrent  bool
}

// InstallationStatus tracks installation progress
type InstallationStatus struct {
    State           InstallationState
    WorkerType      string
    Version         string
    Progress        float64  // 0.0 to 1.0
    LastUpdated     time.Time
    Error           error
    EstimatedTime   time.Duration
}

type InstallationState string

const (
    InstallationStateNotInstalled InstallationState = "not_installed"
    InstallationStateInstalling   InstallationState = "installing"
    InstallationStateInstalled    InstallationState = "installed"
    InstallationStateUpdating     InstallationState = "updating"
    InstallationStateFailed       InstallationState = "failed"
    InstallationStateCorrupted    InstallationState = "corrupted"
)

// PackageHealth provides package health information
type PackageHealth struct {
    IsHealthy       bool
    ExecutablePath  string
    IsExecutable    bool
    ChecksumValid   bool
    DependenciesMet bool
    Issues          []string
}

// Dependency represents a package dependency
type Dependency struct {
    Name     string
    Version  string
    Required bool
}
```

### 2. Enhanced Master Interface

```go
// Enhanced Master methods for package management
func (m *Master) SetPackageManager(pm packagemanager.WorkerPackageManager)
func (m *Master) CheckWorkerPackageAvailability(workerType string) (bool, error)
func (m *Master) InstallWorkerPackage(ctx context.Context, req packagemanager.InstallRequest) error
func (m *Master) GetWorkerPackageStatus(workerType string) (packagemanager.InstallationStatus, error)
func (m *Master) GetInstalledWorkerPackages() ([]packagemanager.PackageInfo, error)

// Enhanced worker addition with package checking
func (m *Master) AddWorkerWithPackageCheck(worker workers.Worker, options AddWorkerOptions) error

type AddWorkerOptions struct {
    AutoInstall      bool
    InstallTimeout   time.Duration
    AllowDowngrade   bool
    RequiredVersion  string
}
```

## Implementation Strategy

### Phase 1: Foundation (Weeks 1-2)
1. **Define interfaces** in new `pkg/packagemanager` package
2. **Create mock implementation** for testing
3. **Add package manager integration points** in Master
4. **Update ProcessControl** to provide better error categorization

### Phase 2: Core Implementation (Weeks 3-4)
1. **Implement basic package managers**:
   - File-based package manager (for development)
   - Docker-based package manager
2. **Add installation status tracking**
3. **Implement package health checking**
4. **Add retry mechanisms with installation**

### Phase 3: Advanced Features (Weeks 5-6)
1. **Repository management**
2. **Dependency resolution**
3. **Package signing and validation**
4. **Installation progress tracking**

### Phase 4: Production Readiness (Weeks 7-8)
1. **Production package managers** (apt, yum, etc.)
2. **Comprehensive error handling**
3. **Performance optimization**
4. **Security hardening**

## Package Manager Implementations

### 1. Docker Package Manager
```go
type DockerPackageManager struct {
    client      docker.Client
    registry    string
    namespace   string
    localCache  string
}

// Pulls worker images and manages containers
```

### 2. System Package Manager
```go
type SystemPackageManager struct {
    packageManager string  // "apt", "yum", "dnf"
    repository     string
    installPrefix  string
}

// Uses system package managers
```

### 3. Binary Package Manager
```go
type BinaryPackageManager struct {
    repository   string
    installDir   string
    checksumMode ChecksumMode
}

// Downloads and installs binary packages
```

## Integration Workflow

### 1. Worker Addition with Auto-Installation
```go
// Client code
options := master.AddWorkerOptions{
    AutoInstall:     true,
    InstallTimeout:  5 * time.Minute,
    RequiredVersion: "1.2.3",
}

err := master.AddWorkerWithPackageCheck(worker, options)
if err != nil {
    // Handle installation failure
}
```

### 2. Manual Package Management
```go
// Check installation status
status, err := master.GetWorkerPackageStatus("data-processor")

if status.State == packagemanager.InstallationStateNotInstalled {
    // Install package
    req := packagemanager.InstallRequest{
        WorkerType: "data-processor",
        Version:    "latest",
    }
    err = master.InstallWorkerPackage(ctx, req)
}
```

### 3. Package Health Monitoring
```go
// Regular health checks
packages, _ := master.GetInstalledWorkerPackages()
for _, pkg := range packages {
    health, _ := packageManager.GetPackageHealth(pkg.WorkerType)
    if !health.IsHealthy {
        // Handle unhealthy package
    }
}
```

## Security Considerations

### 1. Package Validation
- **Checksum verification** for all downloaded packages
- **Digital signature validation** for trusted sources
- **Sandboxed installation** to prevent system compromise

### 2. Access Control
- **Repository authentication** for private packages
- **Installation permissions** to prevent unauthorized installations
- **Audit logging** for all package operations

### 3. Isolation
- **User-space installations** when possible
- **Containerized execution** for untrusted packages
- **Resource limits** during installation

## Configuration

### 1. Package Manager Configuration
```yaml
package_manager:
  type: "docker"  # docker, system, binary
  config:
    registry: "your-registry.com"
    namespace: "workers"
    cache_dir: "/opt/hsu/cache"
    
  repositories:
    - name: "official"
      url: "https://packages.hsu.io/workers"
      trusted: true
    - name: "internal"
      url: "https://internal.company.com/packages"
      auth_required: true

  policies:
    auto_update: false
    allow_downgrades: false
    installation_timeout: "10m"
    max_concurrent_installs: 3
```

### 2. Worker Package Metadata
```yaml
# worker-package.yaml
name: "data-processor"
version: "1.2.3"
description: "High-performance data processing worker"

executable:
  path: "bin/data-processor"
  args: ["--config", "config.yaml"]
  
dependencies:
  - name: "libprocessor"
    version: ">=2.1.0"
  - name: "python3"
    version: ">=3.8"

resources:
  min_memory: "512MB"
  min_disk: "1GB"
  
health_check:
  endpoint: "/health"
  interval: "30s"
```

## Monitoring and Observability

### 1. Package Installation Metrics
- Installation success/failure rates
- Installation duration
- Package download sizes and times
- Dependency resolution times

### 2. Package Health Metrics
- Package corruption detection
- Executable availability
- Dependency status
- Health check results

### 3. Logging and Tracing
- Structured logging for all package operations
- Installation progress tracking
- Error categorization and analysis
- Audit trails for security compliance

## Future Considerations

### 1. Advanced Features
- **Rollback capabilities** for failed updates
- **Blue-green deployments** for worker updates
- **A/B testing** for worker versions
- **Canary deployments** for gradual rollouts

### 2. Integration Points
- **CI/CD pipeline integration** for automated deployments
- **Configuration management** integration
- **Monitoring system** integration
- **Alert management** for package issues

### 3. Scalability
- **Distributed package caching** for large deployments
- **Parallel installation** support
- **Delta updates** for efficient bandwidth usage
- **CDN integration** for global distribution

## Conclusion

This architecture provides a robust foundation for dynamic worker package management while maintaining the HSU Master's core focus on process lifecycle management. The modular design allows for gradual adoption and supports multiple deployment scenarios from development to production.

The separation of concerns ensures that package management complexity doesn't interfere with the Master's primary responsibilities, while the well-defined interfaces allow for flexible implementation strategies based on specific deployment requirements.