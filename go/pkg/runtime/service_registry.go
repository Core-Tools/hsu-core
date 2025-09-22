package runtime

import (
	"sync"

	"github.com/core-tools/hsu-core/pkg/errors"
)

type RemoteModuleServerAddresses struct {
	Protocols map[string]string
}

type RemoteModuleGetter interface {
	FindModule(moduleID string) (*RemoteModuleServerAddresses, error)
}

type RemoteModuleRegistry interface {
	RegisterModule(moduleID string, addresses RemoteModuleServerAddresses) error
	UnregisterModule(moduleID string) error
}

type RemoteModuleManager interface {
	RemoteModuleGetter
	RemoteModuleRegistry
}

func NewRemoteModuleManager() RemoteModuleManager {
	return &remoteModuleRegistry{
		modules: make(map[string]RemoteModuleServerAddresses),
	}
}

type remoteModuleRegistry struct {
	modules map[string]RemoteModuleServerAddresses
	mx      sync.Mutex
}

func (r *remoteModuleRegistry) RegisterModule(moduleID string, addresses RemoteModuleServerAddresses) error {
	r.mx.Lock()
	defer r.mx.Unlock()

	if _, exists := r.modules[moduleID]; exists {
		return errors.NewConflictError("module already registered", nil).WithContext("module_id", moduleID)
	}

	r.modules[moduleID] = addresses
	return nil
}

func (r *remoteModuleRegistry) UnregisterModule(moduleID string) error {
	r.mx.Lock()
	defer r.mx.Unlock()

	if _, exists := r.modules[moduleID]; !exists {
		return errors.NewNotFoundError("module not found", nil).WithContext("module_id", moduleID)
	}

	delete(r.modules, moduleID)
	return nil
}

func (r *remoteModuleRegistry) FindModule(moduleID string) (*RemoteModuleServerAddresses, error) {
	r.mx.Lock()
	defer r.mx.Unlock()

	if addresses, exists := r.modules[moduleID]; exists {
		return &addresses, nil
	}

	return nil, errors.NewNotFoundError("module not found", nil).WithContext("module_id", moduleID)
}
