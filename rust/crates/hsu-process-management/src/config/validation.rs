use super::*;
use anyhow::{anyhow, Result};
use std::collections::HashSet;

/// Validate the complete configuration
pub fn validate_config(config: &ProcessManagerConfig) -> Result<()> {
    validate_process_manager_options(&config.process_manager)?;
    validate_process_configs(&config.managed_processes)?;
    
    if let Some(ref log_config) = config.log_collection {
        validate_log_collection_config(log_config)?;
    }
    
    Ok(())
}

/// Validate process manager options
fn validate_process_manager_options(options: &ProcessManagerOptions) -> Result<()> {
    if options.port == 0 || options.port > 65535 {
        return Err(anyhow!("Port must be between 1 and 65535, got: {}", options.port));
    }
    
    if options.force_shutdown_timeout.as_secs() == 0 {
        return Err(anyhow!("Force shutdown timeout must be greater than 0"));
    }
    
    match options.log_level.to_lowercase().as_str() {
        "trace" | "debug" | "info" | "warn" | "error" => Ok(()),
        _ => Err(anyhow!("Invalid log level: {}, must be one of: trace, debug, info, warn, error", options.log_level))
    }
}

/// Validate all process configurations
fn validate_process_configs(processes: &[ProcessConfig]) -> Result<()> {
    if processes.is_empty() {
        return Err(anyhow!("At least one process must be configured"));
    }
    
    // Check for duplicate IDs
    let mut ids = HashSet::new();
    for process in processes {
        if !ids.insert(&process.id) {
            return Err(anyhow!("Duplicate process ID: {}", process.id));
        }
        
        validate_process_config(process)?;
    }
    
    Ok(())
}

/// Validate a single process configuration
fn validate_process_config(process: &ProcessConfig) -> Result<()> {
    if process.id.is_empty() {
        return Err(anyhow!("Process ID cannot be empty"));
    }
    
    if process.id.len() > 64 {
        return Err(anyhow!("Process ID too long (max 64 characters): {}", process.id));
    }
    
    // Validate ID contains only safe characters
    if !process.id.chars().all(|c| c.is_alphanumeric() || c == '-' || c == '_') {
        return Err(anyhow!("Process ID can only contain alphanumeric characters, hyphens, and underscores: {}", process.id));
    }
    
    validate_process_management_config(&process.management)?;
    
    Ok(())
}

/// Validate process management configuration
fn validate_process_management_config(management: &ProcessManagementConfig) -> Result<()> {
    validate_process_control_config(&management.control)?;
    
    if let Some(ref health_check) = management.health_check {
        validate_health_check_config(health_check)?;
    }
    
    if let Some(ref resource_limits) = management.resource_limits {
        validate_resource_limits_config(resource_limits)?;
    }
    
    if let Some(ref restart_policy) = management.restart_policy {
        validate_restart_policy_config(restart_policy)?;
    }
    
    Ok(())
}

/// Validate process control configuration
fn validate_process_control_config(control: &ProcessControlConfig) -> Result<()> {
    if control.executable.is_empty() {
        return Err(anyhow!("Executable path cannot be empty"));
    }
    
    if control.startup_timeout.as_secs() == 0 {
        return Err(anyhow!("Startup timeout must be greater than 0"));
    }
    
    if control.shutdown_timeout.as_secs() == 0 {
        return Err(anyhow!("Shutdown timeout must be greater than 0"));
    }
    
    // Validate environment variable names
    for (key, _) in &control.environment {
        if key.is_empty() {
            return Err(anyhow!("Environment variable name cannot be empty"));
        }
        
        if !key.chars().all(|c| c.is_alphanumeric() || c == '_') {
            return Err(anyhow!("Environment variable name can only contain alphanumeric characters and underscores: {}", key));
        }
    }
    
    Ok(())
}

/// Validate health check configuration
fn validate_health_check_config(health_check: &HealthCheckConfig) -> Result<()> {
    if health_check.interval.as_secs() == 0 {
        return Err(anyhow!("Health check interval must be greater than 0"));
    }
    
    if health_check.timeout.as_secs() == 0 {
        return Err(anyhow!("Health check timeout must be greater than 0"));
    }
    
    if health_check.failure_threshold == 0 {
        return Err(anyhow!("Health check failure threshold must be greater than 0"));
    }
    
    if health_check.timeout >= health_check.interval {
        return Err(anyhow!("Health check timeout must be less than interval"));
    }
    
    Ok(())
}

/// Validate resource limits configuration
fn validate_resource_limits_config(limits: &ResourceLimitsConfig) -> Result<()> {
    if let Some(memory) = limits.max_memory_mb {
        if memory == 0 {
            return Err(anyhow!("Max memory limit must be greater than 0"));
        }
        
        if memory > 1024 * 1024 { // 1TB limit
            return Err(anyhow!("Max memory limit too high (max 1TB): {} MB", memory));
        }
    }
    
    if let Some(cpu) = limits.max_cpu_percent {
        if cpu <= 0.0 || cpu > 100.0 {
            return Err(anyhow!("Max CPU percent must be between 0 and 100, got: {}", cpu));
        }
    }
    
    if let Some(fd) = limits.max_file_descriptors {
        if fd == 0 {
            return Err(anyhow!("Max file descriptors must be greater than 0"));
        }
    }
    
    Ok(())
}

/// Validate restart policy configuration
fn validate_restart_policy_config(restart_policy: &RestartPolicyConfig) -> Result<()> {
    if restart_policy.max_attempts == 0 {
        return Err(anyhow!("Restart policy max attempts must be greater than 0"));
    }
    
    if restart_policy.max_attempts > 100 {
        return Err(anyhow!("Restart policy max attempts too high (max 100): {}", restart_policy.max_attempts));
    }
    
    if restart_policy.restart_delay.as_secs() == 0 {
        return Err(anyhow!("Restart delay must be greater than 0"));
    }
    
    if restart_policy.backoff_multiplier < 1.0 || restart_policy.backoff_multiplier > 10.0 {
        return Err(anyhow!("Backoff multiplier must be between 1.0 and 10.0, got: {}", restart_policy.backoff_multiplier));
    }
    
    Ok(())
}

/// Validate log collection configuration
fn validate_log_collection_config(log_config: &LogCollectionConfig) -> Result<()> {
    if let Some(ref aggregation) = log_config.global_aggregation {
        validate_global_aggregation_config(aggregation)?;
    }
    
    Ok(())
}

/// Validate global aggregation configuration
fn validate_global_aggregation_config(aggregation: &GlobalAggregationConfig) -> Result<()> {
    for target in &aggregation.targets {
        validate_log_target_config(target)?;
    }
    
    Ok(())
}

/// Validate log target configuration
fn validate_log_target_config(target: &LogTargetConfig) -> Result<()> {
    match target.target_type.as_str() {
        "file" => {
            if target.path.is_none() {
                return Err(anyhow!("File log target must specify a path"));
            }
        }
        "process_manager_stdout" | "process_manager_stderr" => {
            // These don't need additional validation
        }
        _ => {
            return Err(anyhow!("Invalid log target type: {}", target.target_type));
        }
    }
    
    match target.format.as_str() {
        "plain" | "enhanced_plain" | "json" => Ok(()),
        _ => Err(anyhow!("Invalid log format: {}, must be one of: plain, enhanced_plain, json", target.format))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_validate_process_id() {
        let mut config = ProcessConfig {
            id: "".to_string(),
            process_type: ProcessManagementType::StandardManaged,
            profile_type: "general".to_string(),
            enabled: true,
            management: ProcessManagementConfig {
                control: ProcessControlConfig {
                    executable: "./test".to_string(),
                    arguments: vec![],
                    working_directory: None,
                    environment: std::collections::HashMap::new(),
                    startup_timeout: Duration::from_secs(30),
                    shutdown_timeout: Duration::from_secs(10),
                },
                health_check: None,
                resource_limits: None,
                restart_policy: None,
                logging: None,
            },
        };

        // Empty ID should fail
        assert!(validate_process_config(&config).is_err());

        // Valid ID should pass
        config.id = "test-process".to_string();
        assert!(validate_process_config(&config).is_ok());

        // Invalid characters should fail
        config.id = "test process".to_string();
        assert!(validate_process_config(&config).is_err());

        // Too long ID should fail
        config.id = "a".repeat(65);
        assert!(validate_process_config(&config).is_err());
    }

    #[test]
    fn test_validate_port() {
        let mut options = ProcessManagerOptions {
            port: 0,
            log_level: "info".to_string(),
            force_shutdown_timeout: Duration::from_secs(30),
        };

        // Port 0 should fail
        assert!(validate_process_manager_options(&options).is_err());

        // Valid port should pass
        options.port = 8080;
        assert!(validate_process_manager_options(&options).is_ok());

        // Port too high should fail
        options.port = 70000;
        assert!(validate_process_manager_options(&options).is_err());
    }
}
