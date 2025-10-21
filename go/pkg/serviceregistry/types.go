package serviceregistry

import (
	"time"

	"github.com/core-tools/hsu-core/pkg/modulemanagement/moduletypes"
)

// Re-export module types for convenience
type ModuleID = moduletypes.ModuleID
type ServiceID = moduletypes.ServiceID
type Protocol = moduletypes.Protocol

// RemoteModuleAPI represents a single API endpoint for a module
type RemoteModuleAPI struct {
	ServiceIDs []ServiceID       `json:"serviceIDs"`
	ServerPort int               `json:"serverPort"`
	Protocol   Protocol          `json:"protocol"`
	Address    string            `json:"address,omitempty"` // Empty means localhost
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// ModuleRegistration represents a complete module registration
type ModuleRegistration struct {
	ModuleID     ModuleID          `json:"moduleID"`
	APIs         []RemoteModuleAPI `json:"apis"`
	ProcessID    int               `json:"processID"`
	RegisteredAt time.Time         `json:"registeredAt"`
	LastSeen     time.Time         `json:"lastSeen"`
}

// PublishRequest is the request format for publishing APIs
type PublishRequest struct {
	ModuleID  ModuleID          `json:"moduleID"`
	ProcessID int               `json:"processID"`
	APIs      []RemoteModuleAPI `json:"apis"`
}

// DiscoverResponse is the response format for discovering APIs
type DiscoverResponse struct {
	ModuleID ModuleID          `json:"moduleID"`
	APIs     []RemoteModuleAPI `json:"apis"`
}

// ErrorResponse is the response format for errors
type ErrorResponse struct {
	Success bool        `json:"success"`
	Error   string      `json:"error"`
	Context interface{} `json:"context,omitempty"`
}

// HealthResponse is the response format for health checks
type HealthResponse struct {
	Status            string `json:"status"`
	RegisteredModules int    `json:"registeredModules"`
	TotalAPIs         int    `json:"totalAPIs"`
	Uptime            string `json:"uptime"`
}
