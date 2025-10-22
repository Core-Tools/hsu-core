//! # HSU Module Proto
//! 
//! Protocol abstraction layer for inter-module communication.
//! 
//! This crate provides protocol implementations for different communication methods:
//! - **Direct**: In-process, zero-cost function calls
//! - **gRPC**: Cross-process communication using tonic
//! - **HTTP**: Cross-process communication using HTTP (future)

pub mod traits;
pub mod direct;
pub mod grpc;
pub mod options;

// Re-export commonly used items
pub use traits::{ProtocolHandler, ProtocolGateway};
pub use direct::DirectProtocol;
pub use options::{ProtocolOptions, GrpcOptions};
