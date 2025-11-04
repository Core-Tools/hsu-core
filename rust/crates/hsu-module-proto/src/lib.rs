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
pub mod server;
pub mod grpc_server;
pub mod server_manager;
pub mod client_connection;

// Re-export commonly used items
pub use traits::{ProtocolHandler, ProtocolGateway};
pub use direct::DirectProtocol;
pub use options::{ProtocolOptions, GrpcOptions};
pub use server::ProtocolServer;
pub use grpc_server::{GrpcProtocolServer, GrpcServerOptions};
pub use server_manager::{ServerManager, ServerID};
pub use client_connection::{ClientConnection, ClientConnectionVisitor};
pub use grpc::{GrpcProtocolGateway, GrpcProtocolHandler};
