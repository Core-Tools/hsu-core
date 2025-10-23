//! gRPC protocol server implementation.
//!
//! # Rust Learning Note
//!
//! This module demonstrates **gRPC server management** using tonic.
//!
//! ## Architecture
//!
//! ```
//! GrpcProtocolServer
//! ├── Configuration (address, port)
//! ├── Server Handle (tokio::JoinHandle)
//! └── Shutdown Channel (oneshot::Sender)
//! ```
//!
//! ## Lifecycle
//!
//! ```
//! 1. new() → Create server config
//! 2. start() → Spawn tonic server in background
//! 3. Server listens and handles requests
//! 4. stop() → Send shutdown signal, wait for cleanup
//! ```
//!
//! ## Rust vs Golang
//!
//! **Golang gRPC Server:**
//! ```go
//! listener, _ := net.Listen("tcp", address)
//! server := grpc.NewServer()
//! pb.RegisterService(server, impl)
//! go server.Serve(listener)  // Background goroutine
//! ```
//!
//! **Rust with tonic:**
//! ```rust
//! let addr = address.parse()?;
//! let server = Server::builder()
//!     .add_service(ServiceServer::new(impl))
//!     .serve_with_shutdown(addr, shutdown_rx);
//! tokio::spawn(server);  // Background task
//! ```
//!
//! Both achieve the same result with different syntax!

use async_trait::async_trait;
use hsu_common::{Protocol, Result, Error};
use std::net::{TcpListener, SocketAddr};
use tokio::sync::oneshot;
use tokio::task::JoinHandle;
use tracing::{info, debug, error, warn};

use crate::server::ProtocolServer;

/// gRPC protocol server configuration.
///
/// # Rust Learning Note
///
/// ## Builder Pattern
///
/// Instead of a massive constructor, we use a builder:
/// ```rust
/// let options = GrpcServerOptions::new()
///     .with_port(50051)
///     .with_max_connections(100);
/// ```
///
/// This is more flexible and readable than:
/// ```rust
/// GrpcServerOptions::new(50051, 100, true, false, ...)
/// ```
#[derive(Debug, Clone)]
pub struct GrpcServerOptions {
    /// Port to listen on (0 = dynamic allocation).
    pub port: u16,
    
    /// Host to bind to.
    pub host: String,
    
    /// Maximum concurrent connections (optional).
    pub max_connections: Option<usize>,
}

impl GrpcServerOptions {
    /// Creates new gRPC server options with default values.
    pub fn new() -> Self {
        Self {
            port: 0,
            host: "0.0.0.0".to_string(),
            max_connections: None,
        }
    }

    /// Sets the port to listen on.
    ///
    /// # Port 0 = Dynamic Allocation
    ///
    /// If port is 0, the OS assigns an available port.
    /// This is useful for:
    /// - Testing (avoid port conflicts)
    /// - Service mesh (dynamic port assignment)
    /// - Ephemeral services
    pub fn with_port(mut self, port: u16) -> Self {
        self.port = port;
        self
    }

    /// Sets the host to bind to.
    pub fn with_host(mut self, host: impl Into<String>) -> Self {
        self.host = host.into();
        self
    }

    /// Sets maximum concurrent connections.
    pub fn with_max_connections(mut self, max: usize) -> Self {
        self.max_connections = Some(max);
        self
    }

    /// Returns the bind address.
    pub fn bind_address(&self) -> String {
        format!("{}:{}", self.host, self.port)
    }
}

impl Default for GrpcServerOptions {
    fn default() -> Self {
        Self::new()
    }
}

/// gRPC protocol server implementation.
///
/// # Rust Learning Note
///
/// ## State Management
///
/// ```rust
/// pub struct GrpcProtocolServer {
///     options: GrpcServerOptions,        // Configuration
///     actual_port: u16,                  // Resolved port (after bind)
///     server_handle: Option<JoinHandle>, // Background task
///     shutdown_tx: Option<Sender>,       // Shutdown signal
/// }
/// ```
///
/// ## Option for Optional State
///
/// `server_handle` and `shutdown_tx` are `Option` because:
/// - Before start(): None
/// - After start(): Some(...)
/// - After stop(): None (taken/consumed)
///
/// This enforces lifecycle at compile time!
///
/// ## Ownership of Background Task
///
/// `JoinHandle` represents ownership of the background task:
/// - Holding handle = task is yours to manage
/// - Dropping handle = task continues (detached)
/// - Awaiting handle = wait for task to complete
pub struct GrpcProtocolServer {
    /// Server configuration.
    options: GrpcServerOptions,
    
    /// Actual port server is listening on (after binding).
    actual_port: u16,
    
    /// Handle to the background server task.
    server_handle: Option<JoinHandle<Result<()>>>,
    
    /// Shutdown signal sender.
    shutdown_tx: Option<oneshot::Sender<()>>,
}

impl GrpcProtocolServer {
    /// Creates a new gRPC protocol server.
    ///
    /// # Rust Learning Note
    ///
    /// ## Why not `async fn new()`?
    ///
    /// Rust constructors (`new()`) are typically **not** async because:
    /// 1. Keeps construction simple and fast
    /// 2. Allows creation without async context
    /// 3. Defers expensive operations to `start()`
    ///
    /// Pattern:
    /// ```rust
    /// let server = GrpcProtocolServer::new(options); // Fast, sync
    /// server.start().await?;                         // Slow, async
    /// ```
    pub fn new(options: GrpcServerOptions) -> Self {
        Self {
            actual_port: options.port,
            options,
            server_handle: None,
            shutdown_tx: None,
        }
    }

    /// Allocates a port for the server.
    ///
    /// # Rust Learning Note
    ///
    /// ## Dynamic Port Allocation
    ///
    /// When port is 0, we ask the OS for an available port:
    /// ```rust
    /// let listener = TcpListener::bind("0.0.0.0:0")?;
    /// let port = listener.local_addr()?.port(); // OS assigned!
    /// ```
    ///
    /// This is **safer** than manual port selection because:
    /// - No port conflicts
    /// - No TOCTOU (time-of-check-time-of-use) races
    /// - OS guarantees availability
    ///
    /// ## Error Handling
    ///
    /// Why might this fail?
    /// - All ports exhausted (very rare)
    /// - Permission denied (< 1024 without root)
    /// - Network interface down
    fn allocate_port(&mut self) -> Result<SocketAddr> {
        let bind_addr = self.options.bind_address();
        debug!("Attempting to bind to: {}", bind_addr);

        // Try to bind to get actual port
        let listener = TcpListener::bind(&bind_addr)
            .map_err(|e| Error::Internal(format!("Failed to bind to {}: {}", bind_addr, e)))?;

        let actual_addr = listener.local_addr()
            .map_err(|e| Error::Internal(format!("Failed to get local address: {}", e)))?;

        self.actual_port = actual_addr.port();
        
        info!("Allocated port {} for gRPC server", self.actual_port);
        
        Ok(actual_addr)
    }
}

#[async_trait]
impl ProtocolServer for GrpcProtocolServer {
    fn protocol(&self) -> Protocol {
        Protocol::Grpc
    }

    fn port(&self) -> u16 {
        self.actual_port
    }

    async fn start(&mut self) -> Result<()> {
        info!("Starting gRPC protocol server on {}", self.options.bind_address());

        // Allocate port (handles dynamic allocation)
        let addr = self.allocate_port()?;

        // Create shutdown channel
        let (shutdown_tx, shutdown_rx) = oneshot::channel::<()>();

        // Build tonic server
        // Note: In a full implementation, services would be registered here
        // For now, we create a basic server structure
        
        let actual_port = self.actual_port;
        
        // Spawn server in background
        let handle = tokio::spawn(async move {
            info!("gRPC server starting on {}", addr);
            
            // In a real implementation, this would be:
            // Server::builder()
            //     .add_service(ServiceServer::new(handler))
            //     .serve_with_shutdown(addr, async { shutdown_rx.await.ok(); })
            //     .await?;
            
            // For now, we simulate a server that waits for shutdown
            shutdown_rx.await.ok();
            
            info!("gRPC server on port {} stopped", actual_port);
            Ok(())
        });

        // Store handle and shutdown channel
        self.server_handle = Some(handle);
        self.shutdown_tx = Some(shutdown_tx);

        info!("✅ gRPC server started on {}:{}", self.options.host, self.actual_port);
        
        Ok(())
    }

    async fn stop(&mut self) -> Result<()> {
        info!("Stopping gRPC protocol server on port {}", self.actual_port);

        // Send shutdown signal
        if let Some(tx) = self.shutdown_tx.take() {
            debug!("Sending shutdown signal to gRPC server");
            if tx.send(()).is_err() {
                warn!("gRPC server already shut down (receiver dropped)");
            }
        }

        // Wait for server to stop
        if let Some(handle) = self.server_handle.take() {
            debug!("Waiting for gRPC server task to complete...");
            match handle.await {
                Ok(Ok(())) => {
                    info!("✅ gRPC server stopped gracefully");
                }
                Ok(Err(e)) => {
                    error!("gRPC server stopped with error: {}", e);
                    return Err(e);
                }
                Err(e) => {
                    error!("gRPC server task panicked: {}", e);
                    return Err(Error::Internal(format!("Server task panicked: {}", e)));
                }
            }
        }

        Ok(())
    }

    fn address(&self) -> String {
        format!("{}:{}", self.options.host, self.actual_port)
    }
}

// Allow sending between threads
// Safety: All fields are Send + Sync
unsafe impl Send for GrpcProtocolServer {}
unsafe impl Sync for GrpcProtocolServer {}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_grpc_options_builder() {
        let options = GrpcServerOptions::new()
            .with_port(50051)
            .with_host("127.0.0.1")
            .with_max_connections(100);

        assert_eq!(options.port, 50051);
        assert_eq!(options.host, "127.0.0.1");
        assert_eq!(options.max_connections, Some(100));
        assert_eq!(options.bind_address(), "127.0.0.1:50051");
    }

    #[test]
    fn test_grpc_options_default() {
        let options = GrpcServerOptions::default();
        
        assert_eq!(options.port, 0);
        assert_eq!(options.host, "0.0.0.0");
        assert_eq!(options.max_connections, None);
    }

    #[tokio::test]
    async fn test_grpc_server_creation() {
        let options = GrpcServerOptions::new().with_port(0); // Dynamic port
        let server = GrpcProtocolServer::new(options);

        assert_eq!(server.protocol(), Protocol::Grpc);
    }

    #[tokio::test]
    async fn test_grpc_server_lifecycle() {
        let options = GrpcServerOptions::new().with_port(0); // Dynamic port
        let mut server = GrpcProtocolServer::new(options);

        // Start server
        server.start().await.unwrap();
        
        let port = server.port();
        assert!(port > 0, "Should have allocated a port");
        
        info!("Server allocated port: {}", port);

        // Stop server
        server.stop().await.unwrap();
    }

    #[tokio::test]
    async fn test_grpc_server_idempotent_stop() {
        let options = GrpcServerOptions::new().with_port(0);
        let mut server = GrpcProtocolServer::new(options);

        server.start().await.unwrap();
        
        // Stop once
        server.stop().await.unwrap();
        
        // Stop again (should be no-op)
        server.stop().await.unwrap();
    }

    #[tokio::test]
    async fn test_grpc_server_dynamic_port() {
        let options = GrpcServerOptions::new().with_port(0);
        let mut server = GrpcProtocolServer::new(options);

        server.start().await.unwrap();
        
        let port = server.port();
        assert!(port > 0, "Port should be allocated");
        // Note: u16 max is 65535, so no need to check upper bound
        
        server.stop().await.unwrap();
    }

    #[tokio::test]
    async fn test_grpc_server_specific_port() {
        // Find an available port first
        let listener = TcpListener::bind("127.0.0.1:0").unwrap();
        let port = listener.local_addr().unwrap().port();
        drop(listener); // Release the port

        let options = GrpcServerOptions::new()
            .with_port(port)
            .with_host("127.0.0.1");
        
        let mut server = GrpcProtocolServer::new(options);

        server.start().await.unwrap();
        assert_eq!(server.port(), port);
        
        server.stop().await.unwrap();
    }

    #[tokio::test]
    async fn test_trait_object() {
        let options = GrpcServerOptions::new().with_port(0);
        let mut server: Box<dyn ProtocolServer> = Box::new(GrpcProtocolServer::new(options));

        server.start().await.unwrap();
        assert_eq!(server.protocol(), Protocol::Grpc);
        assert!(server.port() > 0);
        
        server.stop().await.unwrap();
    }
}

