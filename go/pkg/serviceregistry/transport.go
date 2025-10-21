package serviceregistry

import (
	"fmt"
	"net"
	"os"
	"runtime"

	"github.com/core-tools/hsu-core/pkg/errors"
)

// TransportType defines the type of transport for the registry
type TransportType string

const (
	TransportAuto TransportType = "auto"
	TransportPipe TransportType = "pipe"
	TransportUDS  TransportType = "uds"
	TransportTCP  TransportType = "tcp"
)

// TransportConfig configures the registry transport
type TransportConfig struct {
	// Transport type (auto, pipe, uds, tcp)
	TransportType TransportType

	// Windows named pipe name (e.g., "hsu-registry" -> \\.\pipe\hsu-registry)
	PipeName string

	// Unix domain socket path
	SocketPath string

	// TCP address (host:port)
	TCPAddress string

	// Unix socket file permissions
	FileMode os.FileMode
}

// DefaultTransportConfig returns the default transport configuration for the platform
func DefaultTransportConfig() TransportConfig {
	if runtime.GOOS == "windows" {
		return TransportConfig{
			TransportType: TransportPipe,
			PipeName:      "hsu-registry", // Will become \\.\pipe\hsu-registry
		}
	} else {
		return TransportConfig{
			TransportType: TransportUDS,
			SocketPath:    "/var/run/hsu/registry.sock",
			FileMode:      0600, // Owner only
		}
	}
}

// DefaultTCPTransportConfig returns a TCP-based transport config (for development)
func DefaultTCPTransportConfig() TransportConfig {
	return TransportConfig{
		TransportType: TransportTCP,
		TCPAddress:    "127.0.0.1:17951", // HSU = 17 (H=8, S=19, U=21) -> 1+7+9+5+1=23 -> port 17951
	}
}

// CreateListener creates a network listener based on the transport configuration
func CreateListener(config TransportConfig) (net.Listener, error) {
	// Resolve "auto" to platform-specific default
	if config.TransportType == TransportAuto {
		config = DefaultTransportConfig()
	}

	switch config.TransportType {
	case TransportPipe:
		return createPipeListener(config)
	case TransportUDS:
		return createUDSListener(config)
	case TransportTCP:
		return createTCPListener(config)
	default:
		return nil, errors.NewValidationError("invalid transport type", nil).
			WithContext("transport_type", config.TransportType)
	}
}

func createPipeListener(config TransportConfig) (net.Listener, error) {
	if runtime.GOOS != "windows" {
		return nil, errors.NewValidationError("named pipes are only supported on Windows", nil)
	}

	pipeName := config.PipeName
	if pipeName == "" {
		pipeName = "hsu-registry"
	}

	// Windows named pipe path format
	pipePath := fmt.Sprintf(`\\.\pipe\%s`, pipeName)

	// For Windows, we need to use the winio package or fall back to TCP
	// Since winio might not be available, we'll document this as a requirement
	// For now, we'll return an error directing to use TCP on Windows for development
	return nil, fmt.Errorf("named pipe listener requires github.com/Microsoft/go-winio package. "+
		"For development, use TCP transport instead. "+
		"Pipe path would be: %s", pipePath)
}

func createUDSListener(config TransportConfig) (net.Listener, error) {
	if runtime.GOOS == "windows" {
		return nil, errors.NewValidationError("Unix domain sockets are not supported on Windows, use named pipes instead", nil)
	}

	socketPath := config.SocketPath
	if socketPath == "" {
		socketPath = "/tmp/hsu-registry.sock" // Fallback to /tmp for non-root users
	}

	// Remove existing socket file if it exists
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to remove existing socket file: %w", err)
	}

	// Ensure directory exists
	dir := socketPath[:len(socketPath)-len("/registry.sock")]
	if dir != "" && dir != "/tmp" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create socket directory: %w", err)
		}
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create Unix domain socket listener: %w", err)
	}

	// Set file permissions
	fileMode := config.FileMode
	if fileMode == 0 {
		fileMode = 0600 // Default: owner only
	}

	if err := os.Chmod(socketPath, fileMode); err != nil {
		listener.Close()
		return nil, fmt.Errorf("failed to set socket file permissions: %w", err)
	}

	return listener, nil
}

func createTCPListener(config TransportConfig) (net.Listener, error) {
	address := config.TCPAddress
	if address == "" {
		address = "127.0.0.1:17951"
	}

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("failed to create TCP listener: %w", err)
	}

	return listener, nil
}

// GetListenerAddress returns a string representation of the listener address
func GetListenerAddress(listener net.Listener) string {
	addr := listener.Addr()
	network := addr.Network()

	switch network {
	case "tcp":
		return fmt.Sprintf("tcp://%s", addr.String())
	case "unix":
		return fmt.Sprintf("unix://%s", addr.String())
	case "pipe":
		return fmt.Sprintf("pipe://%s", addr.String())
	default:
		return addr.String()
	}
}

// ParseRegistryURL parses a registry URL and returns transport configuration
func ParseRegistryURL(url string) (TransportConfig, error) {
	// Format: pipe://name, unix:///path, tcp://host:port
	if len(url) < 7 {
		return TransportConfig{}, fmt.Errorf("invalid registry URL: %s", url)
	}

	// Extract scheme
	var scheme, address string
	if url[:7] == "pipe://" {
		scheme = "pipe"
		address = url[7:]
	} else if url[:7] == "unix://" {
		scheme = "unix"
		address = url[7:]
	} else if url[:6] == "tcp://" {
		scheme = "tcp"
		address = url[6:]
	} else if url[:7] == "http://" {
		// Accept http:// for TCP
		scheme = "tcp"
		address = url[7:]
	} else {
		return TransportConfig{}, fmt.Errorf("unsupported URL scheme: %s", url)
	}

	switch scheme {
	case "pipe":
		return TransportConfig{
			TransportType: TransportPipe,
			PipeName:      address,
		}, nil
	case "unix":
		return TransportConfig{
			TransportType: TransportUDS,
			SocketPath:    address,
		}, nil
	case "tcp":
		return TransportConfig{
			TransportType: TransportTCP,
			TCPAddress:    address,
		}, nil
	default:
		return TransportConfig{}, fmt.Errorf("unsupported transport type: %s", scheme)
	}
}
