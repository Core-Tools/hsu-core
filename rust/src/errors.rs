use thiserror::Error;

/// Main error type for the HSU Process Manager
#[derive(Error, Debug)]
pub enum ProcessManagerError {
    #[error("Configuration error: {0}")]
    Config(#[from] anyhow::Error),

    #[error("Process error: {0}")]
    Process(#[from] ProcessError),

    #[error("IO error: {0}")]
    Io(#[from] std::io::Error),

    #[error("Serialization error: {0}")]
    Serialization(#[from] serde_yaml::Error),

    #[error("System error: {0}")]
    System(String),

    #[error("Timeout error: {0}")]
    Timeout(String),

    #[error("Network error: {0}")]
    Network(String),

    #[error("Permission error: {0}")]
    Permission(String),

    #[error("Resource limit exceeded: {0}")]
    ResourceLimit(String),

    #[error("Health check failed: {0}")]
    HealthCheck(String),

    #[error("Shutdown error: {0}")]
    Shutdown(String),
}

/// Process-specific error types
#[derive(Error, Debug)]
pub enum ProcessError {
    #[error("Process not found: {id}")]
    NotFound { id: String },

    #[error("Process already exists: {id}")]
    AlreadyExists { id: String },

    #[error("Process spawn failed: {id} - {reason}")]
    SpawnFailed { id: String, reason: String },

    #[error("Process start failed: {id} - {reason}")]
    StartFailed { id: String, reason: String },

    #[error("Process stop failed: {id} - {reason}")]
    StopFailed { id: String, reason: String },

    #[error("Process killed: {id} - {signal}")]
    Killed { id: String, signal: String },

    #[error("Process crashed: {id} - exit code {exit_code:?}")]
    Crashed { id: String, exit_code: Option<i32> },

    #[error("Process timeout: {id} - {operation}")]
    Timeout { id: String, operation: String },

    #[error("Process state error: {id} - expected {expected}, got {actual}")]
    InvalidState {
        id: String,
        expected: String,
        actual: String,
    },

    #[error("Process configuration error: {id} - {reason}")]
    Configuration { id: String, reason: String },

    #[error("Process health check failed: {id} - {reason}")]
    HealthCheckFailed { id: String, reason: String },

    #[error("Process resource limit exceeded: {id} - {resource}: {limit}")]
    ResourceLimitExceeded {
        id: String,
        resource: String,
        limit: String,
    },

    #[error("Process restart failed: {id} - attempt {attempt} of {max_attempts}")]
    RestartFailed {
        id: String,
        attempt: u32,
        max_attempts: u32,
    },

    #[error("Process operation not allowed: {id} - {operation} (state: {state})")]
    OperationNotAllowed {
        id: String,
        operation: String,
        state: String,
    },

    #[error("Process monitoring error: {id} - {reason}")]
    MonitoringError { id: String, reason: String },

    #[error("Process logging error: {id} - {reason}")]
    LoggingError { id: String, reason: String },

    #[error("Process gRPC error: {id} - {reason}")]
    GrpcError { id: String, reason: String },
}

/// Health check specific errors
#[derive(Error, Debug)]
pub enum HealthCheckError {
    #[error("Health check timeout: {id}")]
    Timeout { id: String },

    #[error("Health check connection failed: {id} - {reason}")]
    ConnectionFailed { id: String, reason: String },

    #[error("Health check invalid response: {id} - {response}")]
    InvalidResponse { id: String, response: String },

    #[error("Health check endpoint not configured: {id}")]
    NotConfigured { id: String },

    #[error("Health check service unavailable: {id}")]
    ServiceUnavailable { id: String },
}

/// Monitoring specific errors
#[derive(Error, Debug)]
pub enum MonitoringError {
    #[error("Failed to get system information: {reason}")]
    SystemInfo { reason: String },

    #[error("Failed to get process metrics: {id} - {reason}")]
    ProcessMetrics { id: String, reason: String },

    #[error("Resource monitoring failed: {id} - {resource} - {reason}")]
    ResourceMonitoring {
        id: String,
        resource: String,
        reason: String,
    },

    #[error("Metric collection failed: {metric} - {reason}")]
    MetricCollection { metric: String, reason: String },
}

/// Logging specific errors
#[derive(Error, Debug)]
pub enum LoggingError {
    #[error("Log file creation failed: {path} - {reason}")]
    FileCreation { path: String, reason: String },

    #[error("Log writing failed: {reason}")]
    WriteFailed { reason: String },

    #[error("Log rotation failed: {path} - {reason}")]
    RotationFailed { path: String, reason: String },

    #[error("Log buffer overflow: {process_id}")]
    BufferOverflow { process_id: String },

    #[error("Log format error: {format} - {reason}")]
    FormatError { format: String, reason: String },
}

/// Network/API specific errors
#[derive(Error, Debug)]
pub enum NetworkError {
    #[error("Server bind failed: {address} - {reason}")]
    BindFailed { address: String, reason: String },

    #[error("Client connection failed: {address} - {reason}")]
    ConnectionFailed { address: String, reason: String },

    #[error("Request processing failed: {request} - {reason}")]
    RequestFailed { request: String, reason: String },

    #[error("Authentication failed: {reason}")]
    AuthenticationFailed { reason: String },

    #[error("Authorization failed: {operation} - {reason}")]
    AuthorizationFailed { operation: String, reason: String },
}

// Helper functions for creating specific error types
impl ProcessError {
    pub fn not_found(id: impl Into<String>) -> Self {
        Self::NotFound { id: id.into() }
    }

    pub fn already_exists(id: impl Into<String>) -> Self {
        Self::AlreadyExists { id: id.into() }
    }

    pub fn spawn_failed(id: impl Into<String>, reason: impl Into<String>) -> Self {
        Self::SpawnFailed {
            id: id.into(),
            reason: reason.into(),
        }
    }

    pub fn start_failed(id: impl Into<String>, reason: impl Into<String>) -> Self {
        Self::StartFailed {
            id: id.into(),
            reason: reason.into(),
        }
    }

    pub fn stop_failed(id: impl Into<String>, reason: impl Into<String>) -> Self {
        Self::StopFailed {
            id: id.into(),
            reason: reason.into(),
        }
    }

    pub fn timeout(id: impl Into<String>, operation: impl Into<String>) -> Self {
        Self::Timeout {
            id: id.into(),
            operation: operation.into(),
        }
    }

    pub fn invalid_state(
        id: impl Into<String>,
        expected: impl Into<String>,
        actual: impl Into<String>,
    ) -> Self {
        Self::InvalidState {
            id: id.into(),
            expected: expected.into(),
            actual: actual.into(),
        }
    }

    pub fn operation_not_allowed(
        id: impl Into<String>,
        operation: impl Into<String>,
        state: impl Into<String>,
    ) -> Self {
        Self::OperationNotAllowed {
            id: id.into(),
            operation: operation.into(),
            state: state.into(),
        }
    }
}

// Result type aliases for convenience
pub type Result<T> = std::result::Result<T, ProcessManagerError>;
pub type ProcessResult<T> = std::result::Result<T, ProcessError>;
pub type HealthCheckResult<T> = std::result::Result<T, HealthCheckError>;
pub type MonitoringResult<T> = std::result::Result<T, MonitoringError>;
pub type LoggingResult<T> = std::result::Result<T, LoggingError>;
pub type NetworkResult<T> = std::result::Result<T, NetworkError>;

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_process_error_construction() {
        let error = ProcessError::not_found("test-process");
        assert!(matches!(error, ProcessError::NotFound { .. }));
        assert_eq!(format!("{}", error), "Process not found: test-process");

        let error = ProcessError::spawn_failed("test-process", "executable not found");
        assert!(matches!(error, ProcessError::SpawnFailed { .. }));
        assert!(format!("{}", error).contains("spawn failed"));
    }

    #[test]
    fn test_error_conversion() {
        let io_error = std::io::Error::new(std::io::ErrorKind::NotFound, "file not found");
        let manager_error: ProcessManagerError = io_error.into();
        assert!(matches!(manager_error, ProcessManagerError::Io(_)));
    }
}
