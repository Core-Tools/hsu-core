package serviceregistry

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/core-tools/hsu-core/pkg/errors"
	"github.com/core-tools/hsu-core/pkg/logging"
)

// Server is the HTTP server for the service registry
type Server struct {
	registry  *Registry
	listener  net.Listener
	server    *http.Server
	transport TransportConfig
	logger    logging.Logger
}

// NewServer creates a new registry server
func NewServer(registry *Registry, transport TransportConfig, logger logging.Logger) (*Server, error) {
	if registry == nil {
		return nil, fmt.Errorf("registry is required")
	}
	if logger == nil {
		logger = logging.NewNullLogger()
	}

	// Create listener based on transport
	listener, err := CreateListener(transport)
	if err != nil {
		return nil, fmt.Errorf("failed to create listener: %w", err)
	}

	s := &Server{
		registry:  registry,
		listener:  listener,
		transport: transport,
		logger:    logger,
	}

	// Create HTTP handler
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/publish", s.handlePublish)
	mux.HandleFunc("/api/v1/discover/", s.handleDiscover)
	mux.HandleFunc("/api/v1/unpublish/", s.handleUnpublish)
	mux.HandleFunc("/api/v1/health", s.handleHealth)

	s.server = &http.Server{
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	return s, nil
}

// Start starts the server
func (s *Server) Start(ctx context.Context) error {
	address := GetListenerAddress(s.listener)
	s.logger.Infof("Starting service registry server on %s", address)

	go func() {
		if err := s.server.Serve(s.listener); err != nil && err != http.ErrServerClosed {
			s.logger.Errorf("Server error: %v", err)
		}
	}()

	return nil
}

// Stop stops the server gracefully
func (s *Server) Stop(ctx context.Context) error {
	s.logger.Infof("Stopping service registry server")

	if err := s.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("server shutdown failed: %w", err)
	}

	// Stop registry cleanup loop
	s.registry.Stop()

	return nil
}

// GetAddress returns the server's listen address
func (s *Server) GetAddress() string {
	return GetListenerAddress(s.listener)
}

// HTTP Handlers

func (s *Server) handlePublish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendError(w, http.StatusMethodNotAllowed, "method not allowed", nil)
		return
	}

	var req PublishRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, http.StatusBadRequest, "invalid request body", err)
		return
	}

	if err := s.registry.Publish(req); err != nil {
		s.sendErrorFromDomainError(w, err)
		return
	}

	s.sendSuccess(w, map[string]bool{"success": true})
}

func (s *Server) handleDiscover(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.sendError(w, http.StatusMethodNotAllowed, "method not allowed", nil)
		return
	}

	// Extract module ID from path: /api/v1/discover/:moduleID
	path := r.URL.Path
	prefix := "/api/v1/discover/"
	if !strings.HasPrefix(path, prefix) {
		s.sendError(w, http.StatusBadRequest, "invalid path", nil)
		return
	}

	moduleID := ModuleID(path[len(prefix):])
	if moduleID == "" {
		s.sendError(w, http.StatusBadRequest, "module ID is required", nil)
		return
	}

	apis, err := s.registry.Discover(moduleID)
	if err != nil {
		s.sendErrorFromDomainError(w, err)
		return
	}

	response := DiscoverResponse{
		ModuleID: moduleID,
		APIs:     apis,
	}

	s.sendSuccess(w, response)
}

func (s *Server) handleUnpublish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		s.sendError(w, http.StatusMethodNotAllowed, "method not allowed", nil)
		return
	}

	// Extract module ID from path: /api/v1/unpublish/:moduleID
	path := r.URL.Path
	prefix := "/api/v1/unpublish/"
	if !strings.HasPrefix(path, prefix) {
		s.sendError(w, http.StatusBadRequest, "invalid path", nil)
		return
	}

	moduleID := ModuleID(path[len(prefix):])
	if moduleID == "" {
		s.sendError(w, http.StatusBadRequest, "module ID is required", nil)
		return
	}

	// Optional: process ID from query parameter
	processIDStr := r.URL.Query().Get("processID")
	processID := 0
	if processIDStr != "" {
		if _, err := fmt.Sscanf(processIDStr, "%d", &processID); err != nil {
			s.sendError(w, http.StatusBadRequest, "invalid process ID", err)
			return
		}
	}

	if err := s.registry.Unpublish(moduleID, processID); err != nil {
		s.sendErrorFromDomainError(w, err)
		return
	}

	s.sendSuccess(w, map[string]bool{"success": true})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.sendError(w, http.StatusMethodNotAllowed, "method not allowed", nil)
		return
	}

	registeredModules, totalAPIs, uptime := s.registry.GetStats()

	response := HealthResponse{
		Status:            "healthy",
		RegisteredModules: registeredModules,
		TotalAPIs:         totalAPIs,
		Uptime:            uptime.String(),
	}

	s.sendSuccess(w, response)
}

// Helper methods

func (s *Server) sendSuccess(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		s.logger.Errorf("Failed to encode response: %v", err)
	}
}

func (s *Server) sendError(w http.ResponseWriter, statusCode int, message string, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := ErrorResponse{
		Success: false,
		Error:   message,
	}

	if err != nil {
		response.Context = map[string]string{"details": err.Error()}
	}

	if encErr := json.NewEncoder(w).Encode(response); encErr != nil {
		s.logger.Errorf("Failed to encode error response: %v", encErr)
	}

	s.logger.Warnf("Request error: %s (status: %d)", message, statusCode)
}

func (s *Server) sendErrorFromDomainError(w http.ResponseWriter, err error) {
	statusCode := http.StatusInternalServerError
	message := "internal server error"

	// Map domain errors to HTTP status codes
	if errors.IsNotFoundError(err) {
		statusCode = http.StatusNotFound
		message = "not found"
	} else if errors.IsValidationError(err) {
		statusCode = http.StatusBadRequest
		message = "validation error"
	}

	s.sendError(w, statusCode, message, err)
}
