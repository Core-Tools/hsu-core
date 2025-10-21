package moduleapi

import (
	"fmt"
	"net/url"

	"github.com/core-tools/hsu-core/pkg/errors"
	"github.com/core-tools/hsu-core/pkg/logging"
	"github.com/core-tools/hsu-core/pkg/modulemanagement/moduletypes"
)

type ServiceRegistryClient interface {
	PublishAPIs(apis []RemoteAPI) error
	FindModuleAPIs(moduleID moduletypes.ModuleID) ([]RemoteModuleAPI, error)
}

func NewServiceRegistryClient(serverRegistryURL string, logger logging.Logger) (ServiceRegistryClient, error) {
	url, err := url.Parse(serverRegistryURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse server registry URL: %v", err)
	}
	return &serviceRegistryClient{
		url:    url,
		logger: logger,
	}, nil
}

type serviceRegistryClient struct {
	url    *url.URL
	logger logging.Logger
}

func (c *serviceRegistryClient) PublishAPIs(apis []RemoteAPI) error {
	if len(apis) == 0 {
		return nil
	}
	return nil
}

func (c *serviceRegistryClient) FindModuleAPIs(moduleID moduletypes.ModuleID) ([]RemoteModuleAPI, error) {
	if moduleID == "echo" {
		return []RemoteModuleAPI{
			{
				ServiceIDs: []moduletypes.ServiceID{"service1"},
				ServerPort: 50051,
				Protocol:   moduletypes.ProtocolGRPC,
			},
		}, nil
	}
	return nil, errors.NewNotFoundError("module not found", nil).
		WithContext("module_id", moduleID)
}
