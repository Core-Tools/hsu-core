//! # HSU Process File
//!
//! Process state persistence for the HSU framework.
//!
//! This crate provides functionality for:
//! - Saving process state to disk
//! - Loading process state from disk
//! - Process attachment after manager restart
//!
//! This corresponds to the Go package `pkg/processfile`.

use hsu_common::ProcessResult;
use hsu_process_state::ProcessState;
use serde::{Deserialize, Serialize};
use std::path::Path;

/// Process file data structure.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ProcessFile {
    pub process_id: String,
    pub pid: u32,
    pub state: ProcessState,
    pub start_time: chrono::DateTime<chrono::Utc>,
    pub config_hash: Option<String>,
}

impl ProcessFile {
    /// Create a new process file.
    pub fn new(
        process_id: String,
        pid: u32,
        state: ProcessState,
        start_time: chrono::DateTime<chrono::Utc>,
    ) -> Self {
        Self {
            process_id,
            pid,
            state,
            start_time,
            config_hash: None,
        }
    }

    /// Save process file to disk.
    pub async fn save<P: AsRef<Path>>(&self, path: P) -> ProcessResult<()> {
        let json = serde_json::to_string_pretty(self)
            .map_err(|e| hsu_common::ProcessError::Configuration {
                id: self.process_id.clone(),
                reason: format!("Failed to serialize process file: {}", e),
            })?;

        tokio::fs::write(&path, json)
            .await
            .map_err(|e| hsu_common::ProcessError::Configuration {
                id: self.process_id.clone(),
                reason: format!("Failed to write process file: {}", e),
            })?;

        Ok(())
    }

    /// Load process file from disk.
    pub async fn load<P: AsRef<Path>>(path: P) -> ProcessResult<Self> {
        let content = tokio::fs::read_to_string(&path)
            .await
            .map_err(|e| hsu_common::ProcessError::Configuration {
                id: "unknown".to_string(),
                reason: format!("Failed to read process file: {}", e),
            })?;

        let process_file: ProcessFile = serde_json::from_str(&content)
            .map_err(|e| hsu_common::ProcessError::Configuration {
                id: "unknown".to_string(),
                reason: format!("Failed to parse process file: {}", e),
            })?;

        Ok(process_file)
    }

    /// Delete process file from disk.
    pub async fn delete<P: AsRef<Path>>(path: P) -> ProcessResult<()> {
        tokio::fs::remove_file(&path)
            .await
            .map_err(|e| hsu_common::ProcessError::Configuration {
                id: "unknown".to_string(),
                reason: format!("Failed to delete process file: {}", e),
            })?;

        Ok(())
    }
}

