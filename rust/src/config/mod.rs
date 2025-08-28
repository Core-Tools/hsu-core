use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::path::Path;
use std::time::Duration;
use anyhow::{Context, Result};

pub mod validation;

/// Top-level configuration structure
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ProcessManagerConfig {
    pub process_manager: ProcessManagerOptions,
    pub managed_processes: Vec<ProcessConfig>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub log_collection: Option<LogCollectionConfig>,
}

/// Process manager configuration options
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ProcessManagerOptions {
    pub port: u16,
    #[serde(default = "default_log_level")]
    pub log_level: String,
    #[serde(
        default = "default_force_shutdown_timeout",
        with = "duration_serde"
    )]
    pub force_shutdown_timeout: Duration,
}

/// Individual process configuration
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ProcessConfig {
    pub id: String,
    #[serde(rename = "type")]
    pub process_type: ProcessManagementType,
    #[serde(default = "default_profile_type")]
    pub profile_type: String,
    #[serde(default = "default_enabled")]
    pub enabled: bool,
    pub management: ProcessManagementConfig,
}

/// Process management type enumeration
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Hash)]
#[serde(rename_all = "snake_case")]
pub enum ProcessManagementType {
    StandardManaged,
    IntegratedManaged,
    Unmanaged,
}

/// Process management configuration
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ProcessManagementConfig {
    pub control: ProcessControlConfig,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub health_check: Option<HealthCheckConfig>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub resource_limits: Option<ResourceLimitsConfig>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub restart_policy: Option<RestartPolicyConfig>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub logging: Option<ProcessLoggingConfig>,
}

/// Process control configuration
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ProcessControlConfig {
    pub executable: String,
    #[serde(default)]
    pub arguments: Vec<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub working_directory: Option<String>,
    #[serde(default)]
    pub environment: HashMap<String, String>,
    #[serde(
        default = "default_startup_timeout",
        with = "duration_serde"
    )]
    pub startup_timeout: Duration,
    #[serde(
        default = "default_shutdown_timeout",
        with = "duration_serde"
    )]
    pub shutdown_timeout: Duration,
}

/// Health check configuration
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct HealthCheckConfig {
    #[serde(default = "default_enabled")]
    pub enabled: bool,
    #[serde(
        default = "default_health_check_interval",
        with = "duration_serde"
    )]
    pub interval: Duration,
    #[serde(
        default = "default_health_check_timeout",
        with = "duration_serde"
    )]
    pub timeout: Duration,
    #[serde(default = "default_failure_threshold")]
    pub failure_threshold: u32,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub http_endpoint: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub grpc_service: Option<String>,
}

/// Resource limits configuration
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ResourceLimitsConfig {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub max_memory_mb: Option<u64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub max_cpu_percent: Option<f32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub max_file_descriptors: Option<u32>,
}

/// Restart policy configuration
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct RestartPolicyConfig {
    #[serde(default = "default_restart_strategy")]
    pub strategy: RestartStrategy,
    #[serde(default = "default_max_attempts")]
    pub max_attempts: u32,
    #[serde(
        default = "default_restart_delay",
        with = "duration_serde"
    )]
    pub restart_delay: Duration,
    #[serde(
        default = "default_backoff_multiplier"
    )]
    pub backoff_multiplier: f32,
}

/// Restart strategy enumeration
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
#[serde(rename_all = "snake_case")]
pub enum RestartStrategy {
    Never,
    OnFailure,
    Always,
}

/// Process logging configuration
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ProcessLoggingConfig {
    #[serde(default = "default_enabled")]
    pub capture_stdout: bool,
    #[serde(default = "default_enabled")]
    pub capture_stderr: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub log_file: Option<String>,
    #[serde(default = "default_buffer_size")]
    pub buffer_size: usize,
}

/// Log collection configuration (optional top-level)
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LogCollectionConfig {
    #[serde(default = "default_enabled")]
    pub enabled: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub global_aggregation: Option<GlobalAggregationConfig>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub enhancement: Option<LogEnhancementConfig>,
}

/// Global log aggregation configuration
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct GlobalAggregationConfig {
    #[serde(default = "default_enabled")]
    pub enabled: bool,
    #[serde(default)]
    pub targets: Vec<LogTargetConfig>,
}

/// Log target configuration
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LogTargetConfig {
    #[serde(rename = "type")]
    pub target_type: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub path: Option<String>,
    #[serde(default = "default_log_format")]
    pub format: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub prefix: Option<String>,
}

/// Log enhancement configuration
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LogEnhancementConfig {
    #[serde(default = "default_enabled")]
    pub enabled: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub metadata: Option<LogMetadataConfig>,
}

/// Log metadata configuration
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LogMetadataConfig {
    #[serde(default)]
    pub add_process_manager_id: bool,
    #[serde(default)]
    pub add_hostname: bool,
    #[serde(default)]
    pub add_timestamp: bool,
    #[serde(default)]
    pub add_sequence: bool,
    #[serde(default)]
    pub add_line_number: bool,
}

impl ProcessManagerConfig {
    /// Load configuration from a YAML file
    pub fn load_from_file<P: AsRef<Path>>(path: P) -> Result<Self> {
        let content = std::fs::read_to_string(&path)
            .with_context(|| format!("Failed to read config file: {}", path.as_ref().display()))?;
        
        Self::load_from_string(&content)
    }

    /// Load configuration from a YAML string
    pub fn load_from_string(content: &str) -> Result<Self> {
        let config: ProcessManagerConfig = serde_yaml::from_str(content)
            .context("Failed to parse YAML configuration")?;
        
        config.validate()?;
        Ok(config)
    }

    /// Validate the configuration
    pub fn validate(&self) -> Result<()> {
        validation::validate_config(self)
    }

    /// Get enabled processes only
    pub fn enabled_processes(&self) -> Vec<&ProcessConfig> {
        self.managed_processes
            .iter()
            .filter(|p| p.enabled)
            .collect()
    }
}

// Default value functions
fn default_log_level() -> String {
    "info".to_string()
}

fn default_force_shutdown_timeout() -> Duration {
    Duration::from_secs(30)
}

fn default_profile_type() -> String {
    "general".to_string()
}

fn default_enabled() -> bool {
    true
}

fn default_startup_timeout() -> Duration {
    Duration::from_secs(30)
}

fn default_shutdown_timeout() -> Duration {
    Duration::from_secs(10)
}

fn default_health_check_interval() -> Duration {
    Duration::from_secs(30)
}

fn default_health_check_timeout() -> Duration {
    Duration::from_secs(5)
}

fn default_failure_threshold() -> u32 {
    3
}

fn default_restart_strategy() -> RestartStrategy {
    RestartStrategy::OnFailure
}

fn default_max_attempts() -> u32 {
    3
}

fn default_restart_delay() -> Duration {
    Duration::from_secs(5)
}

fn default_backoff_multiplier() -> f32 {
    1.5
}

fn default_buffer_size() -> usize {
    8192
}

fn default_log_format() -> String {
    "plain".to_string()
}

// Custom serialization for Duration
mod duration_serde {
    use serde::{Deserialize, Deserializer, Serializer};
    use std::time::Duration;

    pub fn serialize<S>(duration: &Duration, serializer: S) -> Result<S::Ok, S::Error>
    where
        S: Serializer,
    {
        let secs = duration.as_secs();
        serializer.serialize_str(&format!("{}s", secs))
    }

    pub fn deserialize<'de, D>(deserializer: D) -> Result<Duration, D::Error>
    where
        D: Deserializer<'de>,
    {
        let s = String::deserialize(deserializer)?;
        parse_duration(&s).map_err(serde::de::Error::custom)
    }

    fn parse_duration(s: &str) -> Result<Duration, String> {
        if s.ends_with('s') {
            let num_str = &s[..s.len() - 1];
            let secs: u64 = num_str.parse().map_err(|_| format!("Invalid duration: {}", s))?;
            Ok(Duration::from_secs(secs))
        } else if s.ends_with("ms") {
            let num_str = &s[..s.len() - 2];
            let millis: u64 = num_str.parse().map_err(|_| format!("Invalid duration: {}", s))?;
            Ok(Duration::from_millis(millis))
        } else {
            Err(format!("Duration must end with 's' or 'ms': {}", s))
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_config_parsing() {
        let yaml = r#"
process_manager:
  port: 50055
  log_level: "info"
  force_shutdown_timeout: "30s"

managed_processes:
  - id: "test-process"
    type: "standard_managed"
    profile_type: "long_running_service"
    enabled: true
    management:
      control:
        executable: "./test-app"
        arguments: ["--config", "test.conf"]
        working_directory: "/opt/test"
        environment:
          TEST_MODE: "true"
        startup_timeout: "60s"
        shutdown_timeout: "10s"
      health_check:
        enabled: true
        interval: "30s"
        timeout: "5s"
        failure_threshold: 3
      resource_limits:
        max_memory_mb: 512
        max_cpu_percent: 50.0
      restart_policy:
        strategy: "on_failure"
        max_attempts: 3
        restart_delay: "5s"
        backoff_multiplier: 1.5
"#;

        let config = ProcessManagerConfig::load_from_string(yaml).unwrap();
        assert_eq!(config.process_manager.port, 50055);
        assert_eq!(config.managed_processes.len(), 1);
        
        let process = &config.managed_processes[0];
        assert_eq!(process.id, "test-process");
        assert_eq!(process.process_type, ProcessManagementType::StandardManaged);
        assert!(process.enabled);
        
        let control = &process.management.control;
        assert_eq!(control.executable, "./test-app");
        assert_eq!(control.arguments, vec!["--config", "test.conf"]);
    }
}
