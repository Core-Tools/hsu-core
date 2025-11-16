// Process logging and log collection module
// TODO: Implement log collection from child processes

use crate::LogCollectionConfig;

pub struct LogCollectionManager {
    config: Option<LogCollectionConfig>,
}

impl LogCollectionManager {
    pub fn new(config: Option<LogCollectionConfig>) -> Self {
        Self {
            config,
        }
    }
    
    pub fn config(&self) -> Option<&LogCollectionConfig> {
        self.config.as_ref()
    }
}
