# Unified Worker Architecture: Location-Transparent Services

## Overview

This document set outlines the architectural design for unified worker management in the HSU Master system, enabling location-transparent services that can be deployed either as out-of-process workers or in-process modules with the same interface and lifecycle management.

## Problem Statement

Currently, services/modules in distributed systems must be designed differently depending on their deployment model:
- **Out-of-process**: Separate executables with process isolation and network communication
- **In-process**: Compiled/linked code with function calls and shared memory

This creates:
- **Development complexity**: Different code paths for different deployment scenarios
- **Operational overhead**: Different management approaches for process vs module deployment
- **Migration friction**: Difficulty switching between deployment models
- **Testing challenges**: Different test environments for different deployment types

## Vision: Location-Transparent Services

### Core Principle
Services should be designed **once** and deployed **anywhere** - the deployment decision (in-process vs out-of-process) should be a **configuration choice**, not a design constraint.

### Core Insight
**Deployment location should be a configuration decision, not a design constraint.**

The native module approach achieves this by:
1. **Compile-time Integration**: Modules are regular Go packages compiled into the binary
2. **Runtime Registration**: Simple factory pattern connects modules to Master
3. **Configuration-Driven**: YAML determines whether services run in-process or out-of-process
4. **Unified Management**: Same lifecycle, monitoring, and error handling for all worker types

## Architecture Principles

### 1. Location Transparency
- **Single Interface**: Same service interface regardless of deployment location
- **Unified Lifecycle**: Same start/stop/health operations for all worker types
- **Protocol Consistency**: Same communication protocols (gRPC, HTTP) for both deployments

### 2. Deployment Flexibility
- **Runtime Decision**: Choose deployment model via configuration
- **Either/Or Approach**: Simple binary choice - process OR module
- **No Hybrid Complexity**: Avoid complex mode-switching or fallback mechanisms

### 3. Acceptable Constraints
- **Shared Fate for Modules**: In-process modules can crash the Master (documented limitation)
- **No Resource Limits**: In-process modules cannot be resource-limited (like unmanaged workers)
- **Restart Required**: No hot-swapping; deployment changes require restart
- **Communication Scope**: Leave service-to-service communication protocols out of scope

### 4. Progressive Enhancement
- **Backward Compatibility**: Existing process workers continue to work unchanged
- **Incremental Adoption**: Add module support without affecting existing functionality
- **Evolutionary Path**: Start simple, evolve based on real-world usage

## Document Organization

### 📋 For Module Authors
- **[Module Development Guide](module-development-guide.md)** - How to implement WorkerModule interface, compilation models, examples

### 🏗️ For Platform Developers  
- **[Master Integration](master-integration.md)** - Enhanced Master interface, WorkerController abstraction, runtime components
- **[Implementation Guide](implementation-guide.md)** - Implementation phases, technical details, constraints

### 🎮 For Application Developers
- **[Configuration and Usage](configuration-and-usage.md)** - YAML configuration, usage examples, package management

### 📊 Reference Materials
- **[Diagrams](diagrams.md)** - Architecture diagrams and state machines
- **[Developer Experience](developer-experience.md)** - Benefits analysis, migration strategies, best practices
- **[Client Experience](client-experience.md)** - How clients interact with modules, location transparency solution

## Quick Start

### For Module Authors
1. Implement the `WorkerModule` interface (4 simple methods)
2. Provide a factory function: `func NewMyService() worker.WorkerModule`
3. Use the same protocols (gRPC, HTTP) regardless of deployment

### For Application Authors  
1. Import modules as regular Go packages
2. Register factories at startup: `master.RegisterNativeModuleFactory("service", factory)`
3. Configure deployment via YAML: `type: "module"` or `type: "process"`

### Key Benefits
- ✅ **True Location Transparency**: Same service code works in-process or out-of-process
- ✅ **Go-Idiomatic Development**: Just import packages and register factories
- ✅ **Zero Complexity Overhead**: No dynamic loading, plugins, or special build requirements
- ✅ **Operational Consistency**: Same Master APIs, monitoring, and lifecycle management

## Architecture Impact

**This architecture transforms HSU Master from a process manager into a unified service orchestrator that can elegantly handle both traditional process workers and modern in-process modules with the same simplicity and power.**

---

## Navigation

| Document | Purpose | Audience |
|----------|---------|----------|
| [Module Development Guide](module-development-guide.md) | How to build modules | Module Authors |
| [Master Integration](master-integration.md) | Core platform changes | Platform Developers |
| [Configuration and Usage](configuration-and-usage.md) | How to use the system | Application Developers |
| [Client Experience](client-experience.md) | How clients interact with modules | Client Developers |
| [Implementation Guide](implementation-guide.md) | How to build it | Platform Developers |
| [Diagrams](diagrams.md) | Visual architecture | All |
| [Developer Experience](developer-experience.md) | Benefits and migration | All |

---

*For questions or contributions to this architecture, please refer to the individual documents or contact the HSU Master development team.*