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
use hsu_resource_limits::{ResourceMonitor, ResourceUsage, check_resource_limits};
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
                        
                        // Check limits
                        if let Some(ref limits_config) = limits {
                            // Convert config to ResourceLimits
                            let resource_limits = hsu_resource_limits::ResourceLimits {
                                max_memory_mb: limits_config.max_memory_mb,
                                max_cpu_percent: limits_config.max_cpu_percent,
                                max_file_descriptors: limits_config.max_file_descriptors,
                            };
                            
                            // Check returns Result<(), Vec<String>> where Err contains violations
                            if let Err(violations) = check_resource_limits(&usage, &resource_limits) {
                                warn!("Resource limit violations for {}: {:?}", process_id, violations);
                                // In full implementation, this could trigger restart or other actions
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

