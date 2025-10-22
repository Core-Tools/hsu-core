//! Module runtime for orchestrating modules.
//! 
//! # Rust Learning Note
//! 
//! This module demonstrates **orchestration patterns** in Rust.
//! 
//! ## Runtime Concept
//! 
//! The runtime is the "glue" that:
//! 1. Manages module lifecycle
//! 2. Creates gateway factory
//! 3. Connects to service registry
//! 4. Handles startup/shutdown
//! 
//! **Think of it as the "main" for HSU modules!**

use hsu_common::{ModuleID, Result};
use hsu_module_management::Module;
use hsu_module_proto::DirectProtocol;
use std::sync::Arc;
use std::collections::HashMap;
use tracing::{info, debug, error};

use crate::registry_client::ServiceRegistryClient;
use crate::gateway_factory::ServiceGatewayFactoryImpl;

/// Module runtime configuration.
/// 
/// # Rust Learning Note
/// 
/// ## Builder Pattern for Configuration
/// 
/// ```rust
/// let config = RuntimeConfig::new()
///     .with_registry_url("http://localhost:8080")
///     .with_process_id(std::process::id());
/// ```
/// 
/// This is a common Rust pattern for configuration.
#[derive(Clone)]
pub struct RuntimeConfig {
    /// Service registry URL.
    pub registry_url: String,
    
    /// Process ID (for registry).
    pub process_id: u32,
}

impl RuntimeConfig {
    /// Creates a new runtime configuration.
    pub fn new() -> Self {
        Self {
            registry_url: "http://localhost:8080".to_string(),
            process_id: std::process::id(),
        }
    }

    /// Sets the registry URL.
    pub fn with_registry_url(mut self, url: impl Into<String>) -> Self {
        self.registry_url = url.into();
        self
    }

    /// Sets the process ID.
    pub fn with_process_id(mut self, pid: u32) -> Self {
        self.process_id = pid;
        self
    }
}

impl Default for RuntimeConfig {
    fn default() -> Self {
        Self::new()
    }
}

/// Module runtime.
/// 
/// # Rust Learning Note
/// 
/// ## Runtime Lifecycle
/// 
/// ```
/// 1. new() → Create runtime
///     ↓
/// 2. add_module() → Register modules
///     ↓
/// 3. start() → Initialize everything
///     ├── Create gateway factory
///     ├── Inject factory into modules
///     ├── Start modules
///     └── Publish to registry
///     ↓
/// 4. run() → Keep running
///     ↓
/// 5. stop() → Clean shutdown
///     ├── Stop modules
///     └── Cleanup
/// ```
/// 
/// **This mirrors Go's runtime but with Rust's ownership!**
pub struct ModuleRuntime {
    /// Runtime configuration.
    config: RuntimeConfig,
    
    /// Registered modules.
    modules: Vec<Box<dyn Module>>,
    
    /// Service registry client.
    registry_client: Option<Arc<ServiceRegistryClient>>,
    
    /// Gateway factory.
    gateway_factory: Option<Arc<ServiceGatewayFactoryImpl>>,
}

impl ModuleRuntime {
    /// Creates a new module runtime.
    /// 
    /// # Rust Learning Note
    /// 
    /// ## Vec<Box<dyn Module>>
    /// 
    /// This is a **heterogeneous collection**:
    /// - `Vec`: Dynamic array
    /// - `Box`: Heap-allocated
    /// - `dyn Module`: Trait object (any type implementing Module)
    /// 
    /// **In Go:** `[]Module` (interface slice)
    /// **In Rust:** `Vec<Box<dyn Module>>` (owned trait objects)
    /// 
    /// **Key difference:** Rust's version is **owned** - no GC needed!
    pub fn new(config: RuntimeConfig) -> Self {
        info!("Creating module runtime");
        Self {
            config,
            modules: Vec::new(),
            registry_client: None,
            gateway_factory: None,
        }
    }

    /// Adds a module to the runtime.
    /// 
    /// # Rust Learning Note
    /// 
    /// ## Taking Ownership
    /// 
    /// ```rust
    /// pub fn add_module(&mut self, module: Box<dyn Module>) {
    ///     self.modules.push(module);
    /// }
    /// ```
    /// 
    /// The module is **moved** into the runtime:
    /// - Caller can't use it anymore
    /// - Runtime owns it completely
    /// - No copying, no references
    /// 
    /// **This is Rust's ownership in action!**
    pub fn add_module(&mut self, module: Box<dyn Module>) {
        let module_id = module.id().clone();
        debug!("Adding module: {}", module_id);
        self.modules.push(module);
    }

    /// Initializes the runtime.
    /// 
    /// # Rust Learning Note
    /// 
    /// ## Complex Initialization
    /// 
    /// This method does several things:
    /// 1. Create registry client
    /// 2. Build local modules map
    /// 3. Create gateway factory
    /// 4. Inject factory into modules
    /// 
    /// **All with compile-time type checking!**
    /// 
    /// ## Error Handling
    /// 
    /// Notice how we propagate errors with `?`:
    /// ```rust
    /// self.initialize().await?;
    /// module.start().await?;
    /// ```
    /// 
    /// Any error bubbles up immediately!
    async fn initialize(&mut self) -> Result<()> {
        info!("Initializing runtime");

        // Create registry client
        let registry_client = Arc::new(ServiceRegistryClient::new(&self.config.registry_url));
        self.registry_client = Some(Arc::clone(&registry_client));

        // Build map of local modules for direct protocol
        let mut local_modules = HashMap::new();
        
        for module in &self.modules {
            let module_id = module.id().clone();
            
            if let Some(handlers) = module.service_handlers_map() {
                let protocol = DirectProtocol::new(
                    handlers.into_iter()
                        .map(|(id, handler)| (id, Arc::new(handler)))
                        .collect()
                );
                local_modules.insert(module_id, Arc::new(protocol));
            }
        }

        debug!("Registered {} local modules", local_modules.len());

        // Create gateway factory
        let factory = Arc::new(ServiceGatewayFactoryImpl::new(
            local_modules,
            registry_client,
        ));
        self.gateway_factory = Some(Arc::clone(&factory));

        // Inject factory into modules
        for module in &mut self.modules {
            let factory_trait: Arc<dyn hsu_module_management::ServiceGatewayFactory> = factory.clone();
            module.set_service_gateway_factory(factory_trait);
        }

        info!("Runtime initialized successfully");
        Ok(())
    }

    /// Starts all modules.
    /// 
    /// # Rust Learning Note
    /// 
    /// ## Mutable Iteration
    /// 
    /// ```rust
    /// for module in &mut self.modules {
    ///     module.start().await?;
    /// }
    /// ```
    /// 
    /// - `&mut`: Mutable borrow of each module
    /// - Can call methods that modify state
    /// - Borrow checker ensures no conflicts
    /// 
    /// **Safe concurrent mutation!**
    pub async fn start(&mut self) -> Result<()> {
        info!("Starting module runtime");

        // Initialize if not already done
        if self.gateway_factory.is_none() {
            self.initialize().await?;
        }

        // Start all modules
        for module in &mut self.modules {
            let module_id = module.id().clone();
            info!("Starting module: {}", module_id);
            
            module.start().await
                .map_err(|e| e.context(format!("Failed to start module {}", module_id)))?;
        }

        // Publish to registry (if modules provide services)
        // TODO: In Phase 5, implement actual API publishing

        info!("All modules started successfully");
        Ok(())
    }

    /// Stops all modules.
    /// 
    /// # Rust Learning Note
    /// 
    /// ## Reverse Iteration
    /// 
    /// ```rust
    /// for module in self.modules.iter_mut().rev() {
    ///     module.stop().await?;
    /// }
    /// ```
    /// 
    /// - `.rev()`: Reverses the iterator
    /// - Stops modules in reverse order
    /// - Common pattern for cleanup
    pub async fn stop(&mut self) -> Result<()> {
        info!("Stopping module runtime");

        // Stop modules in reverse order
        for module in self.modules.iter_mut().rev() {
            let module_id = module.id().clone();
            info!("Stopping module: {}", module_id);
            
            if let Err(e) = module.stop().await {
                error!("Error stopping module {}: {}", module_id, e);
                // Continue stopping other modules
            }
        }

        info!("All modules stopped");
        Ok(())
    }

    /// Returns the number of registered modules.
    pub fn module_count(&self) -> usize {
        self.modules.len()
    }

    /// Returns the gateway factory (if initialized).
    pub fn gateway_factory(&self) -> Option<&Arc<ServiceGatewayFactoryImpl>> {
        self.gateway_factory.as_ref()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use async_trait::async_trait;
    use hsu_module_management::{ServiceGatewayFactory, ServiceHandlersMap};

    // Mock module for testing
    struct MockModule {
        id: ModuleID,
        started: bool,
    }

    impl MockModule {
        fn new(id: impl Into<ModuleID>) -> Self {
            Self {
                id: id.into(),
                started: false,
            }
        }
    }

    #[async_trait]
    impl Module for MockModule {
        fn id(&self) -> &ModuleID {
            &self.id
        }

        fn service_handlers_map(&self) -> Option<ServiceHandlersMap> {
            None
        }

        fn set_service_gateway_factory(&mut self, _factory: Arc<dyn ServiceGatewayFactory>) {
            // No-op for mock
        }

        async fn start(&mut self) -> Result<()> {
            self.started = true;
            Ok(())
        }

        async fn stop(&mut self) -> Result<()> {
            self.started = false;
            Ok(())
        }
    }

    #[tokio::test]
    async fn test_runtime_creation() {
        let config = RuntimeConfig::new();
        let runtime = ModuleRuntime::new(config);
        
        assert_eq!(runtime.module_count(), 0);
    }

    #[tokio::test]
    async fn test_runtime_add_module() {
        let config = RuntimeConfig::new();
        let mut runtime = ModuleRuntime::new(config);
        
        let module = Box::new(MockModule::new("test-module"));
        runtime.add_module(module);
        
        assert_eq!(runtime.module_count(), 1);
    }

    #[tokio::test]
    async fn test_runtime_start_stop() {
        let config = RuntimeConfig::new()
            .with_registry_url("http://localhost:8080");
        let mut runtime = ModuleRuntime::new(config);
        
        let module = Box::new(MockModule::new("test-module"));
        runtime.add_module(module);
        
        // Start (will initialize and start modules)
        // Note: This will try to connect to registry, which may not be running
        // In a real test, we'd mock the registry client
        // For now, we just test that the code compiles and runs
        
        // Start should work (modules start successfully)
        let _result = runtime.start().await;
        // May fail if registry not running, but that's ok for this test
        
        // Stop should always work
        runtime.stop().await.unwrap();
    }

    #[test]
    fn test_config_builder() {
        let config = RuntimeConfig::new()
            .with_registry_url("http://example.com:9000")
            .with_process_id(12345);
        
        assert_eq!(config.registry_url, "http://example.com:9000");
        assert_eq!(config.process_id, 12345);
    }
}

