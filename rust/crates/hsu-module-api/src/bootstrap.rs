//! Bootstrap - Module Wiring and Runtime Initialization
//!
//! This module provides the `run_with_config()` function that orchestrates
//! the entire module runtime lifecycle.
//!
//! ## Architecture
//!
//! ```text
//! run_with_config(config)
//!     ↓
//! 1. Create ServiceConnector
//! 2. For each enabled module:
//!    a. Get ModuleDescriptor from registry
//!    b. Create ServiceProvider
//!    c. Create Module instance
//!    d. Register handlers (if server module)
//!    e. Enable direct closure (if available)
//! 3. Start all modules
//! 4. Wait for Ctrl+C
//! 5. Stop all modules gracefully
//! ```
//!
//! ## Comparison with Golang
//!
//! This is the Rust equivalent of:
//! - `hsu-core/go/pkg/modulemanagement/modulewiring/bootstrap.go`
//! - Function: `RunWithConfig(cfg *Config, logger logging.Logger) error`
//!
//! **Key differences:**
//! - Rust: Async/await (Golang: goroutines)
//! - Rust: Result<()> (Golang: error)
//! - Rust: tracing (Golang: logger parameter)

use std::sync::Arc;
use hsu_common::Result;
use tracing::{info, debug, error};

use crate::{
    Config,
    Module,
    service_connector::ServiceConnectorImpl,
    registry::{get_module_descriptor, create_module_from_descriptor},
    ServiceRegistryClient,
};

/// Runs the module runtime with the given configuration.
///
/// This is the main entry point for running HSU modules.
///
/// # Arguments
///
/// * `config` - Runtime and module configuration
///
/// # Returns
///
/// * `Ok(())` - Runtime completed successfully
/// * `Err(_)` - Runtime encountered an error
///
/// # Example
///
/// ```rust,no_run
/// use hsu_module_api::{Config, ModuleConfig, run_with_config};
/// use hsu_common::ModuleID;
///
/// #[tokio::main]
/// async fn main() -> hsu_common::Result<()> {
///     // Register modules first
///     echo_server::init_echo_server_module(Default::default())?;
///     echo_client::init_echo_client_module(Default::default())?;
///     
///     // Configure and run
///     let config = Config {
///         runtime: Default::default(),
///         modules: vec![
///             ModuleConfig {
///                 id: ModuleID::from("echo"),
///                 enabled: true,
///                 servers: vec![],
///             },
///             ModuleConfig {
///                 id: ModuleID::from("echo-client"),
///                 enabled: true,
///                 servers: vec![],
///             },
///         ],
///     };
///     
///     run_with_config(config).await
/// }
/// ```
pub async fn run_with_config(config: Config) -> Result<()> {
    info!("🚀 Starting HSU Module Runtime");
    info!("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━");
    
    // Step 1: Create service registry client
    debug!("[Bootstrap] Creating service registry client");
    let registry_client = Arc::new(ServiceRegistryClient::new(&config.runtime.service_registry.url));
    info!("✅ Service registry client created: {}", config.runtime.service_registry.url);
    
    // Step 2: Create service connector
    debug!("[Bootstrap] Creating service connector");
    let service_connector = Arc::new(ServiceConnectorImpl::new(registry_client));
    info!("✅ Service connector created");
    
    // Step 3: Create service providers FIRST (matches Golang!)
    //
    // IMPORTANT: We create service providers in a SEPARATE step before creating modules.
    // This matches Golang's bootstrap.go flow and is architecturally correct.
    //
    // Why separate steps?
    // - Service providers are created independently
    // - Modules are then created using those service providers
    // - Allows for future features like service gateway maps, protocol server registration
    debug!("[Bootstrap] Creating service providers for {} modules", config.modules.len());
    use std::collections::HashMap;
    use crate::create_service_provider;
    
    let mut service_provider_map: HashMap<hsu_common::ModuleID, crate::ServiceProviderHandle> = HashMap::new();
    
    for module_config in &config.modules {
        if !module_config.enabled {
            debug!("[Bootstrap] Skipping disabled module: {}", module_config.id);
            continue;
        }
        
        info!("[Bootstrap] Creating service provider for module: {}", module_config.id);
        
        // Create service provider options
        let options = crate::module_descriptor::CreateServiceProviderOptions {
            service_connector: service_connector.clone(),
        };
        
        // Create service provider from registry
        let service_provider_handle = create_service_provider(&module_config.id, options)?;
        debug!("[Bootstrap]   - Service provider created");
        
        service_provider_map.insert(module_config.id.clone(), service_provider_handle);
        info!("✅ Service provider for '{}' created", module_config.id);
    }
    
    info!("✅ All {} service providers created", service_provider_map.len());
    
    // Step 4: Create modules using service providers
    debug!("[Bootstrap] Creating {} modules", config.modules.len());
    let mut modules: Vec<Box<dyn Module>> = Vec::new();
    
    for module_config in &config.modules {
        if !module_config.enabled {
            continue;
        }
        
        info!("[Bootstrap] Creating module: {}", module_config.id);
        
        // Get service provider for this module
        let _service_provider = service_provider_map.get(&module_config.id)
            .ok_or_else(|| hsu_common::Error::Internal(
                format!("Service provider not found for module '{}'", module_config.id)
            ))?;
        debug!("[Bootstrap]   - Got service provider");
        
        // Create module from registry (uses service provider internally)
        let (module, _handlers) = create_module_from_descriptor(
            &module_config.id,
            service_connector.clone(),
        )?;
        debug!("[Bootstrap]   - Module instance created");
        
        // TODO: Register handlers with protocol servers (Phase 10)
        // TODO: Enable direct closure if available (Phase 10)
        
        modules.push(module);
        info!("✅ Module '{}' created successfully", module_config.id);
    }
    
    info!("✅ All {} modules created", modules.len());
    
    // Step 5: Start all modules
    info!("\n🔄 Starting modules...");
    for module in &mut modules {
        let module_id = module.id().clone();
        debug!("[Bootstrap] Starting module: {}", module_id);
        
        module.start().await?;
        
        info!("✅ Module '{}' started", module_id);
    }
    
    info!("\n🎉 All modules started successfully!");
    info!("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━");
    info!("📝 Runtime is operational");
    info!("Press Ctrl+C to stop...\n");
    
    // Step 6: Wait for shutdown signal (Ctrl+C)
    tokio::signal::ctrl_c().await
        .map_err(|e| hsu_common::Error::Internal(format!("Signal error: {}", e)))?;
    
    info!("\n📋 Shutting down gracefully...");
    
    // Step 7: Stop all modules in reverse order
    info!("🔄 Stopping modules...");
    for module in modules.iter_mut().rev() {
        let module_id = module.id().clone();
        debug!("[Bootstrap] Stopping module: {}", module_id);
        
        if let Err(e) = module.stop().await {
            error!("❌ Error stopping module '{}': {}", module_id, e);
        } else {
            info!("✅ Module '{}' stopped", module_id);
        }
    }
    
    info!("\n✅ Clean shutdown complete!");
    info!("👋 Goodbye!\n");
    
    Ok(())
}

