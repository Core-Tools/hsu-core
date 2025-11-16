use crate::config::{ProcessConfig, ProcessManagerConfig};
use hsu_common::ProcessError;
use hsu_process_state::{ProcessState, ProcessStateMachine};
use std::collections::HashMap;
use std::process::Stdio;
use std::sync::Arc;
use tokio::process::{Child, Command};
use tokio::sync::{Mutex, RwLock};
use tokio::time::{timeout, Duration};
use tracing::{debug, error, info, warn};

/// Result type for process management operations
type Result<T> = std::result::Result<T, ProcessError>;

/// Main process manager that orchestrates all child processes
pub struct ProcessManager {
    config: ProcessManagerConfig,
    processes: Arc<RwLock<HashMap<String, ManagedProcessInstance>>>,
    state: Arc<Mutex<ProcessManagerState>>,
}

/// Individual managed process instance with runtime state
pub struct ManagedProcessInstance {
    pub config: ProcessConfig,
    pub child: Option<Child>,
    pub state_machine: ProcessStateMachine,
    pub pid: Option<u32>,
    pub start_time: Option<chrono::DateTime<chrono::Utc>>,
    pub restart_count: u32,
    pub last_restart_time: Option<chrono::DateTime<chrono::Utc>>,
}

/// Process manager overall state
#[derive(Debug, Clone)]
pub enum ProcessManagerState {
    Initializing,
    Starting,
    Running,
    Stopping,
    Stopped,
    Error(String),
}

/// Process information structure for external queries
#[derive(Debug, Clone)]
pub struct ProcessInfo {
    pub id: String,
    pub state: ProcessState,
    pub pid: Option<u32>,
    pub start_time: Option<chrono::DateTime<chrono::Utc>>,
    pub restart_count: u32,
    pub cpu_usage: Option<f32>,
    pub memory_usage: Option<u64>,
    pub uptime: Option<Duration>,
}

/// Process diagnostics information
#[derive(Debug, Clone)]
pub struct ProcessDiagnostics {
    pub process_info: ProcessInfo,
    pub health_status: Option<HealthStatus>,
    pub resource_usage: ResourceUsage,
    pub last_error: Option<String>,
    pub logs_preview: Vec<String>,
}

/// Health status information
#[derive(Debug, Clone)]
pub struct HealthStatus {
    pub is_healthy: bool,
    pub last_check: Option<chrono::DateTime<chrono::Utc>>,
    pub consecutive_failures: u32,
    pub last_success: Option<chrono::DateTime<chrono::Utc>>,
    pub failure_reason: Option<String>,
}

/// Resource usage information
#[derive(Debug, Clone)]
pub struct ResourceUsage {
    pub cpu_percent: Option<f32>,
    pub memory_mb: Option<u64>,
    pub file_descriptors: Option<u32>,
    pub network_connections: Option<u32>,
}

impl ProcessManager {
    /// Create a new process manager with the given configuration
    pub async fn new(config: ProcessManagerConfig) -> Result<Self> {
        info!("Creating process manager with {} processes", config.managed_processes.len());
        
        let manager = Self {
            config,
            processes: Arc::new(RwLock::new(HashMap::new())),
            state: Arc::new(Mutex::new(ProcessManagerState::Initializing)),
        };

        // Initialize processes from configuration
        manager.initialize_processes().await?;

        Ok(manager)
    }

    /// Initialize all processes from configuration
    async fn initialize_processes(&self) -> Result<()> {
        let mut processes = self.processes.write().await;
        
        for process_config in &self.config.managed_processes {
            if !process_config.enabled {
                debug!("Skipping disabled process: {}", process_config.id);
                continue;
            }

            let managed_process = ManagedProcessInstance {
                config: process_config.clone(),
                child: None,
                state_machine: ProcessStateMachine::new(&process_config.id),
                pid: None,
                start_time: None,
                restart_count: 0,
                last_restart_time: None,
            };

            processes.insert(process_config.id.clone(), managed_process);
            info!("Initialized process: {}", process_config.id);
        }

        Ok(())
    }

    /// Start the process manager and all enabled processes
    pub async fn start(&mut self) -> Result<()> {
        info!("Starting process manager");
        
        {
            let mut state = self.state.lock().await;
            *state = ProcessManagerState::Starting;
        }

        // Start all enabled processes
        let process_ids: Vec<String> = {
            let processes = self.processes.read().await;
            processes.keys().cloned().collect()
        };

        for process_id in process_ids {
            if let Err(e) = self.start_process(&process_id).await {
                error!("Failed to start process {}: {}", process_id, e);
                // Continue with other processes
            }
        }

        {
            let mut state = self.state.lock().await;
            *state = ProcessManagerState::Running;
        }

        info!("Process manager started successfully");
        Ok(())
    }

    /// Start a specific process
    pub async fn start_process(&self, process_id: &str) -> Result<()> {
        info!("Starting process: {}", process_id);

        let mut processes = self.processes.write().await;
        let managed_process = processes
            .get_mut(process_id)
            .ok_or_else(|| ProcessError::not_found(process_id))?;

        // Check if process can be started
        if !managed_process.state_machine.can_start() {
            return Err(ProcessError::invalid_state(
                process_id,
                "Stopped or Failed",
                format!("{:?}", managed_process.state_machine.current_state()),
            ).into());
        }

        // Transition to starting state
        managed_process.state_machine.transition_to_starting()?;

        // Spawn the process
        let child = self.spawn_process_child(&managed_process.config).await?;
        let pid = child.id().unwrap_or(0);

        managed_process.child = Some(child);
        managed_process.pid = Some(pid);
        managed_process.start_time = Some(chrono::Utc::now());

        // Transition to running state
        managed_process.state_machine.transition_to_running()?;

        info!("Process started successfully: {} (PID: {})", process_id, pid);
        Ok(())
    }

    /// Spawn a child process from configuration
    async fn spawn_process_child(&self, config: &ProcessConfig) -> Result<Child> {
        let mut command = Command::new(&config.management.control.executable);
        
        // Set arguments
        command.args(&config.management.control.arguments);
        
        // Set working directory
        if let Some(ref wd) = config.management.control.working_directory {
            command.current_dir(wd);
        }
        
        // Set environment variables
        for (key, value) in &config.management.control.environment {
            command.env(key, value);
        }
        
        // Configure stdio
        command
            .stdout(Stdio::piped())
            .stderr(Stdio::piped())
            .stdin(Stdio::null());

        // Platform-specific configuration
        #[cfg(windows)]
        {
            // Windows-specific process creation flags
            #[allow(unused_imports)]
            use std::os::windows::process::CommandExt;
            command.creation_flags(0x08000000); // CREATE_NO_WINDOW
        }

        // Spawn the process
        let child = command.spawn()
            .map_err(|e| ProcessError::spawn_failed(&config.id, e.to_string()))?;

        Ok(child)
    }

    /// Stop a specific process
    pub async fn stop_process(&self, process_id: &str) -> Result<()> {
        info!("Stopping process: {}", process_id);

        let mut processes = self.processes.write().await;
        let managed_process = processes
            .get_mut(process_id)
            .ok_or_else(|| ProcessError::not_found(process_id))?;

        // Check if process can be stopped
        if !managed_process.state_machine.can_stop() {
            return Err(ProcessError::invalid_state(
                process_id,
                "Running",
                format!("{:?}", managed_process.state_machine.current_state()),
            ).into());
        }

        // Transition to stopping state
        managed_process.state_machine.transition_to_stopping()?;

        // Stop the child process
        if let Some(ref mut child) = managed_process.child {
            self.stop_child_process(child, &managed_process.config).await?;
        }

        // Clean up
        managed_process.child = None;
        managed_process.pid = None;
        managed_process.state_machine.transition_to_stopped()?;

        info!("Process stopped successfully: {}", process_id);
        Ok(())
    }

    /// Stop a child process gracefully or forcefully
    async fn stop_child_process(&self, child: &mut Child, config: &ProcessConfig) -> Result<()> {
        let shutdown_timeout = config.management.control.shutdown_timeout;

        // Try graceful shutdown first
        #[cfg(unix)]
        {
            if let Some(pid) = child.id() {
                // Send SIGTERM
                let nix_pid = nix::unistd::Pid::from_raw(pid as i32);
                if let Err(e) = nix::sys::signal::kill(nix_pid, nix::sys::signal::Signal::SIGTERM) {
                    warn!("Failed to send SIGTERM to process {}: {}", pid, e);
                }
            }
        }

        #[cfg(windows)]
        {
            // On Windows, we'll use the kill method directly
            // In a real implementation, you might want to try a more graceful approach first
        }

        // Wait for graceful shutdown
        match timeout(shutdown_timeout, child.wait()).await {
            Ok(Ok(exit_status)) => {
                info!("Process exited gracefully with status: {}", exit_status);
                return Ok(());
            }
            Ok(Err(e)) => {
                warn!("Error waiting for process exit: {}", e);
            }
            Err(_) => {
                warn!("Process did not exit gracefully within timeout, force killing");
            }
        }

        // Force kill if graceful shutdown failed
        if let Err(e) = child.kill().await {
            error!("Failed to force kill process: {}", e);
            return Err(ProcessError::stop_failed(&config.id, e.to_string()).into());
        }

        // Wait for force kill to complete
        if let Err(e) = child.wait().await {
            warn!("Error waiting for force-killed process: {}", e);
        }

        Ok(())
    }

    /// Shutdown the entire process manager
    pub async fn shutdown(&mut self) -> Result<()> {
        info!("Shutting down process manager");

        {
            let mut state = self.state.lock().await;
            *state = ProcessManagerState::Stopping;
        }

        // Stop all processes
        let process_ids: Vec<String> = {
            let processes = self.processes.read().await;
            processes.keys().cloned().collect()
        };

        for process_id in process_ids {
            if let Err(e) = self.stop_process(&process_id).await {
                error!("Failed to stop process {}: {}", process_id, e);
                // Continue with other processes
            }
        }

        // Force shutdown after timeout
        let force_timeout = self.config.process_manager.force_shutdown_timeout;
        tokio::time::sleep(force_timeout).await;

        // Force kill any remaining processes
        let mut processes = self.processes.write().await;
        for (id, managed_process) in processes.iter_mut() {
            if let Some(ref mut child) = managed_process.child {
                warn!("Force killing remaining process: {}", id);
                let _ = child.kill().await;
            }
        }

        {
            let mut state = self.state.lock().await;
            *state = ProcessManagerState::Stopped;
        }

        info!("Process manager shut down successfully");
        Ok(())
    }

    /// Get information about a specific process
    pub async fn get_process_info(&self, process_id: &str) -> Result<ProcessInfo> {
        let processes = self.processes.read().await;
        let managed_process = processes
            .get(process_id)
            .ok_or_else(|| ProcessError::not_found(process_id))?;

        let uptime = managed_process.start_time.map(|start_time| {
            let now = chrono::Utc::now();
            Duration::from_secs((now - start_time).num_seconds() as u64)
        });

        Ok(ProcessInfo {
            id: process_id.to_string(),
            state: managed_process.state_machine.current_state(),
            pid: managed_process.pid,
            start_time: managed_process.start_time,
            restart_count: managed_process.restart_count,
            cpu_usage: None, // TODO: Implement monitoring
            memory_usage: None, // TODO: Implement monitoring
            uptime,
        })
    }

    /// Get information about all processes
    pub async fn get_all_process_info(&self) -> Vec<ProcessInfo> {
        let processes = self.processes.read().await;
        let mut info_list = Vec::new();

        for (id, _) in processes.iter() {
            if let Ok(info) = self.get_process_info(id).await {
                info_list.push(info);
            }
        }

        info_list
    }

    /// Get the current state of the process manager
    pub async fn get_manager_state(&self) -> ProcessManagerState {
        let state = self.state.lock().await;
        state.clone()
    }
}

impl ManagedProcessInstance {
    /// Check if the process is currently running
    pub fn is_running(&self) -> bool {
        matches!(self.state_machine.current_state(), ProcessState::Running)
    }

    /// Check if the process has a valid PID
    pub fn has_pid(&self) -> bool {
        self.pid.is_some()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::config::{ProcessControlConfig, ProcessManagementConfig, ProcessManagementType};

    fn create_test_config() -> ProcessManagerConfig {
        ProcessManagerConfig {
            process_manager: crate::config::ProcessManagerOptions {
                port: 8080,
                log_level: "info".to_string(),
                force_shutdown_timeout: Duration::from_secs(30),
            },
            managed_processes: vec![ProcessConfig {
                id: "test-process".to_string(),
                process_type: ProcessManagementType::StandardManaged,
                profile_type: "test".to_string(),
                enabled: true,
                management: ProcessManagementConfig {
                    control: ProcessControlConfig {
                        executable: "echo".to_string(),
                        arguments: vec!["hello".to_string()],
                        working_directory: None,
                        environment: HashMap::new(),
                        startup_timeout: Duration::from_secs(10),
                        shutdown_timeout: Duration::from_secs(5),
                    },
                    health_check: None,
                    resource_limits: None,
                    restart_policy: None,
                    logging: None,
                },
            }],
            log_collection: None,
        }
    }

    #[tokio::test]
    async fn test_process_manager_creation() {
        let config = create_test_config();
        let manager = ProcessManager::new(config).await;
        assert!(manager.is_ok());

        let manager = manager.unwrap();
        let processes = manager.processes.read().await;
        assert_eq!(processes.len(), 1);
        assert!(processes.contains_key("test-process"));
    }

    #[tokio::test]
    async fn test_process_manager_state_transitions() {
        let config = create_test_config();
        let manager = ProcessManager::new(config).await.unwrap();

        let initial_state = manager.get_manager_state().await;
        assert!(matches!(initial_state, ProcessManagerState::Initializing));
    }
}
