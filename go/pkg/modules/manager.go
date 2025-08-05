package modules

import (
	"context"
	"fmt"
	"sync"

	"github.com/core-tools/hsu-core/pkg/errors"
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
	remoteGatewayFactories  map[string]GatewayConfig
	remoteHandlerRegistrars map[string]HandlerConfig
	logger                  logging.Logger
	mx                      sync.Mutex
}

func NewManager(logger logging.Logger) Manager {
	return &manager{
		modules:                 make(map[string]Module),
		directClosures:          make(map[string]interface{}),
		remoteGatewayFactories:  make(map[string]GatewayConfig),
		remoteHandlerRegistrars: make(map[string]HandlerConfig),
		logger:                  logger,
	}
}

func (m *manager) RegisterModule(moduleID string, module Module) error {
	if moduleID == "" {
		return errors.NewValidationError("module ID cannot be empty", nil)
	}
	if module == nil {
		return errors.NewValidationError("module cannot be nil", nil).WithContext("module_id", moduleID)
	}

	m.mx.Lock()
	defer m.mx.Unlock()

	if _, exists := m.modules[moduleID]; exists {
		return errors.NewConflictError("module already registered", nil).WithContext("module_id", moduleID)
	}

	m.modules[moduleID] = module
	m.logger.Infof("Module registered: %s", moduleID)
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
	m.logger.Infof("Initializing %d modules", len(modules))

	errorCollection := errors.NewErrorCollection()

	for _, module := range modules {
		moduleID := module.ID()
		m.logger.Debugf("Initializing module: %s", moduleID)
		
		err := module.Initialize(m)
		if err != nil {
			wrappedErr := errors.NewInternalError("module initialization failed", err).
				WithContext("module_id", moduleID)
			errorCollection.Add(wrappedErr)
			m.logger.Errorf("Failed to initialize module %s: %v", moduleID, err)
			continue
		}
		
		m.logger.Infof("Module initialized successfully: %s", moduleID)
	}

	if errorCollection.HasErrors() {
		return errors.NewInternalError("some modules failed to initialize", errorCollection.ToError())
	}

	m.logger.Infof("All modules initialized successfully")
	return nil
}

func (m *manager) Start(ctx context.Context, gatewayFactory GatewayFactory) error {
	modules := m.getModules()
	m.logger.Infof("Starting %d modules", len(modules))

	errorCollection := errors.NewErrorCollection()

	for _, module := range modules {
		moduleID := module.ID()
		m.logger.Debugf("Starting module: %s", moduleID)
		
		err := module.Start(ctx, gatewayFactory)
		if err != nil {
			wrappedErr := errors.NewInternalError("module start failed", err).
				WithContext("module_id", moduleID)
			errorCollection.Add(wrappedErr)
			m.logger.Errorf("Failed to start module %s: %v", moduleID, err)
			continue
		}
		
		m.logger.Infof("Module started successfully: %s", moduleID)
	}

	if errorCollection.HasErrors() {
		return errors.NewInternalError("some modules failed to start", errorCollection.ToError())
	}

	m.logger.Infof("All modules started successfully")
	return nil
}

func (m *manager) Stop(ctx context.Context) error {
	modules := m.getModules()
	m.logger.Infof("Stopping %d modules", len(modules))

	errorCollection := errors.NewErrorCollection()

	for _, module := range modules {
		moduleID := module.ID()
		m.logger.Debugf("Stopping module: %s", moduleID)
		
		err := module.Stop(ctx)
		if err != nil {
			wrappedErr := errors.NewInternalError("module stop failed", err).
				WithContext("module_id", moduleID)
			errorCollection.Add(wrappedErr)
			m.logger.Errorf("Failed to stop module %s: %v", moduleID, err)
			continue
		}
		
		m.logger.Infof("Module stopped successfully: %s", moduleID)
	}

	if errorCollection.HasErrors() {
		return errors.NewInternalError("some modules failed to stop", errorCollection.ToError())
	}

	m.logger.Infof("All modules stopped successfully")
	return nil
}

func (m *manager) getModuleEndpointKey(moduleID, endpointID string) (string, error) {
	if moduleID == "" {
		return "", errors.NewValidationError("module ID cannot be empty", nil)
	}

	if endpointID == "" {
		return moduleID, nil
	}

	return fmt.Sprintf("%s:%s", moduleID, endpointID), nil
}

func (m *manager) ProvideGatewayFactory(moduleID, endpointID string, factory GatewayConfig) error {
	key, err := m.getModuleEndpointKey(moduleID, endpointID)
	if err != nil {
		return err
	}

	m.mx.Lock()
	defer m.mx.Unlock()

	if _, exists := m.remoteGatewayFactories[key]; exists {
		m.logger.Warnf("Gateway factory already exists for %s, overwriting", key)
	}

	m.remoteGatewayFactories[key] = factory
	m.logger.Infof("Gateway factory registered: %s (gRPC: %t, Direct: %t)", 
		key, factory.GRPC != nil, factory.EnableDirect)
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
		return nil, errors.NewNotFoundError("gateway factory not found", nil).
			WithContext("module_id", moduleID).
			WithContext("endpoint_id", endpointID).
			WithContext("key", key)
	}

	var directClosure interface{}
	if factory.EnableDirect {
		directClosure, ok = m.directClosures[key]
		if !ok {
			return nil, errors.NewNotFoundError("direct closure not found for module with direct communication enabled", nil).
				WithContext("module_id", moduleID).
				WithContext("endpoint_id", endpointID).
				WithContext("key", key)
		}
	}

	m.logger.Debugf("Gateway factory info retrieved: %s (gRPC: %t, Direct: %t)", 
		key, factory.GRPC != nil, factory.EnableDirect)

	return &GatewayFactoryInfo{
		Factory:       factory,
		DirectClosure: directClosure,
	}, nil
}

func (m *manager) ProvideHandlerRegistrar(moduleID, endpointID string, registrar HandlerConfig) error {
	key, err := m.getModuleEndpointKey(moduleID, endpointID)
	if err != nil {
		return err
	}

	m.mx.Lock()
	defer m.mx.Unlock()

	if _, exists := m.remoteHandlerRegistrars[key]; exists {
		m.logger.Warnf("Handler registrar already exists for %s, overwriting", key)
	}

	m.remoteHandlerRegistrars[key] = registrar
	m.logger.Infof("Handler registrar registered: %s (gRPC: %t)", 
		key, registrar.GRPC != nil)
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
	if closure == nil {
		return errors.NewValidationError("direct closure cannot be nil", nil).
			WithContext("module_id", moduleID).
			WithContext("endpoint_id", endpointID)
	}

	key, err := m.getModuleEndpointKey(moduleID, endpointID)
	if err != nil {
		return err
	}

	m.mx.Lock()
	defer m.mx.Unlock()

	if _, exists := m.directClosures[key]; exists {
		m.logger.Warnf("Direct closure already exists for %s, overwriting", key)
	}

	m.directClosures[key] = closure
	m.logger.Infof("Direct closure registered: %s", key)
	return nil
}
