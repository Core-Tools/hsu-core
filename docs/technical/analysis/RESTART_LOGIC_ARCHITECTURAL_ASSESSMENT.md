# Restart Logic Architectural Assessment: Inconsistencies and Unified Solution

## 🔍 **Current State Analysis**

After thorough review of the restart logic across `process_control_impl.go`, `restart_circuit_breaker.go`, `health_check.go`, and `resourcelimits/manager.go`, several **critical inconsistencies** have been identified that create architectural complexity and potential conflicts.

## ⚠️ **Identified Inconsistencies**

### **1. Configuration Asymmetry**
```go
// ✅ Health Monitor: CONSUMES restart config
NewHealthMonitorWithRestart(config, id, processInfo, restartPolicy, logger)

// ❌ Resource Limits Manager: NO restart config  
NewResourceLimitManager(pid, limits, logger) // Missing restart policy!
```

**Impact**: Different restart behavior for health vs resource violations, despite both using the same circuit breaker.

### **2. Duplicate Retry Logic in Health Check**

```go
// health_check.go lines 360-364: OWN retry logic
if h.restartPolicy.MaxRetries > 0 && h.state.Retries >= h.restartPolicy.MaxRetries {
    h.logger.Errorf("Max restart retries exceeded, id: %s, retries: %d, max: %d",
        h.id, h.state.Retries, h.restartPolicy.MaxRetries)
    return // ❌ BLOCKS restart before circuit breaker even sees it!
}

// health_check.go line 367: OWN retry counter
h.state.Retries++ // ❌ SEPARATE from circuit breaker counter

// process_control_impl.go lines 304: THEN goes through circuit breaker  
return pc.restartCircuitBreaker.ExecuteRestart(wrappedRestart) // ❌ REDUNDANT retry logic!
```

### **3. Conflicting Retry Counters**

```go
// Health Check Counter
h.state.Retries++                    // ❌ Health monitor's own counter

// Circuit Breaker Counter  
rcb.restartAttempts++                // ❌ Circuit breaker's separate counter
```

**Problem**: Two independent retry counters that can diverge, leading to inconsistent restart behavior.

### **4. Resource Limits vs Health Check Asymmetry**

```go
// Resource Violation: RELIES ONLY on circuit breaker
if pc.restartCircuitBreaker != nil {
    return pc.restartCircuitBreaker.ExecuteRestart(wrappedRestart) // ✅ Clean path
}

// Health Check: DUPLICATE logic THEN circuit breaker
if h.restartPolicy.MaxRetries > 0 && h.state.Retries >= h.restartPolicy.MaxRetries {
    return // ❌ Blocks circuit breaker logic
}
// ... then later calls circuit breaker anyway
```

**Result**: Resource violations get sophisticated circuit breaker logic, health failures get limited by primitive retry counting.

### **5. Sophisticated vs Primitive Logic Conflict**

**Health Check (Primitive)**:
```go
// Simple retry counting, no backoff, no timing intelligence
if h.state.Retries >= h.restartPolicy.MaxRetries {
    return // ❌ Blocks restart permanently
}
```

**Circuit Breaker (Sophisticated)**:
```go
// Exponential backoff, timing delays, circuit breaker state management
retryDelay := time.Duration(float64(retryDelay) * backoffMultiplier)
if timeSinceLastRestart < retryDelay {
    // ✅ Intelligent timing-based retry logic
}
```

**Interference**: Health check's primitive logic can block circuit breaker's sophisticated logic from executing.

## 🎯 **Proposed Unified Solution**

### **Phase 1: Remove Health Check Restart Logic**

**Goal**: Eliminate duplicate retry logic, make circuit breaker the single source of truth.

#### **Changes to `health_check.go`**:

```go
// ❌ REMOVE: checkRestartCondition's internal retry logic
func (h *healthMonitor) checkRestartCondition(message string) {
    // ❌ REMOVE: h.restartPolicy.MaxRetries checking
    // ❌ REMOVE: h.state.Retries management  
    // ✅ KEEP: Only policy-based shouldRestart logic
    
    shouldRestart := h.shouldRestartBasedOnPolicy()
    if shouldRestart && h.restartCallback != nil {
        go h.restartCallback(fmt.Sprintf("Health check failure: %s", message))
    }
}

// ✅ NEW: Simplified policy checking (no retry counting)
func (h *healthMonitor) shouldRestartBasedOnPolicy() bool {
    switch h.restartPolicy.Policy {
    case RestartAlways:
        return true
    case RestartOnFailure:
        return h.state.Status == HealthCheckStatusUnhealthy
    case RestartUnlessStopped:
        return h.state.Status == HealthCheckStatusUnhealthy
    case RestartNever:
        return false
    }
    return false
}
```

#### **Changes to Health Monitor Creation**:

```go
// ❌ REMOVE: RestartConfig from health monitor
// ✅ KEEP: Policy-only configuration for shouldRestart logic
func NewHealthMonitorWithRestartPolicy(config *HealthCheckConfig, id string, processInfo *ProcessInfo, restartPolicy RestartPolicy, logger logging.Logger) HealthMonitor {
    return &healthMonitor{
        config:        config,
        // ... other fields
        restartPolicyOnly: restartPolicy, // ✅ Policy only, no MaxRetries/RetryDelay
    }
}
```

### **Phase 2: Enhanced Circuit Breaker**

**Goal**: Make circuit breaker context-aware and sophisticated enough to handle all restart scenarios.

#### **Enhanced Circuit Breaker Interface**:

```go
type RestartContext struct {
    TriggerType   RestartTriggerType // health_failure, resource_violation, manual
    Severity      string             // warning, critical, emergency  
    WorkerType    string             // batch, web, database, etc.
    ViolationType string             // memory, cpu, health, etc.
}

type RestartTriggerType string
const (
    RestartTriggerHealthFailure   RestartTriggerType = "health_failure"
    RestartTriggerResourceViolation RestartTriggerType = "resource_violation"
    RestartTriggerManual         RestartTriggerType = "manual"
)

type ContextAwareRestartConfig struct {
    // Default behavior
    Default RestartConfig `yaml:"default"`
    
    // Context-specific overrides
    HealthFailures     *RestartConfig `yaml:"health_failures,omitempty"`
    ResourceViolations *RestartConfig `yaml:"resource_violations,omitempty"`
    
    // Severity-based scaling
    SeverityMultipliers map[string]float64 `yaml:"severity_multipliers,omitempty"`
    
    // Worker-type awareness
    WorkerTypeMultipliers map[string]float64 `yaml:"worker_type_multipliers,omitempty"`
    
    // Time-based context
    StartupGracePeriod      time.Duration `yaml:"startup_grace_period,omitempty"`
    SustainedViolationTime  time.Duration `yaml:"sustained_violation_time,omitempty"`
    SpikeToleranceTime      time.Duration `yaml:"spike_tolerance_time,omitempty"`
}

type EnhancedRestartCircuitBreaker interface {
    ExecuteRestart(restartFunc RestartFunc, context RestartContext) error
    Reset()
    GetState() CircuitBreakerState
}
```

#### **Enhanced Circuit Breaker Implementation**:

```go
func (rcb *enhancedRestartCircuitBreaker) ExecuteRestart(restartFunc RestartFunc, context RestartContext) error {
    // ✅ Get context-appropriate config
    config := rcb.getConfigForContext(context)
    
    // ✅ Apply severity and worker-type multipliers
    effectiveMaxRetries := rcb.applyMultipliers(config.MaxRetries, context)
    effectiveRetryDelay := rcb.applyMultipliers(config.RetryDelay, context)
    
    // ✅ Check startup grace period
    if rcb.isInStartupGracePeriod() {
        return fmt.Errorf("restart blocked: within startup grace period")
    }
    
    // ✅ Check for sustained violations (resource context)
    if context.TriggerType == RestartTriggerResourceViolation {
        if !rcb.isSustainedViolation(context) {
            return fmt.Errorf("restart blocked: violation not sustained long enough")
        }
    }
    
    // ✅ Existing sophisticated logic with context-aware parameters
    return rcb.executeWithConfig(restartFunc, effectiveMaxRetries, effectiveRetryDelay, config)
}

func (rcb *enhancedRestartCircuitBreaker) getConfigForContext(context RestartContext) *RestartConfig {
    switch context.TriggerType {
    case RestartTriggerHealthFailure:
        if rcb.config.HealthFailures != nil {
            return rcb.config.HealthFailures
        }
    case RestartTriggerResourceViolation:
        if rcb.config.ResourceViolations != nil {
            return rcb.config.ResourceViolations
        }
    }
    return &rcb.config.Default
}
```

### **Phase 3: Integrated Usage**

#### **Process Control Integration**:

```go
// ✅ Health Monitor Callback (simplified)
healthMonitor.SetRestartCallback(func(reason string) error {
    context := RestartContext{
        TriggerType: RestartTriggerHealthFailure,
        Severity:    "critical", // Based on health status
        WorkerType:  pc.workerType, // From worker configuration
    }
    return pc.restartCircuitBreaker.ExecuteRestart(wrappedRestart, context)
})

// ✅ Resource Violation Callback (enhanced)
func (pc *processControl) handleResourceViolation(policy resourcelimits.ResourcePolicy, violation *resourcelimits.ResourceViolation) {
    context := RestartContext{
        TriggerType:   RestartTriggerResourceViolation,
        Severity:      string(violation.Severity), // warning/critical/emergency
        WorkerType:    pc.workerType,
        ViolationType: string(violation.LimitType), // memory/cpu/process
    }
    return pc.restartCircuitBreaker.ExecuteRestart(wrappedRestart, context)
}
```

## 📋 **Configuration Example**

```yaml
# Unified restart configuration
restart_circuit_breaker:
  default:
    max_retries: 3
    retry_delay: 30s
    backoff_rate: 2.0
    
  # Health failures: standard behavior
  health_failures:
    max_retries: 3
    retry_delay: 30s
    backoff_rate: 2.0
    
  # Resource violations: more lenient
  resource_violations:
    max_retries: 5
    retry_delay: 60s
    backoff_rate: 1.5
    
  # Severity-based multipliers
  severity_multipliers:
    warning: 0.5      # Half normal limits
    critical: 1.0     # Normal limits  
    emergency: 2.0    # Double limits
    
  # Worker-type multipliers
  worker_type_multipliers:
    batch_processor: 3.0     # Very lenient
    web_frontend: 1.0        # Standard
    database: 5.0            # Extremely lenient
    
  # Context awareness
  startup_grace_period: 2m
  sustained_violation_time: 5m
  spike_tolerance_time: 30s
```

## 🏆 **Benefits of Unified Approach**

### **1. Elimination of Conflicts** ✅
- Single source of truth for restart logic
- No competing retry counters
- Consistent behavior across trigger types

### **2. Enhanced Intelligence** 🧠
- Context-aware restart decisions
- Severity and worker-type consideration
- Time-based intelligence (startup grace, sustained violations)

### **3. Simplified Architecture** 🏗️
- Health monitor focuses on health detection only
- Resource limits focus on violation detection only
- Circuit breaker handles ALL restart complexity

### **4. Better User Experience** 👥
- Predictable restart behavior
- Fine-grained configuration control
- Appropriate responses to different scenarios

### **5. Maintainability** 🔧
- Single place to modify restart logic
- Clear separation of concerns
- Easier testing and debugging

## 🚀 **Implementation Roadmap**

### **Step 1: Health Check Simplification**
- Remove retry logic from health monitor
- Keep only restart policy evaluation
- Update health monitor creation interfaces

### **Step 2: Circuit Breaker Enhancement**
- Add RestartContext parameter
- Implement context-aware configuration
- Add time-based intelligence

### **Step 3: Integration Update**
- Update ProcessControl to use context-aware circuit breaker
- Add worker type and severity context
- Update configuration structures

### **Step 4: Documentation & Testing**
- Update configuration documentation
- Add comprehensive tests for context-aware logic
- Create migration guide for existing configurations

## 💡 **Conclusion**

The current dual-retry-logic approach creates unnecessary complexity and potential conflicts. By **consolidating all restart intelligence into an enhanced circuit breaker**, we achieve:

- **Architectural simplicity** through single responsibility
- **Enhanced functionality** through context awareness  
- **Consistent behavior** across all restart triggers
- **Better user control** through sophisticated configuration

This unified approach transforms restart logic from a **fragmented, conflicting system** into a **coherent, intelligent, context-aware restart management system** that can appropriately handle the philosophical differences between health failures and resource violations while maintaining consistent, predictable behavior.

**Recommendation**: Proceed with this unified approach to eliminate architectural inconsistencies and create a superior restart management system. 