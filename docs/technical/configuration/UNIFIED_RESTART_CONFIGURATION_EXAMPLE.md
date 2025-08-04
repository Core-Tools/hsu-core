# Unified Context-Aware Restart Configuration Guide

This document provides comprehensive examples of how to configure the enhanced context-aware restart system in HSU Master.

## 🎯 **Overview**

The unified restart system provides intelligent, context-aware restart decisions based on:
- **Trigger Type**: Health failure vs resource violation vs manual
- **Severity Level**: Warning, critical, emergency
- **Worker Profile Type**: Batch, web, database, etc. (workload characteristics)
- **Violation Type**: Memory, CPU, health, etc.
- **Time Context**: Startup grace, sustained violations, spikes

## 📋 **Basic Configuration Structure**

```yaml
# Master configuration with unified restart system
restart_circuit_breaker:
  # Default configuration for all restart scenarios
  default:
    policy: "on-failure"
    max_retries: 3
    retry_delay: "30s"
    backoff_rate: 2.0
    
  # Context-specific overrides
  health_failures:
    policy: "on-failure"
    max_retries: 3
    retry_delay: "30s"
    backoff_rate: 2.0
    
  resource_violations:
    policy: "on-failure"
    max_retries: 5        # More lenient for resource issues
    retry_delay: "60s"    # Longer delays for resource issues
    backoff_rate: 1.5     # Gentler backoff
    
  # Severity-based multipliers (applied to max_retries and retry_delay)
  severity_multipliers:
    warning: 0.5    # Half normal limits for warnings
    critical: 1.0   # Normal limits for critical issues
    emergency: 2.0  # Double limits for emergencies
    
  # ✅ UPDATED: Worker profile type multipliers (applied to max_retries and retry_delay)
  worker_profile_multipliers:
    batch: 3.0      # Very lenient for batch processors
    web: 1.0        # Standard for web services
    database: 5.0   # Extremely lenient for databases
    worker: 2.0     # Lenient for background workers
    scheduler: 2.5  # Moderately lenient for schedulers
    default: 1.0    # Standard for unknown types
    
  # Time-based context awareness
  startup_grace_period: "2m"      # No restarts during startup
  sustained_violation_time: "5m"  # Resource violations must be sustained
  spike_tolerance_time: "30s"     # Allow brief resource spikes
```

## 🏭 **Production Web Service Configuration**

```yaml
# Production web service with strict restart policies
workers:
  web-frontend:
    type: "managed"                # ✅ Management type: how the worker is managed
    profile_type: "web"           # ✅ Profile type: workload characteristics for restart policies
    
    unit:
      managed:
        execution:
          command: "./web-server"
          args: ["--port", "8080"]
          
        restart:
          policy: "on-failure"
          max_retries: 3
          retry_delay: "10s"
          backoff_rate: 2.0
          
        health_check:
          type: "http"
          http:
            url: "http://localhost:8080/health"
            method: "GET"
          run_options:
            enabled: true
            interval: "30s"
            timeout: "5s"
            initial_delay: "10s"
            
        limits:
          memory:
            limit_bytes: 512000000  # 512MB
            policy: "restart"       # Restart on memory violations
          cpu:
            limit_percent: 80.0
            policy: "log"          # Only log CPU violations
            
# Result: Web service gets standard restart behavior with quick recovery
# - Health failures: 3 retries, 10s delay, 2.0 backoff (web multiplier: 1.0x)
# - Resource violations: 5 retries (3*1.0 + 2), 20s delay (10s*2), 1.5 backoff
# - Memory violations trigger restarts, CPU violations only log
```

## 🔄 **Batch Processing Service Configuration**

```yaml
# Batch processor with very lenient restart policies
workers:
  batch-processor:
    type: "managed"               # ✅ Management type: managed by HSU Master
    profile_type: "batch"        # ✅ Profile type: batch processing workload
    
    unit:
      managed:
        execution:
          command: "./batch-processor"
          args: ["--input", "/data/input", "--output", "/data/output"]
          
        restart:
          policy: "on-failure"
          max_retries: 2        # Low default (will be multiplied)
          retry_delay: "60s"
          backoff_rate: 1.5
          
        health_check:
          type: "process"       # Simple process existence check
          run_options:
            enabled: true
            interval: "2m"      # Less frequent health checks
            timeout: "10s"
            initial_delay: "30s"
            
        limits:
          memory:
            limit_bytes: 4000000000  # 4GB
            policy: "restart"
          cpu:
            limit_percent: 95.0      # Very high CPU allowed
            policy: "log"            # Only log high CPU
            
# Result: Batch processor gets very lenient restart behavior
# - Health failures: 6 retries (2*3.0), 180s delay (60s*3.0), 1.5 backoff  
# - Resource violations: 11 retries (2*3.0 + 5), 360s delay (60s*2*3.0), 1.5 backoff
# - High CPU usage tolerated, memory violations restart with long delays
```

## 🗄️ **Database Service Configuration**

```yaml
# Critical database with extremely lenient restart policies
workers:
  database:
    type: "managed"               # ✅ Management type: managed by HSU Master
    profile_type: "database"     # ✅ Profile type: database workload requiring high uptime
    
    unit:
      managed:
        execution:
          command: "./database-server"
          args: ["--data-dir", "/var/lib/db", "--port", "5432"]
          
        restart:
          policy: "on-failure"
          max_retries: 1        # Very low default (will be heavily multiplied)
          retry_delay: "120s"   # Long default delay
          backoff_rate: 1.2     # Very gentle backoff
          
        health_check:
          type: "tcp"
          tcp:
            address: "localhost"
            port: 5432
          run_options:
            enabled: true
            interval: "1m"
            timeout: "10s"
            initial_delay: "60s"  # Long startup time
            
        limits:
          memory:
            limit_bytes: 8000000000  # 8GB
            policy: "log"            # Never restart on memory
          cpu:
            limit_percent: 90.0
            policy: "log"            # Never restart on CPU
            
# Result: Database gets extremely lenient restart behavior
# - Health failures: 5 retries (1*5.0), 600s delay (120s*5.0), 1.2 backoff
# - Resource violations: 7 retries (1*5.0 + 2), 1200s delay (120s*2*5.0), 1.5 backoff  
# - Resource violations only log, never restart (uptime prioritized)
```

## ⚠️ **Emergency Severity Example**

```yaml
# Example showing how emergency severity affects restart behavior
workers:
  critical-service:
    type: "managed"
    profile_type: "web"
    
    unit:
      managed:
        restart:
          policy: "on-failure"
          max_retries: 3
          retry_delay: "30s"
          backoff_rate: 2.0
          
        limits:
          memory:
            limit_bytes: 1000000000
            policy: "restart"
            
# When memory violation reaches "emergency" severity:
# - Base config: 3 retries, 30s delay
# - Resource violation multiplier: +2 retries = 5 total
# - Web profile multiplier: 1.0x = 5 retries, 60s delay
# - Emergency severity multiplier: 2.0x = 10 retries, 120s delay
# - Final: 10 retries with 120s initial delay, 2.0 backoff
```

## 🕒 **Time-Based Context Examples**

### **Startup Grace Period**
```yaml
restart_circuit_breaker:
  startup_grace_period: "5m"  # No restarts during first 5 minutes

# During startup (first 5 minutes):
# - All restart requests blocked with: "restart blocked: within startup grace period"
# - Health failures logged but don't trigger restarts
# - Resource violations logged but don't trigger restarts

# After startup grace period:
# - Normal restart logic applies
```

### **Sustained Violation Detection**  
```yaml
restart_circuit_breaker:
  sustained_violation_time: "3m"  # Resource violations must persist 3 minutes

# Resource violation behavior:
# - Brief spikes (< 3 minutes): Logged but ignored
# - Sustained violations (≥ 3 minutes): Trigger restart logic
# - Prevents restarts due to temporary load spikes
```

## 🎛️ **Advanced Scenario Configurations**

### **Development Environment**
```yaml
# Lenient configuration for development
restart_circuit_breaker:
  worker_profile_multipliers:
    default: 10.0      # Very lenient for all workers
    
  severity_multipliers:
    warning: 2.0       # Even warnings get multiple retries
    critical: 5.0      # Critical issues get many retries
    emergency: 10.0    # Emergency issues get extreme retries
    
  startup_grace_period: "10m"  # Long startup grace for debugging
```

### **High-Availability Production**
```yaml
# Aggressive restart for high availability
restart_circuit_breaker:
  default:
    max_retries: 10
    retry_delay: "5s"    # Quick recovery attempts
    backoff_rate: 1.5    # Gentle backoff
    
  health_failures:
    max_retries: 15      # Even more aggressive for health failures
    retry_delay: "3s"
    
  startup_grace_period: "30s"  # Short grace period for production
```

### **Resource-Constrained Environment**
```yaml
# Conservative restart in resource-constrained environment
restart_circuit_breaker:
  resource_violations:
    max_retries: 1       # Very few retries for resource issues
    retry_delay: "5m"    # Long delays to allow resource recovery
    
  severity_multipliers:
    warning: 0.1         # Almost no retries for warnings
    critical: 0.5        # Few retries for critical
    emergency: 1.0       # Normal retries only for emergencies
```

## 📊 **Effective Configuration Calculator**

Here's how the final effective configuration is calculated:

```
Effective Max Retries = Base Max Retries × Worker Profile Multiplier × Severity Multiplier + Context Override

Effective Retry Delay = Base Retry Delay × Worker Profile Multiplier × Severity Multiplier

Examples:
1. Web service, critical health failure:
   - Base: 3 retries, 30s delay
   - Worker profile multiplier (web): 1.0
   - Severity multiplier (critical): 1.0
   - Context override (health): +0
   - Effective: 3 retries, 30s delay

2. Batch processor, emergency resource violation:
   - Base: 2 retries, 60s delay  
   - Worker profile multiplier (batch): 3.0
   - Severity multiplier (emergency): 2.0
   - Context override (resource): +2
   - Effective: 14 retries [(2×3.0×2.0)+2], 360s delay [60×3.0×2.0]

3. Database, warning health failure:
   - Base: 1 retry, 120s delay
   - Worker profile multiplier (database): 5.0  
   - Severity multiplier (warning): 0.5
   - Context override (health): +0
   - Effective: 3 retries [1×5.0×0.5 = 2.5 → 3], 30s delay [120×5.0×0.5]
```

## 🏆 **Best Practices**

### **Worker Profile Type Assignment**
- **`batch`**: Long-running data processing, ETL jobs, ML training
- **`web`**: HTTP servers, API gateways, frontend services
- **`database`**: Database servers, caches, persistent storage
- **`worker`**: Background job processors, queue workers
- **`scheduler`**: Cron-like schedulers, orchestrators
- **`default`**: Unknown or generic services

### **Management Type vs Profile Type**
- **Management Type** (`type` field): How the worker is managed by HSU Master
  - `managed`: HSU Master controls the process lifecycle
  - `unmanaged`: HSU Master monitors existing processes
  - `integrated`: HSU Master provides integrated services
  
- **Profile Type** (`profile_type` field): Worker's workload characteristics for restart policies
  - Determines restart behavior based on expected resource usage patterns
  - Used for context-aware circuit breaker decisions

### **Severity Classification**  
- **`warning`**: Minor issues, temporary degradation
- **`critical`**: Significant issues affecting functionality
- **`emergency`**: Severe issues requiring immediate attention

### **Policy Recommendations**
- **Production Web Services**: Standard multipliers, quick recovery
- **Batch Processing**: High multipliers, tolerate resource spikes
- **Databases**: Very high multipliers, prioritize uptime
- **Development**: Maximum tolerance, long grace periods
- **Resource-Constrained**: Conservative approach, long delays

This unified system provides the flexibility to handle diverse operational requirements while maintaining consistent, predictable restart behavior across your infrastructure. 