//! Error types for the HSU framework.
//! 
//! # Rust Learning Note
//! 
//! Rust doesn't have exceptions - it uses `Result<T, E>` for error handling.
//! This module defines the error types used throughout HSU.
//! 
//! ## Go Comparison
//! ```go
//! // Go: Return error
//! func doSomething() error {
//!     return errors.New("something went wrong")
//! }
//! 
//! // Caller checks
//! if err := doSomething(); err != nil {
//!     return err
//! }
//! ```
//! 
//! ## Rust
//! ```rust
//! use hsu_common::{Error, Result};
//! 
//! // Rust: Return Result
//! fn do_something() -> Result<()> {
//!     Err(Error::not_found("thing"))
//! }
//! 
//! fn caller() -> Result<()> {
//!     // Caller uses ? operator (automatic propagation)
//!     let result = do_something()?;
//!     Ok(())
//! }
//! ```

use thiserror::Error;
use crate::types::{ModuleID, ServiceID};

/// Result type alias for HSU operations.
/// 
/// This is a convenience alias so we can write `Result<T>` instead of
/// `Result<T, Error>` throughout the codebase.
pub type Result<T> = std::result::Result<T, Error>;

/// Main error type for HSU operations.
/// 
/// # Rust Learning Note
/// 
/// We use the `thiserror` crate to automatically derive error traits.
/// Each variant can carry additional context data.
/// 
/// ## Key Advantages over Go
/// - Type-safe error variants (can't mix up error types)
/// - Pattern matching on errors (exhaustive checking)
/// - Automatic error message formatting
/// - Zero-cost compared to Go errors (no allocations for simple errors)
#[derive(Debug, Error)]
pub enum Error {
    /// A requested resource was not found.
    #[error("Not found: {resource}")]
    NotFound {
        resource: String,
    },

    /// Invalid input or configuration.
    #[error("Validation error: {message}")]
    Validation {
        message: String,
    },

    /// A module was not found.
    #[error("Module not found: {module_id}")]
    ModuleNotFound {
        module_id: ModuleID,
    },

    /// A service was not found.
    #[error("Service not found: module={module_id}, service={service_id}")]
    ServiceNotFound {
        module_id: ModuleID,
        service_id: ServiceID,
    },

    /// Wrong service type (e.g., expected Echo, got Storage).
    #[error("Wrong service type: expected {expected}, got {actual}")]
    WrongServiceType {
        expected: String,
        actual: String,
    },

    /// Wrong protocol type.
    #[error("Wrong protocol: expected {expected}, got {actual}")]
    WrongProtocol {
        expected: String,
        actual: String,
    },

    /// Service registry error.
    #[error("Registry error: {0}")]
    Registry(String),

    /// Protocol error (e.g., gRPC connection failed).
    #[error("Protocol error: {0}")]
    Protocol(String),

    /// Internal error (shouldn't happen in normal operation).
    #[error("Internal error: {0}")]
    Internal(String),

    /// I/O error (wraps std::io::Error).
    #[error("I/O error: {0}")]
    Io(#[from] std::io::Error),

    /// Generic error with context.
    #[error("{message}: {source}")]
    WithContext {
        message: String,
        source: Box<Error>,
    },
}

impl Error {
    /// Creates a NotFound error.
    pub fn not_found(resource: impl Into<String>) -> Self {
        Self::NotFound {
            resource: resource.into(),
        }
    }

    /// Creates a Validation error.
    pub fn validation(message: impl Into<String>) -> Self {
        Self::Validation {
            message: message.into(),
        }
    }

    /// Creates a module not found error.
    pub fn module_not_found(module_id: ModuleID) -> Self {
        Self::ModuleNotFound { module_id }
    }

    /// Creates a service not found error.
    pub fn service_not_found(module_id: ModuleID, service_id: ServiceID) -> Self {
        Self::ServiceNotFound {
            module_id,
            service_id,
        }
    }

    /// Creates a wrong service type error.
    pub fn wrong_service_type(expected: impl Into<String>, actual: impl Into<String>) -> Self {
        Self::WrongServiceType {
            expected: expected.into(),
            actual: actual.into(),
        }
    }

    /// Creates a wrong protocol error.
    pub fn wrong_protocol(expected: impl Into<String>, actual: impl Into<String>) -> Self {
        Self::WrongProtocol {
            expected: expected.into(),
            actual: actual.into(),
        }
    }

    /// Adds context to an error.
    /// 
    /// # Example
    /// ```
    /// use hsu_common::{Error, Result};
    /// 
    /// fn inner() -> Result<()> {
    ///     Err(Error::not_found("file"))
    /// }
    /// 
    /// fn outer() -> Result<()> {
    ///     inner().map_err(|e| e.context("Failed to load config"))
    /// }
    /// ```
    pub fn context(self, message: impl Into<String>) -> Self {
        Self::WithContext {
            message: message.into(),
            source: Box::new(self),
        }
    }
}

// Convenience methods for Result types
pub trait ResultExt<T> {
    /// Adds context to an error result.
    fn context(self, message: impl Into<String>) -> Result<T>;
}

impl<T> ResultExt<T> for Result<T> {
    fn context(self, message: impl Into<String>) -> Result<T> {
        self.map_err(|e| e.context(message))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_error_creation() {
        let err = Error::not_found("test");
        assert!(matches!(err, Error::NotFound { .. }));
    }

    #[test]
    fn test_error_context() {
        let err = Error::not_found("resource")
            .context("Operation failed");
        
        let error_message = err.to_string();
        assert!(error_message.contains("Operation failed"));
    }

    #[test]
    fn test_error_pattern_matching() {
        let err = Error::module_not_found(ModuleID::from("test"));
        
        match err {
            Error::ModuleNotFound { module_id } => {
                assert_eq!(module_id.as_str(), "test");
            }
            _ => panic!("Wrong error type"),
        }
    }
}

