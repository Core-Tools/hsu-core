// HSU Process Manager Library
// 
// This library provides process management capabilities for the HSU framework,
// including process lifecycle management, health monitoring, resource limits,
// and configuration management.

pub mod api;
pub mod config;
pub mod errors;
pub mod health;
pub mod logging;
pub mod monitoring;
pub mod process;
pub mod utils;

// Re-export main types for easy use
pub use config::{ProcessConfig, ProcessManagerConfig, ProcessManagementType};
pub use errors::{ProcessError, ProcessManagerError};
pub use process::{ProcessManager, ProcessState};

/// Library version
pub const VERSION: &str = env!("CARGO_PKG_VERSION");

/// Library name
pub const NAME: &str = env!("CARGO_PKG_NAME");
