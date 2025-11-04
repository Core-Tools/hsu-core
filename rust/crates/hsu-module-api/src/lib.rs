//! # HSU Module API
//! 
//! Module API layer including service registry client and gateway factory.
//! 
//! This crate provides the glue between modules, protocols, and the service registry:
//! - Service registry client for discovering services
//! - Gateway factory for creating protocol-specific gateways
//! - Auto-protocol selection (direct if available, otherwise remote)
//! - Module runtime for orchestration

pub mod registry_client;
pub mod gateway_factory;
pub mod runtime;
pub mod service_connector;
pub mod gateway_factory_typed;

// Re-export commonly used items
pub use registry_client::ServiceRegistryClient;
pub use gateway_factory::ServiceGatewayFactoryImpl;
pub use runtime::{ModuleRuntime, RuntimeConfig};
pub use service_connector::{ServiceConnector, ServiceConnectorImpl};
pub use gateway_factory_typed::{ServiceGatewayFactory, GatewayFactoryFuncs};
