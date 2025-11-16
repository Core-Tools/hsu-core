//! ProcessControlImpl - Implementation of ProcessControl trait
//!
//! This module contains the core implementation of process lifecycle control,
//! encapsulating complex logic for:
//! - Process spawning and termination
//! - Health check integration
//! - Resource monitoring
//! - Restart policies and circuit breaker
//! - State machine management
//!
//! This implementation mirrors Go's `pkg/managedprocess/processcontrolimpl/process_control_impl.go`

use hsu_managed_process::{
    ProcessControl, ProcessControlConfig, ProcessDiagnostics, ProcessError,
    ErrorCategory,
};
use async_trait::async_trait;
use hsu_common::{ProcessError as CommonProcessError, ProcessResult};
// use hsu_process::process_exists; // Not used in this module
use hsu_process_state::{ProcessState, ProcessStateMachine};
use crate::config::ProcessConfig;
use crate::lifecycle::ProcessLifecycleManager;
use hsu_monitoring::{HealthStatus, check_http_health};
use hsu_resource_limits::{
    ResourceMonitor, ResourceUsage,
    check_resource_limits_with_policy, ResourcePolicy, ResourceViolation,
};
use std::sync::Arc;
use tokio::process::{Child, Command};
use tokio::sync::RwLock;
use tokio::time::{timeout, interval, Duration};
use tracing::{debug, error, info, warn};

/// Process control implementation with full lifecycle management
pub struct ProcessControlImpl {
    /// Process configuration
    config: ProcessConfig,
    
    /// Control configuration
    control_config: ProcessControlConfig,
    
    /// State machine for process lifecycle
    state_machine: ProcessStateMachine,
    
    /// Lifecycle manager for restart policies
    lifecycle_manager: ProcessLifecycleManager,
    
    /// Child process handle (if spawned)
    child: Option<Child>,
    
    /// Current process ID (if running)
    pid: Option<u32>,
    
    /// Process start time
    start_time: Option<chrono::DateTime<chrono::Utc>>,
    
    /// Restart counter
    restart_count: u32,
    
    /// Last restart time
    last_restart_time: Option<chrono::DateTime<chrono::Utc>>,
    
    /// Health status tracking
    health_status: Arc<RwLock<HealthStatus>>,
    
    /// Health check background task
    health_check_task: Option<tokio::task::JoinHandle<()>>,
    
    /// Resource usage tracking
    resource_usage: Arc<RwLock<ResourceUsage>>,
    
    /// Resource monitor
    resource_monitor: Arc<ResourceMonitor>,
    
    /// Resource monitoring background task
    resource_monitor_task: Option<tokio::task::JoinHandle<()>>,
    
    /// Channel for sending resource violation events
    violation_tx: Option<tokio::sync::mpsc::UnboundedSender<(ResourcePolicy, ResourceViolation)>>,
    
    /// Resource violation handler task
    violation_handler_task: Option<tokio::task::JoinHandle<()>>,
    
    /// Last error (for diagnostics)
    last_error: Option<ProcessError>,
}

impl ProcessControlImpl {
    /// Create a new process control implementation
    pub fn new(
        config: ProcessConfig,
        control_config: ProcessControlConfig,
    ) -> Self {
        let process_id = config.id.clone();
        let state_machine = ProcessStateMachine::new(&process_id);
        let lifecycle_manager = ProcessLifecycleManager::new(
            process_id.clone(),
            config.management.restart_policy.clone(),
        );
        
        Self {
            config,
            control_config,
            state_machine,
            lifecycle_manager,
            child: None,
            pid: None,
            start_time: None,
            restart_count: 0,
            last_restart_time: None,
            health_status: Arc::new(RwLock::new(HealthStatus::new())),
            health_check_task: None,
            resource_usage: Arc::new(RwLock::new(ResourceUsage {
                cpu_percent: None,
                memory_mb: None,
                file_descriptors: None,
                network_connections: None,
            })),
            resource_monitor: Arc::new(ResourceMonitor::new()),
            resource_monitor_task: None,
            violation_tx: None,
            violation_handler_task: None,
            last_error: None,
        }
    }
    
    /// Spawn a new child process
    async fn spawn_process(&mut self) -> ProcessResult<()> {
        info!("Spawning process: {}", self.config.id);
        
        // Build command
        let mut cmd = Command::new(&self.config.management.control.executable);
        cmd.args(&self.config.management.control.arguments);
        
        if let Some(ref wd) = self.config.management.control.working_directory {
            cmd.current_dir(wd);
        }
        
        for (key, value) in &self.config.management.control.environment {
            cmd.env(key, value);
        }
        
        cmd.stdout(std::process::Stdio::piped())
            .stderr(std::process::Stdio::piped())
            .stdin(std::process::Stdio::null());
        
        // Spawn the process (spawn is synchronous, returns Result)
        match cmd.spawn() {
            Ok(child) => {
                let pid = child.id().unwrap_or(0);
                self.child = Some(child);
                self.pid = Some(pid);
                self.start_time = Some(chrono::Utc::now());
                
                info!("Process spawned successfully: {} (PID: {})", self.config.id, pid);
                Ok(())
            }
            Err(e) => {
                let err = ProcessError::new(
                    ErrorCategory::ExecutableNotFound,
                    format!("Failed to spawn process: {}", e),
                    false,
                );
                self.last_error = Some(err.clone());
                
                Err(CommonProcessError::SpawnFailed {
                    id: self.config.id.clone(),
                    reason: err.details,
                })
            }
        }
    }
    
    /// Terminate the running process
    async fn terminate_process(&mut self) -> ProcessResult<()> {
        info!("Terminating process: {}", self.config.id);
        
        if let Some(ref mut child) = self.child {
            // Try graceful shutdown first
            let shutdown_timeout = self.config.management.control.shutdown_timeout;
            
            if let Err(_e) = timeout(shutdown_timeout, child.wait()).await {
                warn!("Graceful shutdown timed out for {}, force killing", self.config.id);
                
                // Force kill
                if let Err(kill_err) = child.kill().await {
                    error!("Failed to kill process {}: {}", self.config.id, kill_err);
                    return Err(CommonProcessError::Configuration {
                        id: self.config.id.clone(),
                        reason: format!("Failed to kill process: {}", kill_err),
                    });
                }
            }
            
            // Clean up state
            self.child = None;
            self.pid = None;
            
            info!("Process terminated: {}", self.config.id);
            Ok(())
        } else {
            debug!("No child process to terminate for {}", self.config.id);
            Ok(())
        }
    }
    
    /// Spawn health check background task
    fn spawn_health_check_task(&mut self) {
        let Some(ref health_config) = self.config.management.health_check else {
            return;
        };
        
        if !health_config.enabled {
            return;
        }
        
        let Some(ref endpoint) = health_config.http_endpoint else {
            return;
        };
        
        info!("Spawning health check task for {}", self.config.id);
        
        let process_id = self.config.id.clone();
        let endpoint = endpoint.clone();
        let check_interval = health_config.interval;
        let timeout_duration = health_config.timeout;
        let failure_threshold = health_config.failure_threshold;
        let health_status = Arc::clone(&self.health_status);
        
        let task = tokio::spawn(async move {
            let mut interval_timer = interval(check_interval);
            
            loop {
                interval_timer.tick().await;
                
                debug!("Running health check for {}", process_id);
                
                match check_http_health(&endpoint, timeout_duration).await {
                    Ok(data) => {
                        let mut status = health_status.write().await;
                        if data.is_healthy {
                            status.record_success();
                            debug!("Health check passed for {}", process_id);
                        } else {
                            status.record_failure(
                                data.error_message.unwrap_or_else(|| "Unknown failure".to_string()),
                                failure_threshold
                            );
                            warn!("Health check failed for {}: consecutive failures = {}",
                                process_id, status.consecutive_failures);
                            
                            // Check if we've hit the failure threshold
                            if status.consecutive_failures >= failure_threshold {
                                error!("Health check failure threshold reached for {}", process_id);
                                // In full implementation, this would trigger restart
                                // For now, just log
                            }
                        }
                    }
                    Err(e) => {
                        let mut status = health_status.write().await;
                        status.record_failure(format!("Health check error: {}", e), failure_threshold);
                        warn!("Health check error for {}: {}", process_id, e);
                    }
                }
            }
        });
        
        self.health_check_task = Some(task);
    }
    
    /// Spawn violation handler task to process resource violations
    fn spawn_violation_handler_task(&mut self, mut rx: tokio::sync::mpsc::UnboundedReceiver<(ResourcePolicy, ResourceViolation)>) {
        let process_id = self.config.id.clone();
        
        // We need a way to trigger restarts/stops - we'll use a shared state
        // For now, we'll just log and handle simple cases
        info!("Spawning violation handler task for {}", process_id);
        
        let task = tokio::spawn(async move {
            while let Some((policy, violation)) = rx.recv().await {
                // Process violation based on policy
                Self::handle_resource_violation_static(
                    &process_id,
                    policy,
                    violation,
                ).await;
            }
        });
        
        self.violation_handler_task = Some(task);
    }
    
    /// Handle resource violation with policy (static method for async task)
    async fn handle_resource_violation_static(
        process_id: &str,
        policy: ResourcePolicy,
        violation: ResourceViolation,
    ) {
        warn!(
            "Resource violation detected for process {}: {} (policy: {:?})",
            process_id, violation.message, policy
        );
        
        match policy {
            ResourcePolicy::None => {
                // No action - just detected
                debug!("No action for resource violation (policy: none)");
            }
            
            ResourcePolicy::Log => {
                // Log violation only
                warn!(
                    "LOG: Resource limit exceeded (policy: log): {} - Type: {:?}, Severity: {:?}, Current: {}, Limit: {}",
                    violation.message, violation.limit_type, violation.severity, violation.current_value, violation.limit_value
                );
            }
            
            ResourcePolicy::Alert => {
                // Send alert/notification (for now, just error log)
                error!(
                    "ALERT: Resource limit exceeded: {} - Type: {:?}, Severity: {:?}, Current: {}, Limit: {}",
                    violation.message, violation.limit_type, violation.severity, violation.current_value, violation.limit_value
                );
                // Note: Alerting system integration planned for Phase 5 (Enterprise Features)
            }
            
            ResourcePolicy::Throttle => {
                // Future: Suspend/resume process
                warn!(
                    "THROTTLE: Resource limit exceeded (not yet implemented): {}",
                    violation.message
                );
                // TODO: Implement process throttling (SIGSTOP/SIGCONT on Unix)
            }
            
            ResourcePolicy::GracefulShutdown => {
                error!(
                    "GRACEFUL SHUTDOWN: Resource limit exceeded, process should be gracefully stopped (policy: graceful_shutdown): {}",
                    violation.message
                );
                // Note: This would require access to the ProcessControl instance
                // For now, we log the intent. Full implementation requires refactoring
                // to pass a callback or use a message passing system to the main control loop
                warn!("Graceful shutdown not yet fully implemented - requires ProcessControl access");
            }
            
            ResourcePolicy::ImmediateKill => {
                error!(
                    "IMMEDIATE KILL: Resource limit exceeded, process should be killed immediately (policy: immediate_kill): {}",
                    violation.message
                );
                // Note: This would require access to the ProcessControl instance
                warn!("Immediate kill not yet fully implemented - requires ProcessControl access");
            }
            
            ResourcePolicy::Restart => {
                error!(
                    "RESTART: Resource limit exceeded, process should be restarted (policy: restart): {}",
                    violation.message
                );
                // Note: Actual restart requires access to ProcessControl instance
                // Full implementation would:
                // 1. Check circuit breaker via lifecycle_manager.should_restart()
                // 2. Trigger restart via ProcessControl.restart()
                // 3. Record restart attempt in circuit breaker
                warn!("Restart not yet fully implemented - requires ProcessControl access and circuit breaker integration");
            }
            
            ResourcePolicy::RestartAdjusted => {
                error!(
                    "RESTART ADJUSTED: Resource limit exceeded, attempting restart with adjusted limits (policy: restart_adjusted): {}",
                    violation.message
                );
                warn!("Restart with adjusted limits not yet implemented");
            }
        }
    }
    
    /// Spawn resource monitoring background task
    fn spawn_resource_monitor_task(&mut self) {
        let Some(pid) = self.pid else {
            return;
        };
        
        info!("Spawning resource monitor task for {} (PID: {})", self.config.id, pid);
        
        let process_id = self.config.id.clone();
        let resource_usage = Arc::clone(&self.resource_usage);
        let resource_monitor = Arc::clone(&self.resource_monitor);
        let limits = self.config.management.resource_limits.clone();
        
        // Create channel for sending violations
        let (tx, rx) = tokio::sync::mpsc::unbounded_channel();
        self.violation_tx = Some(tx.clone());
        
        // Spawn violation handler task
        self.spawn_violation_handler_task(rx);
        
        let task = tokio::spawn(async move {
            let mut interval_timer = interval(Duration::from_secs(10));
            
            loop {
                interval_timer.tick().await;
                
                // Collect resource usage
                match resource_monitor.get_process_resource_usage(pid) {
                    Ok(usage) => {
                        // Update shared resource usage
                        {
                            let mut res = resource_usage.write().await;
                            *res = usage.clone();
                        }
                        
                        // Check limits with policy-aware checking
                        if let Some(ref limits_config) = limits {
                            // Convert config to ResourceLimits
                            let resource_limits = hsu_resource_limits::ResourceLimits {
                                memory: limits_config.memory.clone(),
                                cpu: limits_config.cpu.clone(),
                                process: limits_config.process.clone(),
                                // Legacy fields for backward compatibility
                                max_memory_mb: limits_config.max_memory_mb,
                                max_cpu_percent: limits_config.max_cpu_percent,
                                max_file_descriptors: limits_config.max_file_descriptors,
                            };
                            
                            // Check with policy-aware function
                            let violations = check_resource_limits_with_policy(&process_id, &usage, &resource_limits);
                            
                            // Send violations through channel for handling
                            for (violation, policy) in violations {
                                if let Err(e) = tx.send((policy, violation)) {
                                    error!("Failed to send resource violation for {}: {}", process_id, e);
                                }
                            }
                        }
                    }
                    Err(e) => {
                        debug!("Failed to collect resource usage for {}: {}", process_id, e);
                        // Process might have exited, just continue
                    }
                }
            }
        });
        
        self.resource_monitor_task = Some(task);
    }
    
    /// Stop all background tasks
    fn stop_background_tasks(&mut self) {
        if let Some(task) = self.health_check_task.take() {
            task.abort();
            debug!("Health check task stopped for {}", self.config.id);
        }
        
        if let Some(task) = self.resource_monitor_task.take() {
            task.abort();
            debug!("Resource monitor task stopped for {}", self.config.id);
        }
        
        if let Some(task) = self.violation_handler_task.take() {
            task.abort();
            debug!("Violation handler task stopped for {}", self.config.id);
        }
        
        // Drop the violation sender to signal handler task to exit
        self.violation_tx = None;
    }
}

#[async_trait]
impl ProcessControl for ProcessControlImpl {
    async fn start(&mut self) -> ProcessResult<()> {
        info!("Starting process control: {}", self.config.id);
        
        // Transition to starting state
        self.state_machine.transition_to_starting()
            .map_err(|e| CommonProcessError::StartFailed {
                id: self.config.id.clone(),
                reason: format!("Failed to transition to starting: {}", e),
            })?;
        
        // Spawn the process
        self.spawn_process().await?;
        
        // Transition to running
        self.state_machine.transition_to_running()
            .map_err(|e| CommonProcessError::StartFailed {
                id: self.config.id.clone(),
                reason: format!("Failed to transition to running: {}", e),
            })?;
        
        // Start background tasks
        self.spawn_health_check_task();
        self.spawn_resource_monitor_task();
        
        info!("Process control started successfully: {}", self.config.id);
        Ok(())
    }
    
    async fn stop(&mut self) -> ProcessResult<()> {
        info!("Stopping process control: {}", self.config.id);
        
        // Stop background tasks first
        self.stop_background_tasks();
        
        // Transition to stopping state
        if self.state_machine.can_stop() {
            let _ = self.state_machine.transition_to_stopping();
        }
        
        // Terminate the process
        self.terminate_process().await?;
        
        // Transition to stopped
        let _ = self.state_machine.transition_to_stopped();
        
        info!("Process control stopped successfully: {}", self.config.id);
        Ok(())
    }
    
    async fn restart(&mut self, force: bool) -> ProcessResult<()> {
        info!("Restarting process control: {} (force: {})", self.config.id, force);
        
        // Check circuit breaker unless forced
        if !force && self.lifecycle_manager.is_circuit_breaker_tripped() {
            let err = ProcessError::new(
                ErrorCategory::CircuitBreakerTripped,
                "Circuit breaker is tripped, cannot restart".to_string(),
                false,
            );
            self.last_error = Some(err.clone());
            
            return Err(CommonProcessError::Configuration {
                id: self.config.id.clone(),
                reason: err.details,
            });
        }
        
        // Stop first
        self.stop().await?;
        
        // Wait a bit before restarting
        tokio::time::sleep(Duration::from_secs(2)).await;
        
        // Update restart tracking
        self.restart_count += 1;
        self.last_restart_time = Some(chrono::Utc::now());
        
        // Reset health status for fresh start
        {
            let mut status = self.health_status.write().await;
            *status = HealthStatus::new();
        }
        
        // Start again
        self.start().await?;
        
        // Reset lifecycle manager on successful restart
        self.lifecycle_manager.reset_restart_counters();
        
        info!("Process control restarted successfully: {}", self.config.id);
        Ok(())
    }
    
    fn get_state(&self) -> ProcessState {
        self.state_machine.current_state()
    }
    
    fn get_diagnostics(&self) -> ProcessDiagnostics {
        let health_status = self.health_status.try_read()
            .map(|s| s.clone())
            .unwrap_or_else(|_| HealthStatus::new());
        
        let resource_usage = self.resource_usage.try_read()
            .map(|r| r.clone())
            .unwrap_or(ResourceUsage {
                cpu_percent: None,
                memory_mb: None,
                file_descriptors: None,
                network_connections: None,
            });
        
        ProcessDiagnostics {
            state: self.state_machine.current_state(),
            last_error: self.last_error.clone(),
            process_id: self.pid,
            start_time: self.start_time,
            executable_path: self.config.management.control.executable.clone(),
            executable_exists: std::path::Path::new(&self.config.management.control.executable).exists(),
            failure_count: self.lifecycle_manager.get_restart_stats().consecutive_failures,
            last_attempt_time: self.last_restart_time,
            is_healthy: health_status.is_healthy,
            cpu_usage: resource_usage.cpu_percent,
            memory_usage: resource_usage.memory_mb,
        }
    }
    
    fn get_pid(&self) -> Option<u32> {
        self.pid
    }
    
    fn is_healthy(&self) -> bool {
        self.health_status.try_read()
            .map(|s| s.is_healthy)
            .unwrap_or(true)
    }
}

impl Drop for ProcessControlImpl {
    fn drop(&mut self) {
        // Clean up background tasks
        if let Some(task) = self.health_check_task.take() {
            task.abort();
        }
        if let Some(task) = self.resource_monitor_task.take() {
            task.abort();
        }
    }
}

