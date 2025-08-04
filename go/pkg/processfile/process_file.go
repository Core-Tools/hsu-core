package processfile

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/core-tools/hsu-core/pkg/errors"
	"github.com/core-tools/hsu-core/pkg/logging"
)

// Default application name for HSU Master
const DefaultAppName = "hsu-master"

// ProcessFileConfig holds configuration for process file generation (PID files, port files, etc.)
type ProcessFileConfig struct {
	// Base directory for PID files. If empty, uses OS-appropriate default
	BaseDirectory string

	// Service context - affects directory selection
	ServiceContext ServiceContext

	// Application name for subdirectory creation
	AppName string

	// Create subdirectory for the app (recommended for system services)
	UseSubdirectory bool

	// Service scenario - fills defaults to all fields
	// Used when service context is not set up
	Scenario string
}

// ServiceContext defines the context in which the service runs
type ServiceContext string

const (
	// SystemService runs as a system service (daemon)
	SystemService ServiceContext = "system"

	// UserService runs as a user service
	UserService ServiceContext = "user"

	// SessionService runs as a session service (cleaned up on logout)
	SessionService ServiceContext = "session"
)

// ProcessFileManager provides process file path generation and management (PID files, port files, etc.)
type ProcessFileManager struct {
	config ProcessFileConfig
	logger logging.Logger
}

// NewProcessFileManager creates a new process file manager with the given configuration
func NewProcessFileManager(config ProcessFileConfig, logger logging.Logger) *ProcessFileManager {
	if config.AppName == "" {
		config.AppName = DefaultAppName
	}
	if config.ServiceContext == "" {
		config = GetRecommendedProcessFileConfig(config.Scenario, config.AppName)
	}

	return &ProcessFileManager{
		config: config,
		logger: logger,
	}
}

// generateFilePath generates an appropriate PID file path for the given worker ID
func (m *ProcessFileManager) generateFilePath(workerID, ext string) string {
	baseDir := m.getBaseDirectory()

	// Create app subdirectory if requested
	if m.config.UseSubdirectory {
		baseDir = filepath.Join(baseDir, m.config.AppName)
	}

	return filepath.Join(baseDir, workerID+ext)
}

// GeneratePIDFilePath generates an appropriate PID file path for the given worker ID
func (m *ProcessFileManager) GeneratePIDFilePath(workerID string) string {
	return m.generateFilePath(workerID, ".pid")
}

// GeneratePortFilePath generates an appropriate port file path for the given worker ID
func (m *ProcessFileManager) GeneratePortFilePath(workerID string) string {
	// Use same base directory as PID files but with .port extension
	return m.generateFilePath(workerID, ".port")
}

// WritePIDFile writes the process PID to the appropriate file for the given worker ID
func (m *ProcessFileManager) WritePIDFile(workerID string, pid int) error {
	pidFilePath := m.GeneratePIDFilePath(workerID)
	m.logger.Debugf("Writing PID file, worker: %s, pid: %d, path: %s", workerID, pid, pidFilePath)

	// Validate directory exists and is writable
	if err := ValidateDirectory(pidFilePath); err != nil {
		m.logger.Errorf("PID file directory validation failed, worker: %s, path: %s, error: %v", workerID, pidFilePath, err)
		return errors.NewIOError("PID file directory validation failed", err).WithContext("pid_file", pidFilePath)
	}

	// Write PID to file
	pidContent := fmt.Sprintf("%d\n", pid)
	if err := os.WriteFile(pidFilePath, []byte(pidContent), 0644); err != nil {
		m.logger.Errorf("Failed to write PID file, worker: %s, pid: %d, path: %s, error: %v", workerID, pid, pidFilePath, err)
		return errors.NewIOError("failed to write PID file", err).WithContext("pid_file", pidFilePath).WithContext("pid", pid)
	}

	m.logger.Infof("PID file written successfully, worker: %s, pid: %d, path: %s", workerID, pid, pidFilePath)
	return nil
}

// WritePortFile writes a port number to a port file
func (m *ProcessFileManager) WritePortFile(workerID string, port int) error {
	portPath := m.GeneratePortFilePath(workerID)
	m.logger.Debugf("Writing port file, worker: %s, port: %d, path: %s", workerID, port, portPath)

	// Validate directory exists and is writable
	if err := ValidateDirectory(portPath); err != nil {
		m.logger.Errorf("Port file directory validation failed, worker: %s, path: %s, error: %v", workerID, portPath, err)
		return errors.NewIOError("port file directory validation failed", err).WithContext("port_file", portPath)
	}

	// Write port to file
	portContent := fmt.Sprintf("%d\n", port)
	if err := os.WriteFile(portPath, []byte(portContent), 0644); err != nil {
		m.logger.Errorf("Failed to write port file, worker: %s, port: %d, path: %s, error: %v", workerID, port, portPath, err)
		return errors.NewIOError("failed to write port file", err).WithContext("port_file", portPath).WithContext("port", port)
	}

	m.logger.Infof("Port file written successfully, worker: %s, port: %d, path: %s", workerID, port, portPath)
	return nil
}

// ReadPortFile reads a port number from a port file
func (m *ProcessFileManager) ReadPortFile(workerID string) (int, error) {
	portPath := m.GeneratePortFilePath(workerID)
	m.logger.Debugf("Reading port file, worker: %s, path: %s", workerID, portPath)

	// Read port file
	content, err := os.ReadFile(portPath)
	if err != nil {
		m.logger.Warnf("Failed to read port file, worker: %s, path: %s, error: %v", workerID, portPath, err)
		return 0, errors.NewIOError("failed to read port file", err).WithContext("port_file", portPath)
	}

	// Parse port number
	portStr := strings.TrimSpace(string(content))
	port, err := strconv.Atoi(portStr)
	if err != nil {
		m.logger.Errorf("Invalid port content in port file, worker: %s, path: %s, content: %s, error: %v", workerID, portPath, portStr, err)
		return 0, errors.NewValidationError("invalid port in port file", err).WithContext("port_file", portPath).WithContext("content", portStr)
	}

	m.logger.Debugf("Port file read successfully, worker: %s, port: %d, path: %s", workerID, port, portPath)
	return port, nil
}

// ===== LOG DIRECTORY METHODS =====

// GenerateLogDirectoryPath generates the appropriate log directory path for the application
func (m *ProcessFileManager) GenerateLogDirectoryPath() string {
	baseDir := m.getLogBaseDirectory()

	// Create app subdirectory if requested
	if m.config.UseSubdirectory {
		// For consistency with PID files: C:\ProgramData\hsu-master\logs
		baseDir = filepath.Join(baseDir, m.config.AppName)
	}

	// For direct log directory without app subdirectory
	return filepath.Join(baseDir, "logs")
}

// GenerateWorkerLogDirectoryPath generates the log directory path for worker-specific logs
func (m *ProcessFileManager) GenerateWorkerLogDirectoryPath() string {
	return filepath.Join(m.GenerateLogDirectoryPath(), "workers")
}

// GenerateLogFilePath generates a complete log file path from a relative template
func (m *ProcessFileManager) GenerateLogFilePath(relativeTemplate string) string {
	logDir := m.GenerateLogDirectoryPath()
	return filepath.Join(logDir, relativeTemplate)
}

// GenerateWorkerLogFilePath generates a worker-specific log file path from a relative template
func (m *ProcessFileManager) GenerateWorkerLogFilePath(relativeTemplate string, workerID string) string {
	workerLogDir := m.GenerateWorkerLogDirectoryPath()

	// Replace {worker_id} placeholder in template
	resolvedTemplate := strings.ReplaceAll(relativeTemplate, "{worker_id}", workerID)

	return filepath.Join(workerLogDir, resolvedTemplate)
}

// getBaseDirectory returns the appropriate base directory for PID files
func (m *ProcessFileManager) getBaseDirectory() string {
	// Use explicit configuration if provided
	if m.config.BaseDirectory != "" {
		return m.config.BaseDirectory
	}

	// Use OS-appropriate defaults based on service context
	switch m.config.ServiceContext {
	case SystemService:
		return m.getSystemServiceDirectory()
	case UserService:
		return m.getUserServiceDirectory()
	case SessionService:
		return m.getSessionServiceDirectory()
	default:
		return m.getSystemServiceDirectory()
	}
}

// getSystemServiceDirectory returns the directory for system services
func (m *ProcessFileManager) getSystemServiceDirectory() string {
	switch runtime.GOOS {
	case "windows":
		// Use ProgramData for system services on Windows
		programData := os.Getenv("PROGRAMDATA")
		if programData == "" {
			programData = "C:\\ProgramData"
		}
		return programData

	case "darwin":
		// macOS system services
		return "/var/run"

	default:
		// Linux and other Unix systems
		// Modern standard is /run, with fallback to /var/run
		if _, err := os.Stat("/run"); err == nil {
			return "/run"
		}
		return "/var/run"
	}
}

// getUserServiceDirectory returns the directory for user services
func (m *ProcessFileManager) getUserServiceDirectory() string {
	switch runtime.GOOS {
	case "windows":
		// Use LocalAppData for user services on Windows
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData == "" {
			userProfile := os.Getenv("USERPROFILE")
			if userProfile != "" {
				localAppData = filepath.Join(userProfile, "AppData", "Local")
			} else {
				localAppData = "C:\\Users\\Default\\AppData\\Local"
			}
		}
		return localAppData

	case "darwin":
		// macOS user services
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "/tmp"
		}
		return filepath.Join(homeDir, "Library", "Application Support")

	default:
		// Linux and other Unix systems
		// Use XDG_RUNTIME_DIR if available, otherwise /tmp
		if runtimeDir := os.Getenv("XDG_RUNTIME_DIR"); runtimeDir != "" {
			return runtimeDir
		}
		return "/tmp"
	}
}

// getSessionServiceDirectory returns the directory for session services
func (m *ProcessFileManager) getSessionServiceDirectory() string {
	switch runtime.GOOS {
	case "windows":
		// Windows doesn't have a direct equivalent, use temp directory
		return os.TempDir()

	case "darwin":
		// macOS session services
		return os.TempDir()

	default:
		// Linux systemd session services
		uid := os.Getuid()
		sessionDir := fmt.Sprintf("/run/user/%d", uid)

		// Check if session directory exists
		if _, err := os.Stat(sessionDir); err == nil {
			return sessionDir
		}

		// Fallback to temp directory
		return "/tmp"
	}
}

// ValidateDirectory validates that directory exists and is writable
func ValidateDirectory(path string) error {
	dir := filepath.Dir(path)

	// Check if directory exists
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			// Try to create the directory
			if err := os.MkdirAll(dir, 0755); err != nil {
				return errors.NewIOError("failed to create directory", err).WithContext("directory", dir)
			}
		} else {
			return errors.NewIOError("failed to access directory", err).WithContext("directory", dir)
		}
	} else if !info.IsDir() {
		return errors.NewValidationError("Path is not a directory", nil).WithContext("path", dir)
	}

	// Check if directory is writable
	testFile := filepath.Join(dir, ".write_test")
	if file, err := os.Create(testFile); err != nil {
		return errors.NewPermissionError("Directory is not writable", err).WithContext("directory", dir)
	} else {
		file.Close()
		os.Remove(testFile)
	}

	return nil
}

// GetRecommendedProcessFileConfig returns recommended process file configuration for different deployment scenarios
func GetRecommendedProcessFileConfig(scenario string, appName string) ProcessFileConfig {
	if appName == "" {
		appName = DefaultAppName
	}

	switch strings.ToLower(scenario) {
	case "system", "daemon", "service":
		return ProcessFileConfig{
			ServiceContext:  SystemService,
			AppName:         appName,
			UseSubdirectory: true,
		}

	case "user", "personal":
		return ProcessFileConfig{
			ServiceContext:  UserService,
			AppName:         appName,
			UseSubdirectory: true,
		}

	case "session", "desktop":
		return ProcessFileConfig{
			ServiceContext:  SessionService,
			AppName:         appName,
			UseSubdirectory: false,
		}

	case "development", "dev", "test":
		return ProcessFileConfig{
			BaseDirectory:   filepath.Join(os.TempDir(), appName+"-dev"),
			ServiceContext:  UserService,
			AppName:         appName,
			UseSubdirectory: false,
		}

	default:
		// Default to system service
		return ProcessFileConfig{
			ServiceContext:  UserService,
			AppName:         appName,
			UseSubdirectory: true,
		}
	}
}

// getLogBaseDirectory returns the appropriate base directory for log files
func (m *ProcessFileManager) getLogBaseDirectory() string {
	// Use explicit configuration if provided
	if m.config.BaseDirectory != "" {
		return filepath.Join(m.config.BaseDirectory, "logs")
	}

	// Use OS-appropriate defaults based on service context
	switch m.config.ServiceContext {
	case SystemService:
		return m.getSystemLogDirectory()
	case UserService:
		return m.getUserLogDirectory()
	case SessionService:
		return m.getSessionLogDirectory()
	default:
		return m.getSystemLogDirectory()
	}
}

// getSystemLogDirectory returns the directory for system service logs
func (m *ProcessFileManager) getSystemLogDirectory() string {
	switch runtime.GOOS {
	case "windows":
		// Use ProgramData for system service logs on Windows (consistent with PID files)
		programData := os.Getenv("PROGRAMDATA")
		if programData == "" {
			programData = "C:\\ProgramData"
		}
		return programData

	case "darwin":
		// macOS system service logs
		return "/var/log"

	default:
		// Linux and other Unix systems
		return "/var/log"
	}
}

// getUserLogDirectory returns the directory for user service logs
func (m *ProcessFileManager) getUserLogDirectory() string {
	switch runtime.GOOS {
	case "windows":
		// Use LocalAppData for user service logs on Windows
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData == "" {
			userProfile := os.Getenv("USERPROFILE")
			if userProfile != "" {
				localAppData = filepath.Join(userProfile, "AppData", "Local")
			} else {
				localAppData = "C:\\Users\\Default\\AppData\\Local"
			}
		}
		return filepath.Join(localAppData, "logs")

	case "darwin":
		// macOS user service logs
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "/tmp/logs"
		}
		return filepath.Join(homeDir, "Library", "Logs")

	default:
		// Linux and other Unix systems
		// Use XDG_DATA_HOME or ~/.local/share for user logs
		if dataHome := os.Getenv("XDG_DATA_HOME"); dataHome != "" {
			return filepath.Join(dataHome, "logs")
		}
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "/tmp/logs"
		}
		return filepath.Join(homeDir, ".local", "share", "logs")
	}
}

// getSessionLogDirectory returns the directory for session service logs
func (m *ProcessFileManager) getSessionLogDirectory() string {
	switch runtime.GOOS {
	case "windows":
		// Windows doesn't have a direct equivalent, use temp directory
		return filepath.Join(os.TempDir(), "logs")

	case "darwin":
		// macOS session service logs
		return filepath.Join(os.TempDir(), "logs")

	default:
		// Linux systemd session services
		uid := os.Getuid()
		sessionDir := fmt.Sprintf("/run/user/%d/logs", uid)

		// Check if session directory exists
		if _, err := os.Stat(filepath.Dir(sessionDir)); err == nil {
			return sessionDir
		}

		// Fallback to temp directory
		return filepath.Join("/tmp", "logs")
	}
}
