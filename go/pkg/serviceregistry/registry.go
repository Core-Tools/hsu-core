package serviceregistry

import (
	"fmt"
	"sync"
	"time"

	"github.com/core-tools/hsu-core/pkg/errors"
	"github.com/core-tools/hsu-core/pkg/logging"
)

// Registry manages service registrations in memory
type Registry struct {
	modules   map[ModuleID]*ModuleRegistration
	mu        sync.RWMutex
	logger    logging.Logger
	startTime time.Time
	stopChan  chan struct{}
}

// NewRegistry creates a new service registry
func NewRegistry(logger logging.Logger) *Registry {
	if logger == nil {
		logger = logging.NewNullLogger()
	}

	r := &Registry{
		modules:   make(map[ModuleID]*ModuleRegistration),
		logger:    logger,
		startTime: time.Now(),
		stopChan:  make(chan struct{}),
	}

	// Start background cleanup goroutine
	go r.cleanupLoop()

	return r
}

// Publish registers or updates module APIs
func (r *Registry) Publish(req PublishRequest) error {
	if req.ModuleID == "" {
		return errors.NewValidationError("module ID is required", nil)
	}

	if len(req.APIs) == 0 {
		return errors.NewValidationError("at least one API must be provided", nil).
			WithContext("module_id", req.ModuleID)
	}

	// Validate APIs
	for i, api := range req.APIs {
		if len(api.ServiceIDs) == 0 {
			return errors.NewValidationError("API must have at least one service ID", nil).
				WithContext("module_id", req.ModuleID).
				WithContext("api_index", i)
		}
		if api.ServerPort <= 0 || api.ServerPort > 65535 {
			return errors.NewValidationError("invalid server port", nil).
				WithContext("module_id", req.ModuleID).
				WithContext("port", api.ServerPort)
		}
		if api.Protocol == "" {
			return errors.NewValidationError("protocol is required", nil).
				WithContext("module_id", req.ModuleID).
				WithContext("api_index", i)
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()

	// Check if module already exists - preserve original registered time
	existingReg, exists := r.modules[req.ModuleID]
	registeredAt := now
	if exists {
		registeredAt = existingReg.RegisteredAt
	}

	r.modules[req.ModuleID] = &ModuleRegistration{
		ModuleID:     req.ModuleID,
		APIs:         req.APIs,
		ProcessID:    req.ProcessID,
		RegisteredAt: registeredAt,
		LastSeen:     now,
	}

	if exists {
		r.logger.Infof("Updated APIs for module %s: %d APIs", req.ModuleID, len(req.APIs))
	} else {
		r.logger.Infof("Registered module %s: %d APIs, processID=%d", req.ModuleID, len(req.APIs), req.ProcessID)
	}

	return nil
}

// Discover retrieves registered APIs for a module
func (r *Registry) Discover(moduleID ModuleID) ([]RemoteModuleAPI, error) {
	if moduleID == "" {
		return nil, errors.NewValidationError("module ID is required", nil)
	}

	r.mu.RLock()
	registration, ok := r.modules[moduleID]
	r.mu.RUnlock()

	if !ok {
		return nil, errors.NewNotFoundError("module not found", nil).
			WithContext("module_id", moduleID)
	}

	// Check staleness (5 minutes)
	if time.Since(registration.LastSeen) > 5*time.Minute {
		r.logger.Warnf("Module %s registration is stale (last seen: %s)", moduleID, registration.LastSeen)
		return nil, errors.NewNotFoundError("module registration is stale", nil).
			WithContext("module_id", moduleID).
			WithContext("last_seen", registration.LastSeen)
	}

	// Update heartbeat (discovery acts as heartbeat)
	r.mu.Lock()
	if reg, ok := r.modules[moduleID]; ok {
		reg.LastSeen = time.Now()
	}
	r.mu.Unlock()

	// Return copy to avoid external mutation
	apis := make([]RemoteModuleAPI, len(registration.APIs))
	copy(apis, registration.APIs)

	r.logger.Debugf("Discovered %d APIs for module %s", len(apis), moduleID)

	return apis, nil
}

// Unpublish removes a module registration
func (r *Registry) Unpublish(moduleID ModuleID, processID int) error {
	if moduleID == "" {
		return errors.NewValidationError("module ID is required", nil)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	registration, ok := r.modules[moduleID]
	if !ok {
		// Idempotent - no error if module not found
		return nil
	}

	// If processID specified, verify it matches
	if processID != 0 && registration.ProcessID != processID {
		return errors.NewValidationError("process ID mismatch", nil).
			WithContext("module_id", moduleID).
			WithContext("expected_process_id", registration.ProcessID).
			WithContext("actual_process_id", processID)
	}

	delete(r.modules, moduleID)
	r.logger.Infof("Unpublished module %s", moduleID)

	return nil
}

// GetStats returns registry statistics
func (r *Registry) GetStats() (registeredModules, totalAPIs int, uptime time.Duration) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	registeredModules = len(r.modules)
	for _, reg := range r.modules {
		totalAPIs += len(reg.APIs)
	}
	uptime = time.Since(r.startTime)

	return
}

// Stop stops the registry and cleanup goroutine
func (r *Registry) Stop() {
	close(r.stopChan)
}

// cleanupLoop periodically removes stale registrations
func (r *Registry) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			r.cleanupStale(10 * time.Minute)
		case <-r.stopChan:
			r.logger.Infof("Registry cleanup loop stopped")
			return
		}
	}
}

// cleanupStale removes registrations older than maxAge
func (r *Registry) cleanupStale(maxAge time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	removed := make([]ModuleID, 0)

	for moduleID, registration := range r.modules {
		if now.Sub(registration.LastSeen) > maxAge {
			delete(r.modules, moduleID)
			removed = append(removed, moduleID)
		}
	}

	if len(removed) > 0 {
		r.logger.Warnf("Cleaned up %d stale registrations: %v", len(removed), removed)
	}
}

// ListModules returns all registered module IDs (for debugging/admin)
func (r *Registry) ListModules() []ModuleID {
	r.mu.RLock()
	defer r.mu.RUnlock()

	modules := make([]ModuleID, 0, len(r.modules))
	for moduleID := range r.modules {
		modules = append(modules, moduleID)
	}

	return modules
}

// GetRegistration returns the full registration for a module (for debugging/admin)
func (r *Registry) GetRegistration(moduleID ModuleID) (*ModuleRegistration, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	registration, ok := r.modules[moduleID]
	if !ok {
		return nil, fmt.Errorf("module not found: %s", moduleID)
	}

	// Return copy
	regCopy := *registration
	regCopy.APIs = make([]RemoteModuleAPI, len(registration.APIs))
	copy(regCopy.APIs, registration.APIs)

	return &regCopy, nil
}
