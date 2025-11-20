package process

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/core-tools/hsu-procman-go/pkg/errors"
	"github.com/core-tools/hsu-procman-go/pkg/logging"
)

type ExecutionConfig struct {
	ExecutablePath   string        `yaml:"executable_path"`
	Args             []string      `yaml:"args,omitempty"`
	Environment      []string      `yaml:"environment,omitempty"`
	WorkingDirectory string        `yaml:"working_directory,omitempty"`
	WaitDelay        time.Duration `yaml:"wait_delay,omitempty"`
}

type StdExecuteCmd func(ctx context.Context) (*os.Process, io.ReadCloser, error)

func NewStdExecuteCmd(execution ExecutionConfig, id string, logger logging.Logger) StdExecuteCmd {
	return func(ctx context.Context) (*os.Process, io.ReadCloser, error) {
		// Validate context
		if ctx == nil {
			logger.Errorf("Context cannot be nil, id: %s", id)
			return nil, nil, errors.NewValidationError("context cannot be nil", nil).WithContext("id", id)
		}

		// Validate execution configuration
		if err := ValidateExecutionConfig(execution); err != nil {
			logger.Errorf("Execution configuration validation failed, id: %s, error: %v", id, err)
			return nil, nil, errors.NewValidationError("invalid execution configuration", err).WithContext("id", id)
		}

		logger.Infof("Executing process, id: %s, execution config: %+v", id, execution)

		// Check if the process is executable, and make it executable if it's not
		if err := ensureExecutable(execution.ExecutablePath); err != nil {
			return nil, nil, errors.NewPermissionError("failed to ensure process is executable", err).WithContext("id", id).WithContext("executable_path", execution.ExecutablePath)
		}

		workDir := execution.WorkingDirectory
		if workDir == "" {
			absPath, err := filepath.Abs(execution.ExecutablePath)
			if err != nil {
				return nil, nil, errors.NewIOError("failed to get absolute path", err).WithContext("id", id).WithContext("executable_path", execution.ExecutablePath)
			}
			workDir = filepath.Dir(absPath)
		}

		logger.Debugf("Executing process: id: %s, executable path: '%s', args: %v, working directory: '%s'",
			id, execution.ExecutablePath, execution.Args, workDir)

		env := os.Environ()
		for _, e := range execution.Environment {
			env = append(env, e)
		}

		cmd := exec.CommandContext(ctx, execution.ExecutablePath, execution.Args...)
		cmd.Dir = workDir
		cmd.Env = env

		// Platform-specific setup is handled in process_windows.go or process_unix.go
		setupProcessAttributes(cmd)

		// wait after sending the interrupt signal, before sending the kill signal
		cmd.WaitDelay = execution.WaitDelay

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return nil, nil, errors.NewProcessError("failed to create stdout pipe", err).WithContext("id", id).WithContext("executable_path", execution.ExecutablePath)
		}
		cmd.Stderr = cmd.Stdout

		logger.Infof("Executing process, id: %s, cmd: %+v", id, cmd)

		err = cmd.Start()
		if err != nil {
			return nil, nil, errors.NewProcessError("failed to start the process", err).WithContext("id", id).WithContext("executable_path", execution.ExecutablePath)
		}

		logger.Infof("Successfully executed process, id: %s, PID: %d", id, cmd.Process.Pid)

		return cmd.Process, stdout, nil
	}
}

// ensureExecutable checks if a file is executable and makes it executable if it's not
func ensureExecutable(path string) error {
	// Check if file exists
	info, err := os.Stat(path)
	if err != nil {
		return errors.NewIOError("file does not exist", err).WithContext("path", path)
	}

	// On Windows, files with .exe, .bat, .cmd extensions are inherently executable
	// Also, system files in Windows system directories are already executable
	if runtime.GOOS == "windows" {
		ext := filepath.Ext(path)
		if ext == ".exe" || ext == ".bat" || ext == ".cmd" {
			return nil // Already executable
		}
		// For Windows system directories, assume files are executable
		if filepath.Dir(path) == "C:\\Windows\\System32" || filepath.Dir(path) == "C:\\Windows\\System32\\" {
			return nil // System files are already executable
		}
	}

	// Check if file is already executable
	mode := info.Mode()
	if mode&0111 != 0 { // Check if any execute bit is set
		return nil // Already executable
	}

	// Try to make it executable (only on Unix systems or non-system files)
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, mode|0111); err != nil {
			return errors.NewPermissionError("failed to make file executable", err).WithContext("path", path)
		}
	}

	return nil
}
