//! # HSU Resource Limits
//!
//! Resource monitoring and enforcement for the HSU framework.
//!
//! This crate provides:
//! - Resource usage monitoring (CPU, memory, file descriptors)
//! - Resource limit enforcement
//! - Cross-platform resource management
//!
//! This corresponds to the Go package `pkg/resourcelimits`.

use hsu_common::{ProcessError, ProcessResult};
use serde::{Deserialize, Serialize};
use sysinfo::{Pid, ProcessRefreshKind, System};
use std::sync::{Arc, Mutex};
use tracing::debug;

/// Resource usage information for a process.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ResourceUsage {
    pub cpu_percent: Option<f32>,
    pub memory_mb: Option<u64>,
    pub file_descriptors: Option<u32>,
    pub network_connections: Option<u32>,
}

/// Resource limits configuration.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ResourceLimits {
    pub max_memory_mb: Option<u64>,
    pub max_cpu_percent: Option<f32>,
    pub max_file_descriptors: Option<u32>,
}

/// Resource monitor that tracks process resource usage
pub struct ResourceMonitor {
    system: Arc<Mutex<System>>,
}

impl ResourceMonitor {
    /// Create a new resource monitor
    pub fn new() -> Self {
        Self {
            system: Arc::new(Mutex::new(System::new_all())),
        }
    }
    
    /// Get current resource usage for a process
    pub fn get_process_resource_usage(&self, pid: u32) -> ProcessResult<ResourceUsage> {
        let mut system = self.system.lock().unwrap();
        
        // Refresh process information
        let sysinfo_pid = Pid::from_u32(pid);
        system.refresh_process_specifics(sysinfo_pid, ProcessRefreshKind::new());
        
        // Get process information
        let process = system.process(sysinfo_pid)
            .ok_or_else(|| ProcessError::NotFound { 
                id: format!("Process with PID {} not found", pid) 
            })?;
        
        // Get CPU usage (percentage)
        let cpu_percent = process.cpu_usage();
        
        // Get memory usage (convert from bytes to MB)
        let memory_bytes = process.memory();
        let memory_mb = (memory_bytes as f64 / 1024.0 / 1024.0) as u64;
        
        // Get file descriptor count (Unix-specific)
        #[cfg(unix)]
        let file_descriptors = get_process_fd_count(pid);
        
        #[cfg(not(unix))]
        let file_descriptors = None;
        
        debug!(
            "Resource usage for PID {}: CPU={:.1}%, Memory={} MB, FDs={:?}",
            pid, cpu_percent, memory_mb, file_descriptors
        );
        
        Ok(ResourceUsage {
            cpu_percent: Some(cpu_percent),
            memory_mb: Some(memory_mb),
            file_descriptors,
            network_connections: None, // TODO: Implement network connection counting
        })
    }
    
    /// Refresh system information (should be called periodically)
    pub fn refresh(&self) {
        let mut system = self.system.lock().unwrap();
        system.refresh_all();
    }
}

impl Default for ResourceMonitor {
    fn default() -> Self {
        Self::new()
    }
}

/// Get current resource usage for a process (convenience function)
pub fn get_process_resource_usage(pid: u32) -> ProcessResult<ResourceUsage> {
    let monitor = ResourceMonitor::new();
    monitor.get_process_resource_usage(pid)
}

/// Get file descriptor count for a process (Unix-specific)
#[cfg(unix)]
fn get_process_fd_count(pid: u32) -> Option<u32> {
    use std::fs;
    
    // On Linux, read /proc/[pid]/fd directory
    let fd_path = format!("/proc/{}/fd", pid);
    
    match fs::read_dir(&fd_path) {
        Ok(entries) => {
            let count = entries.count() as u32;
            Some(count)
        }
        Err(_) => {
            // Failed to read FD directory (process may not exist or access denied)
            None
        }
    }
}

/// Get file descriptor count for a process (non-Unix stub)
#[cfg(not(unix))]
#[allow(dead_code)]
fn get_process_fd_count(_pid: u32) -> Option<u32> {
    None
}

/// Check if resource usage exceeds limits.
pub fn check_resource_limits(
    usage: &ResourceUsage,
    limits: &ResourceLimits,
) -> Result<(), Vec<String>> {
    let mut violations = Vec::new();

    if let (Some(cpu), Some(limit)) = (usage.cpu_percent, limits.max_cpu_percent) {
        if cpu > limit {
            violations.push(format!("CPU usage {:.1}% exceeds limit {:.1}%", cpu, limit));
        }
    }

    if let (Some(mem), Some(limit)) = (usage.memory_mb, limits.max_memory_mb) {
        if mem > limit {
            violations.push(format!("Memory usage {} MB exceeds limit {} MB", mem, limit));
        }
    }

    if let (Some(fds), Some(limit)) = (usage.file_descriptors, limits.max_file_descriptors) {
        if fds > limit {
            violations.push(format!(
                "File descriptors {} exceeds limit {}",
                fds, limit
            ));
        }
    }

    if violations.is_empty() {
        Ok(())
    } else {
        Err(violations)
    }
}

