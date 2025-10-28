//! Core module types and abstractions.
//! 
//! # Rust Learning Note
//! 
//! This module demonstrates the key difference between Go and Rust:
//! - Go uses `interface{}` for protocol-agnostic abstractions
//! - Rust uses **enums** (sum types) for type-safe variants
//! 
//! ## The Key Pattern: Enums Instead of interface{}
//! 
//! ```go
//! // Go: Type erasure with interface{}
//! type ServiceHandler interface{}  // Can be anything!
//! 
//! handler := handlers[id]
//! concreteHandler := handler.(ConcreteType)  // Runtime cast
//! ```
//! 
//! ```rust,ignore
//! // Rust: Sum type with enum
//! enum ServiceHandler {
//!     Echo(EchoHandler),    // Variant 1
//!     Storage(StorageHandler),  // Variant 2
//! }
//! 
//! let handler = handlers.get(&id)?;
//! match handler {
//!     ServiceHandler::Echo(echo) => { /* echo is the right type! */ }
//!     ServiceHandler::Storage(storage) => { /* storage is the right type! */ }
//! }  // Compiler checks exhaustiveness!
//! ```

use async_trait::async_trait;
use hsu_common::{ModuleID, ServiceID, Protocol, Result};
use std::collections::HashMap;
use std::sync::Arc;

/// Service handler - server-side abstraction for a service.
/// 
/// # Rust Learning Note
/// 
/// This enum demonstrates how Rust replaces Go's `interface{}` with
/// type-safe sum types. Each variant represents a different service type.
/// 
/// ## Why Arc<T>?
/// 
/// `Arc` (Atomic Reference Counted) allows multiple owners to share
/// the same data safely. It's like `shared_ptr` in C++ or `Rc` in Rust's
/// single-threaded contexts.
/// 
/// We use `Arc` because:
/// 1. Handlers are shared between runtime and modules
/// 2. Arc is thread-safe (Send + Sync)
/// 3. Cloning Arc is cheap (just incrementing a counter)
/// 
/// ## Adding New Service Types
/// 
/// To add a new service type:
/// 1. Add a variant: `NewService(Arc<NewServiceHandler>)`
/// 2. Compiler will show you all places that need updating!
/// 3. Update all `match` expressions
/// 
/// This is **impossible to forget** - the compiler enforces it!
#[derive(Clone)]
pub enum ServiceHandler {
    /// Echo service handler (for example purposes).
    /// 
    /// In a real application, this would be your domain-specific service.
    Echo(Arc<dyn EchoService>),
    
    // Add more service types here:
    // Storage(Arc<dyn StorageService>),
    // Compute(Arc<dyn ComputeService>),
}

impl std::fmt::Debug for ServiceHandler {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            ServiceHandler::Echo(_) => write!(f, "ServiceHandler::Echo"),
        }
    }
}

/// Service gateway - client-side abstraction for a service.
/// 
/// # Rust Learning Note
/// 
/// This enum represents how to reach a service:
/// - `Direct`: In-process call (zero overhead)
/// - `Grpc`: Cross-process gRPC call
/// - `Http`: Cross-process HTTP call
/// 
/// The key insight: the **gateway type** tells you the communication method,
/// and the **inner type** tells you which service it talks to.
/// 
/// ## Memory Layout
/// 
/// ```text
/// ServiceGateway in memory:
/// [discriminant: u8][data: actual gateway]
///     ^                    ^
///     0 = Direct           If 0, contains ServiceHandler
///     1 = Grpc             If 1, contains GrpcGateway
///     2 = Http             If 2, contains HttpGateway
/// ```
/// 
/// Pattern matching on this is just an integer comparison - very fast!
#[derive(Clone)]
pub enum ServiceGateway {
    /// Direct in-process gateway (holds the actual handler).
    Direct(Arc<ServiceHandler>),
    
    /// gRPC gateway for cross-process communication.
    Grpc(GrpcGateway),
    
    /// HTTP gateway for cross-process communication.
    Http(HttpGateway),
}

impl std::fmt::Debug for ServiceGateway {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            ServiceGateway::Direct(handler) => write!(f, "ServiceGateway::Direct({:?})", handler),
            ServiceGateway::Grpc(gateway) => write!(f, "ServiceGateway::Grpc({:?})", gateway),
            ServiceGateway::Http(gateway) => write!(f, "ServiceGateway::Http({:?})", gateway),
        }
    }
}

// No domain-specific methods here!
// Framework stays domain-agnostic.
//
// For Go-like simplicity, use extension traits in your application code.
// See the echo example for the pattern.

/// gRPC gateway variants for different services.
/// 
/// # Rust Learning Note
/// 
/// This is a **nested enum** - each gRPC gateway variant can be
/// a different service type. This maintains type safety all the way down!
#[derive(Clone)]
pub enum GrpcGateway {
    /// Echo service gRPC client.
    Echo(Arc<dyn EchoService>),
    
    // Add more service types:
    // Storage(Arc<dyn StorageService>),
}

impl std::fmt::Debug for GrpcGateway {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            GrpcGateway::Echo(_) => write!(f, "GrpcGateway::Echo"),
        }
    }
}

/// HTTP gateway variants (placeholder for future).
#[derive(Clone)]
pub enum HttpGateway {
    /// Placeholder for HTTP gateway.
    _Placeholder,
}

impl std::fmt::Debug for HttpGateway {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "HttpGateway::_Placeholder")
    }
}

/// Map of service IDs to service handlers.
/// 
/// # Rust Learning Note
/// 
/// Unlike Go's `map[ServiceID]interface{}`, this HashMap stores
/// **ServiceHandler enums**, not `dyn Any`. This means:
/// - Type-safe: Can't accidentally store wrong type
/// - Pattern matching: Extract the right variant with match
/// - No unsafe: No downcasting needed!
pub type ServiceHandlersMap = HashMap<ServiceID, ServiceHandler>;

/// Factory for creating service gateways.
/// 
/// # Rust Learning Note
/// 
/// This is a **trait** (similar to Go's interface). Unlike Go's empty
/// interface, Rust traits define actual methods that must be implemented.
/// 
/// ## async_trait
/// 
/// The `#[async_trait]` macro allows us to use `async fn` in traits.
/// Without it, async methods in traits aren't supported yet in stable Rust.
#[async_trait]
pub trait ServiceGatewayFactory: Send + Sync {
    /// Creates a new service gateway for the specified module and service.
    /// 
    /// # Arguments
    /// - `module_id`: The target module
    /// - `service_id`: The target service within the module
    /// - `protocol`: The desired communication protocol
    /// 
    /// # Returns
    /// A `ServiceGateway` enum variant representing how to reach the service.
    /// 
    /// # Rust Learning Note
    /// 
    /// This returns `Result<ServiceGateway>`, not `Result<Box<dyn Any>>`.
    /// The caller knows exactly what they're getting - a ServiceGateway enum!
    async fn new_service_gateway(
        &self,
        module_id: &ModuleID,
        service_id: &ServiceID,
        protocol: Protocol,
    ) -> Result<ServiceGateway>;
}

/// User-provided factory for creating protocol-specific service gateways.
/// 
/// # Rust Learning Note
/// 
/// This trait allows users to provide their own gateway creation logic,
/// similar to Go's `GatewayFactoryFunc` pattern.
/// 
/// ## Pattern: User-Provided Factory
/// 
/// **Go equivalent:**
/// ```go
/// type ProtocolServiceGatewayFactoryFunc func(
///     protocolClientConnection moduleproto.ProtocolClientConnection,
///     logger logging.Logger,
/// ) moduletypes.ServiceGateway
/// 
/// // User provides:
/// GatewayFactoryFunc: grpcapi.NewGRPCGateway1
/// ```
/// 
/// **Rust version:**
/// ```rust
/// #[async_trait]
/// impl ProtocolGatewayFactory for EchoGrpcGatewayFactory {
///     async fn create_gateway(&self, address: String) -> Result<ServiceGateway> {
///         let gateway = EchoGrpcGateway::connect(address).await?;
///         Ok(ServiceGateway::Grpc(GrpcGateway::Echo(Arc::new(gateway))))
///     }
/// }
/// ```
/// 
/// ## Why This Design?
/// 
/// 1. **Type Safety**: User creates properly-typed gateway
/// 2. **Flexibility**: User controls connection/initialization
/// 3. **Testability**: Easy to mock factories
/// 4. **Async-friendly**: Native async support via async_trait
/// 
/// ## Comparison to ServiceGatewayFactory
/// 
/// - `ServiceGatewayFactory`: Framework-level, handles discovery & routing
/// - `ProtocolGatewayFactory`: User-level, creates service-specific gateways
/// 
/// The framework calls user factories to create the actual gateway implementations.
#[async_trait]
pub trait ProtocolGatewayFactory: Send + Sync {
    /// Creates a service-specific gateway for the given address.
    /// 
    /// # Arguments
    /// - `address`: The service address (from service discovery)
    /// 
    /// # Returns
    /// A `ServiceGateway` with the concrete service implementation.
    /// 
    /// # Example
    /// 
    /// ```rust,ignore
    /// use hsu_module_management::ProtocolGatewayFactory;
    /// 
    /// pub struct EchoGrpcGatewayFactory;
    /// 
    /// #[async_trait]
    /// impl ProtocolGatewayFactory for EchoGrpcGatewayFactory {
    ///     async fn create_gateway(&self, address: String) -> Result<ServiceGateway> {
    ///         // Connect to service
    ///         let gateway = EchoGrpcGateway::connect(address).await?;
    ///         
    ///         // Wrap in ServiceGateway enum
    ///         Ok(ServiceGateway::Grpc(GrpcGateway::Echo(Arc::new(gateway))))
    ///     }
    /// }
    /// ```
    async fn create_gateway(&self, address: String) -> Result<ServiceGateway>;
}

/// Module trait - represents a module in the HSU system.
/// 
/// # Rust Learning Note
/// 
/// This trait is equivalent to Go's Module interface, but with some
/// Rust-specific additions:
/// - `Send + Sync`: Required for thread safety (can be shared across threads)
/// - `async_trait`: Allows async methods
#[async_trait]
pub trait Module: Send + Sync {
    /// Returns the module's unique identifier.
    fn id(&self) -> &ModuleID;

    /// Returns the map of service handlers this module provides.
    /// 
    /// # Rust Learning Note
    /// 
    /// Returning `Option<ServiceHandlersMap>` allows modules to be
    /// client-only (no services provided). This is more explicit than
    /// Go's returning nil.
    fn service_handlers_map(&self) -> Option<ServiceHandlersMap>;

    /// Sets the service gateway factory for this module.
    /// 
    /// The factory is used to create gateways to other modules' services.
    fn set_service_gateway_factory(&mut self, factory: Arc<dyn ServiceGatewayFactory>);

    /// Starts the module.
    async fn start(&mut self) -> Result<()>;

    /// Stops the module gracefully.
    async fn stop(&mut self) -> Result<()>;
}

// Example service trait (will be in echo example)
// This demonstrates the pattern for domain-specific service traits

/// Echo service trait - example service interface.
/// 
/// # Rust Learning Note
/// 
/// This is similar to Go's contract.Contract1, but:
/// - Uses `async fn` for async operations
/// - Uses `Result<T>` for error handling (no separate error return)
/// - Requires `Send + Sync` for thread safety
#[async_trait]
pub trait EchoService: Send + Sync {
    /// Echo method - returns the input message.
    async fn echo(&self, message: String) -> Result<String>;
}

#[cfg(test)]
mod tests {
    use super::*;

    // Mock Echo service for testing
    struct MockEchoService;

    #[async_trait]
    impl EchoService for MockEchoService {
        async fn echo(&self, message: String) -> Result<String> {
            Ok(message)
        }
    }

    #[test]
    fn test_service_handler_enum() {
        // Create a handler
        let handler = ServiceHandler::Echo(Arc::new(MockEchoService));
        
        // Pattern match on it
        match handler {
            ServiceHandler::Echo(_) => {
                // Success - we got the Echo variant
            }
        }
    }

    #[test]
    fn test_service_gateway_enum() {
        // Create a direct gateway
        let handler = ServiceHandler::Echo(Arc::new(MockEchoService));
        let gateway = ServiceGateway::Direct(Arc::new(handler));
        
        // Pattern match
        match gateway {
            ServiceGateway::Direct(_) => {
                // Success - it's a direct gateway
            }
            _ => panic!("Expected direct gateway"),
        }
    }

    #[tokio::test]
    async fn test_echo_service() {
        let service = MockEchoService;
        let result = service.echo("test".to_string()).await.unwrap();
        assert_eq!(result, "test");
    }
}

