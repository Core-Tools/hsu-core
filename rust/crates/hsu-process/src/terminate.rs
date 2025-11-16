//! Process termination primitives.
//!
//! This module provides cross-platform process termination.

use hsu_common::ProcessResult;

/// Terminate a process gracefully (SIGTERM on Unix, WM_CLOSE on Windows).
pub fn terminate_gracefully(pid: u32) -> ProcessResult<()> {
    #[cfg(unix)]
    {
        use nix::sys::signal::{kill, Signal};
        use nix::unistd::Pid;

        let nix_pid = Pid::from_raw(pid as i32);
        kill(nix_pid, Signal::SIGTERM)
            .map_err(|e| hsu_common::ProcessError::stop_failed(pid.to_string(), e.to_string()))
    }

    #[cfg(windows)]
    {
        // Windows implementation placeholder
        // TODO: Implement Windows-specific graceful termination
        Err(hsu_common::ProcessError::stop_failed(
            pid.to_string(),
            "Windows graceful termination not yet implemented".to_string(),
        ))
    }
}

/// Force kill a process (SIGKILL on Unix, TerminateProcess on Windows).
pub fn force_kill(pid: u32) -> ProcessResult<()> {
    #[cfg(unix)]
    {
        use nix::sys::signal::{kill, Signal};
        use nix::unistd::Pid;

        let nix_pid = Pid::from_raw(pid as i32);
        kill(nix_pid, Signal::SIGKILL)
            .map_err(|e| hsu_common::ProcessError::stop_failed(pid.to_string(), e.to_string()))
    }

    #[cfg(windows)]
    {
        // Windows implementation placeholder
        // TODO: Implement Windows-specific force kill
        Err(hsu_common::ProcessError::stop_failed(
            pid.to_string(),
            "Windows force kill not yet implemented".to_string(),
        ))
    }
}

