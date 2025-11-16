//! # HSU Process
//!
//! Low-level process operations for the HSU framework.
//!
//! This crate provides cross-platform primitives for:
//! - Process spawning and control
//! - Signal handling
//! - Process termination
//! - Process state checking
//! - Process existence verification
//!
//! This corresponds to the Go package `pkg/process`.

pub mod check;
pub mod execute;
pub mod terminate;
pub mod validation;

// Re-export main types
pub use check::*;
pub use execute::*;
pub use terminate::*;
pub use validation::*;

