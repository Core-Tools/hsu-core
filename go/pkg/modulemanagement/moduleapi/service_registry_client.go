package moduleapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"time"

	"github.com/core-tools/hsu-core/pkg/errors"
	"github.com/core-tools/hsu-core/pkg/logging"
	"github.com/core-tools/hsu-core/pkg/modulemanagement/moduletypes"
)

type ServiceRegistryClient interface {
	PublishAPIs(apis []RemoteAPI) error
	FindModuleAPIs(moduleID moduletypes.ModuleID) ([]RemoteModuleAPI, error)
}

type ServiceRegistryClientOptions struct {
	URL     string
	Timeout time.Duration
	Logger  logging.Logger
}

func (o *ServiceRegistryClientOptions) OptLogger() logging.Logger {
	if o.Logger == nil {
		return logging.NewNullLogger()
	}
	return o.Logger
}

func (o *ServiceRegistryClientOptions) OptURL() string {
	if o.URL == "" {
		return getDefaultRegistryURL()
	}
	return o.URL
}

func (o *ServiceRegistryClientOptions) OptTimeout() time.Duration {
	if o.Timeout == 0 {
		return 5 * time.Second
	}
	return o.Timeout
}

func NewServiceRegistryClient(options ServiceRegistryClientOptions) (ServiceRegistryClient, error) {
	logger := options.OptLogger()

	// If no URL provided, use default for platform
	serverRegistryURL := options.OptURL()

	parsedURL, err := url.Parse(serverRegistryURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse server registry URL: %w", err)
	}

	// Create HTTP client with custom transport for pipe/UDS
	transport, err := createTransport(parsedURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport: %w", err)
	}

	return &serviceRegistryClient{
		baseURL: "http://localhost", // Actual address handled by transport
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   options.OptTimeout(),
		},
		logger: logger,
	}, nil
}

type serviceRegistryClient struct {
	baseURL    string
	httpClient *http.Client
	logger     logging.Logger
}

func (c *serviceRegistryClient) PublishAPIs(apis []RemoteAPI) error {
	if len(apis) == 0 {
		return nil
	}

	for _, api := range apis {
		req := struct {
			ModuleID  moduletypes.ModuleID `json:"moduleID"`
			ProcessID int                  `json:"processID"`
			APIs      []RemoteModuleAPI    `json:"apis"`
		}{
			ModuleID:  api.ModuleID,
			ProcessID: os.Getpid(),
			APIs:      api.ModuleAPIs,
		}

		body, err := json.Marshal(req)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}

		resp, err := c.httpClient.Post(
			c.baseURL+"/api/v1/publish",
			"application/json",
			bytes.NewReader(body),
		)
		if err != nil {
			return fmt.Errorf("failed to publish APIs for module %s: %w", api.ModuleID, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			var errResp struct {
				Error string `json:"error"`
			}
			json.NewDecoder(resp.Body).Decode(&errResp)
			return fmt.Errorf("publish failed with status %d: %s", resp.StatusCode, errResp.Error)
		}

		c.logger.Infof("Published APIs for module %s: %d endpoints", api.ModuleID, len(api.ModuleAPIs))
	}

	return nil
}

func (c *serviceRegistryClient) FindModuleAPIs(moduleID moduletypes.ModuleID) ([]RemoteModuleAPI, error) {
	url := fmt.Sprintf("%s/api/v1/discover/%s", c.baseURL, moduleID)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to discover module %s: %w", moduleID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.NewNotFoundError("module not found", nil).
			WithContext("module_id", moduleID)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&errResp)
		return nil, fmt.Errorf("discover failed with status %d: %s", resp.StatusCode, errResp.Error)
	}

	var result struct {
		ModuleID moduletypes.ModuleID `json:"moduleID"`
		APIs     []RemoteModuleAPI    `json:"apis"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	c.logger.Debugf("Discovered %d APIs for module %s", len(result.APIs), moduleID)

	return result.APIs, nil
}

// Helper functions

func getDefaultRegistryURL() string {
	if runtime.GOOS == "windows" {
		// For now, use TCP on Windows since named pipes require additional package
		return "tcp://127.0.0.1:17951"
	} else {
		// Unix domain socket for Unix-like systems
		return "unix:///tmp/hsu-registry.sock"
	}
}

func createTransport(registryURL *url.URL) (*http.Transport, error) {
	switch registryURL.Scheme {
	case "pipe":
		// Windows named pipe
		// Note: This requires github.com/Microsoft/go-winio
		// For now, return error directing to use TCP
		return nil, fmt.Errorf("named pipe transport requires additional dependencies. Use tcp:// for development")

	case "unix", "uds":
		// Unix domain socket
		socketPath := registryURL.Path
		if socketPath == "" {
			socketPath = "/tmp/hsu-registry.sock"
		}
		return &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", socketPath)
			},
		}, nil

	case "tcp", "http":
		// Standard TCP
		address := registryURL.Host
		if address == "" {
			address = "127.0.0.1:17951"
		}
		return &http.Transport{
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "tcp", address)
			},
		}, nil

	default:
		return nil, fmt.Errorf("unsupported URL scheme: %s (supported: unix, tcp, http)", registryURL.Scheme)
	}
}
