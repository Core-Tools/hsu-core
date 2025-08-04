package modules

import (
	"context"
	"fmt"
	"sync"

	"github.com/core-tools/hsu-core/pkg/logging"
)

type ModuleRegistrar interface {
	RegisterModule(moduleID string, module Module) error
}

type Manager interface {
	ModuleRegistrar
	GatewayFactoryProvider
	GatewayFactoryInfoReader
	HandlerRegistrarProvider
	HandlerRegistrarInfoReader
	DirectClosureProvider
	ModulesLifecycle
}

type manager struct {
	modules                 map[string]Module
	directClosures          map[string]interface{}
	remoteGatewayFactories  map[string]GatewayFactoryUnion
	remoteHandlerRegistrars map[string]HandlerRegistrarUnion
	logger                  logging.Logger
	mx                      sync.Mutex
}

func NewManager(logger logging.Logger) Manager {
	return &manager{
		modules:                 make(map[string]Module),
		directClosures:          make(map[string]interface{}),
		remoteGatewayFactories:  make(map[string]GatewayFactoryUnion),
		remoteHandlerRegistrars: make(map[string]HandlerRegistrarUnion),
		logger:                  logger,
	}
}

func (m *manager) RegisterModule(moduleID string, module Module) error {
	m.mx.Lock()
	defer m.mx.Unlock()

	m.modules[moduleID] = module
	return nil
}

func (m *manager) getModules() []Module {
	m.mx.Lock()
	defer m.mx.Unlock()

	modules := make([]Module, 0)
	for _, module := range m.modules {
		modules = append(modules, module)
	}

	return modules
}

func (m *manager) Initialize() error {
	modules := m.getModules()

	for _, module := range modules {
		err := module.Initialize(m)
		if err != nil {
			return err
		}
	}

	return nil
}

func (m *manager) Start(ctx context.Context, gatewayFactory GatewayFactory) error {
	modules := m.getModules()

	for _, module := range modules {
		err := module.Start(ctx, gatewayFactory)
		if err != nil {
			return err
		}
	}

	return nil
}

func (m *manager) Stop(ctx context.Context) error {
	modules := m.getModules()

	for _, module := range modules {
		err := module.Stop(ctx)
		if err != nil {
			return err
		}
	}

	return nil
}

func (m *manager) getModuleEndpointKey(moduleID, endpointID string) (string, error) {
	if moduleID == "" {
		return "", fmt.Errorf("moduleID is required")
	}

	if endpointID == "" {
		return moduleID, nil
	}

	return fmt.Sprintf("%s:%s", moduleID, endpointID), nil
}

func (m *manager) ProvideGatewayFactory(moduleID, endpointID string, factory GatewayFactoryUnion) error {
	key, err := m.getModuleEndpointKey(moduleID, endpointID)
	if err != nil {
		return err
	}

	m.mx.Lock()
	defer m.mx.Unlock()

	m.remoteGatewayFactories[key] = factory
	return nil
}

func (m *manager) GetGatewayFactoryInfo(moduleID, endpointID string) (*GatewayFactoryInfo, error) {
	key, err := m.getModuleEndpointKey(moduleID, endpointID)
	if err != nil {
		return nil, err
	}

	m.mx.Lock()
	defer m.mx.Unlock()

	factory, ok := m.remoteGatewayFactories[key]
	if !ok {
		return nil, fmt.Errorf("gateway factory %s not found", key)
	}

	var directClosure interface{}
	if factory.EnableDirect {
		directClosure, ok = m.directClosures[key]
		if !ok {
			return nil, fmt.Errorf("direct closure %s not found", key)
		}
	}

	return &GatewayFactoryInfo{
		Factory:       factory,
		DirectClosure: directClosure,
	}, nil
}

func (m *manager) ProvideHandlerRegistrar(moduleID, endpointID string, registrar HandlerRegistrarUnion) error {
	key, err := m.getModuleEndpointKey(moduleID, endpointID)
	if err != nil {
		return err
	}

	m.mx.Lock()
	defer m.mx.Unlock()

	m.remoteHandlerRegistrars[key] = registrar
	return nil
}

func (m *manager) GetAllHandlerRegistrarInfos() map[string]HandlerRegistrarInfo {
	m.mx.Lock()
	defer m.mx.Unlock()

	infos := make(map[string]HandlerRegistrarInfo)
	for key, registrar := range m.remoteHandlerRegistrars {
		directClosure, _ := m.directClosures[key]
		infos[key] = HandlerRegistrarInfo{
			Registrar:     registrar,
			DirectClosure: directClosure,
		}
	}

	return infos
}

func (m *manager) ProvideDirectClosure(moduleID, endpointID string, closure interface{}) error {
	key, err := m.getModuleEndpointKey(moduleID, endpointID)
	if err != nil {
		return err
	}

	m.mx.Lock()
	defer m.mx.Unlock()

	m.directClosures[key] = closure
	return nil
}
