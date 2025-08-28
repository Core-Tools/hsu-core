// Utility functions and helpers

use std::time::Duration;

/// Convert seconds to Duration safely
pub fn seconds_to_duration(seconds: u64) -> Duration {
    Duration::from_secs(seconds.min(u64::MAX / 1000))
}

/// Generate a unique ID for processes
pub fn generate_unique_id() -> String {
    uuid::Uuid::new_v4().to_string()
}

/// Platform-specific path separator
pub fn path_separator() -> &'static str {
    if cfg!(windows) {
        "\\"
    } else {
        "/"
    }
}

/// Validate process ID format
pub fn validate_process_id(id: &str) -> bool {
    !id.is_empty() 
        && id.len() <= 64 
        && id.chars().all(|c| c.is_alphanumeric() || c == '-' || c == '_')
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_validate_process_id() {
        assert!(validate_process_id("valid-process-id"));
        assert!(validate_process_id("test_process_123"));
        assert!(!validate_process_id(""));
        assert!(!validate_process_id("invalid process id")); // spaces not allowed
        assert!(!validate_process_id(&"x".repeat(65))); // too long
    }

    #[test]
    fn test_generate_unique_id() {
        let id1 = generate_unique_id();
        let id2 = generate_unique_id();
        assert_ne!(id1, id2);
        assert!(!id1.is_empty());
        assert!(!id2.is_empty());
    }
}
