//! # HSU Log Collection
//!
//! Log capture and aggregation for the HSU framework.
//!
//! This crate provides:
//! - Process output capture (stdout/stderr)
//! - Log aggregation and forwarding
//! - Structured logging enhancement
//! - Multiple output targets
//!
//! This corresponds to the Go package `pkg/logcollection`.

pub mod capture;

use serde::{Deserialize, Serialize};
use std::path::PathBuf;
use tokio::sync::mpsc;

/// Log entry from a process.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LogEntry {
    pub process_id: String,
    pub timestamp: chrono::DateTime<chrono::Utc>,
    pub level: LogLevel,
    pub source: LogSource,
    pub message: String,
    pub metadata: Option<LogMetadata>,
}

/// Log level.
#[derive(Debug, Clone, Copy, Serialize, Deserialize, PartialEq, Eq)]
pub enum LogLevel {
    Trace,
    Debug,
    Info,
    Warn,
    Error,
}

/// Log source (stdout or stderr).
#[derive(Debug, Clone, Copy, Serialize, Deserialize, PartialEq, Eq)]
pub enum LogSource {
    Stdout,
    Stderr,
}

/// Optional log metadata.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LogMetadata {
    pub hostname: Option<String>,
    pub process_manager_id: Option<String>,
    pub sequence_number: Option<u64>,
    pub line_number: Option<usize>,
}

/// Log collection configuration.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LogCollectionConfig {
    pub capture_stdout: bool,
    pub capture_stderr: bool,
    pub buffer_size: usize,
    pub log_file: Option<PathBuf>,
}

impl Default for LogCollectionConfig {
    fn default() -> Self {
        Self {
            capture_stdout: true,
            capture_stderr: true,
            buffer_size: 8192,
            log_file: None,
        }
    }
}

/// Log collector that aggregates logs from multiple processes.
pub struct LogCollector {
    sender: mpsc::UnboundedSender<LogEntry>,
    receiver: mpsc::UnboundedReceiver<LogEntry>,
}

impl LogCollector {
    pub fn new() -> Self {
        let (sender, receiver) = mpsc::unbounded_channel();
        Self { sender, receiver }
    }

    pub fn get_sender(&self) -> mpsc::UnboundedSender<LogEntry> {
        self.sender.clone()
    }

    pub async fn collect_logs(&mut self) -> Option<LogEntry> {
        self.receiver.recv().await
    }
}

impl Default for LogCollector {
    fn default() -> Self {
        Self::new()
    }
}

