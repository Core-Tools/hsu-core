package process

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/core-tools/hsu-procman-go/pkg/errors"
	"github.com/core-tools/hsu-procman-go/pkg/logging"
	"github.com/core-tools/hsu-procman-go/pkg/processstate"
)

type DiscoveryMethod string

const (
	DiscoveryMethodProcessName DiscoveryMethod = "process-name"
	DiscoveryMethodPort        DiscoveryMethod = "port"
	DiscoveryMethodPIDFile     DiscoveryMethod = "pid-file"
	DiscoveryMethodServiceName DiscoveryMethod = "service-name"
)

type DiscoveryConfig struct {
	Method DiscoveryMethod `yaml:"method"`

	// Process name discovery
	ProcessName string   `yaml:"process_name,omitempty"`
	ProcessArgs []string `yaml:"process_args,omitempty"` // Optional: match by command line args

	// Port discovery
	Port     int    `yaml:"port,omitempty"`
	Protocol string `yaml:"protocol,omitempty"` // "tcp", "udp"

	// PID file discovery
	PIDFile string `yaml:"pid_file,omitempty"`

	// Service discovery (systemd, Windows services)
	ServiceName string `yaml:"service_name,omitempty"`

	// Discovery frequency
	CheckInterval time.Duration `yaml:"check_interval,omitempty"`
}

// AttachCmd represents a command that attaches to an existing process
type StdAttachCmd func(ctx context.Context) (*os.Process, io.ReadCloser, error)

// NewStdAttachCmd creates a standard attachment command function with logging
func NewStdAttachCmd(config DiscoveryConfig, id string, logger logging.Logger) StdAttachCmd {
	return func(ctx context.Context) (*os.Process, io.ReadCloser, error) {
		// Validate context
		if ctx == nil {
			logger.Errorf("Context cannot be nil, id: %s", id)
			return nil, nil, errors.NewValidationError("context cannot be nil", nil).WithContext("id", id)
		}

		// Validate discovery configuration
		if err := ValidateDiscoveryConfig(config); err != nil {
			logger.Errorf("Discovery configuration validation failed, id: %s, error: %v", id, err)
			return nil, nil, errors.NewValidationError("invalid discovery configuration", err).WithContext("id", id)
		}

		logger.Infof("Attaching to process, id: %s, discovery config: %+v", id, config)

		var process *os.Process
		var err error

		// Delegate to specific discovery method
		logger.Debugf("Starting process discovery, id: %s, method: %s", id, config.Method)
		switch config.Method {
		case DiscoveryMethodPIDFile:
			logger.Debugf("Discovering process by PID file, id: %s, file: %s", id, config.PIDFile)
			process, err = openProcessByPIDFile(config.PIDFile)
		case DiscoveryMethodProcessName:
			logger.Debugf("Discovering process by name, id: %s, name: %s, args: %v", id, config.ProcessName, config.ProcessArgs)
			process, err = openProcessByName(config.ProcessName, config.ProcessArgs)
		case DiscoveryMethodPort:
			logger.Debugf("Discovering process by port, id: %s, port: %d, protocol: %s", id, config.Port, config.Protocol)
			process, err = openProcessByPort(config.Port, config.Protocol)
		case DiscoveryMethodServiceName:
			logger.Debugf("Discovering process by service name, id: %s, service: %s", id, config.ServiceName)
			process, err = openProcessByServiceName(config.ServiceName)
		default:
			logger.Errorf("Unsupported discovery method, id: %s, method: %s", id, config.Method)
			return nil, nil, errors.NewValidationError("unsupported discovery method: "+string(config.Method), nil).WithContext("id", id)
		}

		if err != nil {
			logger.Errorf("Failed to discover process, id: %s, method: %s, error: %v", id, config.Method, err)
			return nil, nil, errors.NewDiscoveryError("failed to discover process", err).WithContext("discovery_method", string(config.Method)).WithContext("id", id)
		}

		logger.Infof("Successfully attached to process, id: %s, PID: %d, discovery: %s", id, process.Pid, config.Method)

		return process, nil, nil
	}
}

// openProcessByPIDFile discovers a process by reading its PID from a file
func openProcessByPIDFile(pidFile string) (*os.Process, error) {
	// Validate PID file path
	if err := ValidatePIDFile(pidFile); err != nil {
		return nil, err
	}

	// Read PID from file
	pidBytes, err := os.ReadFile(pidFile)
	if err != nil {
		return nil, errors.NewIOError("failed to read PID file", err).WithContext("pid_file", pidFile)
	}

	// Parse PID (trim whitespace/newlines)
	pidStr := strings.TrimSpace(string(pidBytes))
	if pidStr == "" {
		return nil, errors.NewValidationError("PID file is empty", nil).WithContext("pid_file", pidFile)
	}

	pid, err := ValidatePID(pidStr)
	if err != nil {
		return nil, errors.NewValidationError("invalid PID in file", err).WithContext("pid_file", pidFile).WithContext("pid_content", pidStr)
	}

	// Find the process
	process, err := os.FindProcess(pid)
	if err != nil {
		return nil, errors.NewProcessError("failed to find process", err).WithContext("pid", pid).WithContext("pid_file", pidFile)
	}

	// Verify the process is actually running
	running, err := processstate.IsProcessRunning(process.Pid)
	if !running {
		return nil, errors.NewProcessError("process is not running", err).WithContext("pid", pid).WithContext("pid_file", pidFile)
	}

	return process, nil
}

// openProcessByName discovers a process by its executable name and optional arguments
func openProcessByName(processName string, processArgs []string) (*os.Process, error) {
	// TODO: Implement process discovery by name
	// This would involve scanning running processes (e.g., via /proc on Linux, or WMI on Windows)
	// processArgs can be used for additional filtering when multiple processes have the same name
	return nil, fmt.Errorf("process discovery by name not implemented yet")
}

// openProcessByPort discovers a process by the port it's listening on
func openProcessByPort(port int, protocol string) (*os.Process, error) {
	// TODO: Implement process discovery by port
	// This would involve scanning network connections and finding which process owns the port
	// protocol specifies "tcp" or "udp"
	return nil, fmt.Errorf("process discovery by port not implemented yet")
}

// openProcessByServiceName discovers a process by its service name (systemd, Windows services, etc.)
func openProcessByServiceName(serviceName string) (*os.Process, error) {
	// TODO: Implement process discovery by service name
	// This would involve querying the service manager (systemd, Windows service manager, etc.)
	return nil, fmt.Errorf("process discovery by service name not implemented yet")
}
