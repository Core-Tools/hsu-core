// Process monitoring and resource tracking
// This module will handle CPU, memory, and other resource monitoring

use crate::errors::{MonitoringError, MonitoringResult};
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::time::Duration;
use sysinfo::{ProcessExt, System, SystemExt, Pid};
use tokio::time::interval;
use tracing::{debug, warn};

/// Process resource monitoring manager
pub struct ProcessMonitor {
    system: System,
    monitored_processes: HashMap<String, ProcessMonitoringConfig>,
}

/// Configuration for process monitoring
#[derive(Debug, Clone)]
pub struct ProcessMonitoringConfig {
    pub process_id: String,
    pub pid: Option<u32>,
    pub monitor_interval: Duration,
    pub memory_limit_mb: Option<u64>,
    pub cpu_limit_percent: Option<f32>,
}

/// Resource usage snapshot
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ResourceUsage {
    pub timestamp: chrono::DateTime<chrono::Utc>,
    pub cpu_percent: f32,
    pub memory_mb: u64,
    pub virtual_memory_mb: u64,
    pub file_descriptors: Option<u32>,
    pub threads: Option<u32>,
    pub uptime_seconds: u64,
}

/// Resource limit violation
#[derive(Debug, Clone)]
pub struct ResourceViolation {
    pub process_id: String,
    pub violation_type: ViolationType,
    pub current_value: f64,
    pub limit_value: f64,
    pub timestamp: chrono::DateTime<chrono::Utc>,
}

#[derive(Debug, Clone)]
pub enum ViolationType {
    MemoryExceeded,
    CpuExceeded,
    FileDescriptorsExceeded,
}

impl ProcessMonitor {
    pub fn new() -> Self {
        Self {
            system: System::new_all(),
            monitored_processes: HashMap::new(),
        }
    }

    /// Add a process to monitoring
    pub fn add_process(&mut self, config: ProcessMonitoringConfig) {
        debug!("Adding process to monitoring: {}", config.process_id);
        self.monitored_processes.insert(config.process_id.clone(), config);
    }

    /// Remove a process from monitoring
    pub fn remove_process(&mut self, process_id: &str) {
        debug!("Removing process from monitoring: {}", process_id);
        self.monitored_processes.remove(process_id);
    }

    /// Update PID for a monitored process
    pub fn update_process_pid(&mut self, process_id: &str, pid: u32) {
        if let Some(config) = self.monitored_processes.get_mut(process_id) {
            config.pid = Some(pid);
            debug!("Updated PID for process {}: {}", process_id, pid);
        }
    }

    /// Get current resource usage for a process
    pub fn get_resource_usage(&mut self, process_id: &str) -> MonitoringResult<ResourceUsage> {
        let config = self.monitored_processes
            .get(process_id)
            .ok_or_else(|| MonitoringError::ProcessMetrics {
                id: process_id.to_string(),
                reason: "Process not found in monitoring".to_string(),
            })?;

        let pid = config.pid
            .ok_or_else(|| MonitoringError::ProcessMetrics {
                id: process_id.to_string(),
                reason: "No PID available".to_string(),
            })?;

        self.system.refresh_process(Pid::from(pid as usize));
        
        let process = self.system.process(Pid::from(pid as usize))
            .ok_or_else(|| MonitoringError::ProcessMetrics {
                id: process_id.to_string(),
                reason: "Process not found in system".to_string(),
            })?;

        let memory_mb = process.memory() / 1024; // Convert from KB to MB
        let virtual_memory_mb = process.virtual_memory() / 1024;
        let cpu_percent = process.cpu_usage();
        let uptime_seconds = process.run_time();

        Ok(ResourceUsage {
            timestamp: chrono::Utc::now(),
            cpu_percent,
            memory_mb,
            virtual_memory_mb,
            file_descriptors: None, // TODO: Platform-specific implementation
            threads: None, // TODO: Platform-specific implementation
            uptime_seconds,
        })
    }

    /// Check for resource limit violations
    pub fn check_resource_violations(&mut self, process_id: &str) -> MonitoringResult<Vec<ResourceViolation>> {
        let config = self.monitored_processes
            .get(process_id)
            .cloned()
            .ok_or_else(|| MonitoringError::ProcessMetrics {
                id: process_id.to_string(),
                reason: "Process not found in monitoring".to_string(),
            })?;

        let usage = self.get_resource_usage(process_id)?;
        let mut violations = Vec::new();
        let now = chrono::Utc::now();

        // Check memory limit
        if let Some(memory_limit) = config.memory_limit_mb {
            if usage.memory_mb > memory_limit {
                violations.push(ResourceViolation {
                    process_id: process_id.to_string(),
                    violation_type: ViolationType::MemoryExceeded,
                    current_value: usage.memory_mb as f64,
                    limit_value: memory_limit as f64,
                    timestamp: now,
                });
            }
        }

        // Check CPU limit
        if let Some(cpu_limit) = config.cpu_limit_percent {
            if usage.cpu_percent > cpu_limit {
                violations.push(ResourceViolation {
                    process_id: process_id.to_string(),
                    violation_type: ViolationType::CpuExceeded,
                    current_value: usage.cpu_percent as f64,
                    limit_value: cpu_limit as f64,
                    timestamp: now,
                });
            }
        }

        Ok(violations)
    }

    /// Get resource usage for all monitored processes
    pub fn get_all_resource_usage(&mut self) -> HashMap<String, MonitoringResult<ResourceUsage>> {
        let mut results = HashMap::new();
        
        // Refresh system information once for all processes
        self.system.refresh_processes();
        
        for process_id in self.monitored_processes.keys().cloned().collect::<Vec<_>>() {
            let usage = self.get_resource_usage(&process_id);
            results.insert(process_id, usage);
        }
        
        results
    }

    /// Start periodic monitoring task
    pub async fn start_monitoring_task(
        mut self,
        violation_callback: impl Fn(ResourceViolation) + Send + 'static,
    ) {
        let mut interval = interval(Duration::from_secs(30)); // Default monitoring interval
        
        loop {
            interval.tick().await;
            
            for process_id in self.monitored_processes.keys().cloned().collect::<Vec<_>>() {
                match self.check_resource_violations(&process_id) {
                    Ok(violations) => {
                        for violation in violations {
                            warn!(
                                "Resource violation detected for process {}: {:?}",
                                process_id, violation.violation_type
                            );
                            violation_callback(violation);
                        }
                    }
                    Err(e) => {
                        debug!("Failed to check resource violations for {}: {}", process_id, e);
                    }
                }
            }
        }
    }

    /// Get system-wide resource information
    pub fn get_system_info(&mut self) -> MonitoringResult<SystemInfo> {
        self.system.refresh_all();
        
        Ok(SystemInfo {
            total_memory_mb: self.system.total_memory() / 1024,
            available_memory_mb: self.system.available_memory() / 1024,
            used_memory_mb: self.system.used_memory() / 1024,
            total_swap_mb: self.system.total_swap() / 1024,
            used_swap_mb: self.system.used_swap() / 1024,
            cpu_count: self.system.cpus().len() as u32,
            load_average: {
                let load = self.system.load_average();
                (load.one, load.five, load.fifteen)
            },
            uptime_seconds: self.system.uptime(),
        })
    }
}

/// System-wide resource information
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SystemInfo {
    pub total_memory_mb: u64,
    pub available_memory_mb: u64,
    pub used_memory_mb: u64,
    pub total_swap_mb: u64,
    pub used_swap_mb: u64,
    pub cpu_count: u32,
    pub load_average: (f64, f64, f64), // 1, 5, 15 minute averages
    pub uptime_seconds: u64,
}

impl Default for ProcessMonitor {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_process_monitor_creation() {
        let monitor = ProcessMonitor::new();
        assert!(monitor.monitored_processes.is_empty());
    }

    #[test]
    fn test_add_remove_process() {
        let mut monitor = ProcessMonitor::new();
        
        let config = ProcessMonitoringConfig {
            process_id: "test-process".to_string(),
            pid: Some(1234),
            monitor_interval: Duration::from_secs(30),
            memory_limit_mb: Some(512),
            cpu_limit_percent: Some(80.0),
        };
        
        monitor.add_process(config);
        assert!(monitor.monitored_processes.contains_key("test-process"));
        
        monitor.remove_process("test-process");
        assert!(!monitor.monitored_processes.contains_key("test-process"));
    }

    #[test]
    fn test_update_process_pid() {
        let mut monitor = ProcessMonitor::new();
        
        let config = ProcessMonitoringConfig {
            process_id: "test-process".to_string(),
            pid: None,
            monitor_interval: Duration::from_secs(30),
            memory_limit_mb: Some(512),
            cpu_limit_percent: Some(80.0),
        };
        
        monitor.add_process(config);
        monitor.update_process_pid("test-process", 5678);
        
        let updated_config = monitor.monitored_processes.get("test-process").unwrap();
        assert_eq!(updated_config.pid, Some(5678));
    }
}
