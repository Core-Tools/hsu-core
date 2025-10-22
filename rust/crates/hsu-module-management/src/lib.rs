//! # HSU Module Management
//! 
//! This crate provides the core module management functionality,
//! including module traits, lifecycle management, and the runtime
//! orchestration layer.

pub mod module_types;
pub mod lifecycle;

// Re-export commonly used items
pub use module_types::{
    Module,
    ServiceHandler,
    ServiceGateway,
    ServiceHandlersMap,
    ServiceGatewayFactory,
};
pub use lifecycle::Lifecycle;

