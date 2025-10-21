package moduleproto

import (
	"context"
	"sync"

	"github.com/core-tools/hsu-core/pkg/errors"
	"github.com/core-tools/hsu-core/pkg/logging"
	"github.com/core-tools/hsu-core/pkg/modulemanagement/moduletypes"
)

type ServerManager interface {
	RegisterProtocolServer(serverID ServerID, server ProtocolServer) error
	FindProtocolServer(serverID ServerID) ProtocolServer
	moduletypes.Lifecycle
}

func NewServerManager(logger logging.Logger) ServerManager {
	return &serverManager{
		servers: make(map[ServerID]ProtocolServer),
		logger:  logger,
	}
}

type serverManager struct {
	servers map[ServerID]ProtocolServer
	logger  logging.Logger
	mx      sync.Mutex
}

func (r *serverManager) RegisterProtocolServer(serverID ServerID, server ProtocolServer) error {
	r.mx.Lock()
	defer r.mx.Unlock()

	if _, exists := r.servers[serverID]; exists {
		return errors.NewConflictError("protocol server already registered", nil).
			WithContext("serverID", serverID)
	}

	r.servers[serverID] = server
	return nil
}

func (r *serverManager) FindProtocolServer(serverID ServerID) ProtocolServer {
	r.mx.Lock()
	defer r.mx.Unlock()

	if server, exists := r.servers[serverID]; exists {
		return server
	}

	return nil
}

func (r *serverManager) getServers() []ProtocolServer {
	r.mx.Lock()
	defer r.mx.Unlock()

	servers := make([]ProtocolServer, 0, len(r.servers))
	for _, server := range r.servers {
		servers = append(servers, server)
	}
	return servers
}

func (r *serverManager) Start(ctx context.Context) error {
	servers := r.getServers()

	errCollection := errors.NewErrorCollection()

	for _, server := range servers {
		err := server.Start(ctx)
		errCollection.Add(err)
	}

	return errCollection.ToError()
}

func (r *serverManager) Stop(ctx context.Context) error {
	servers := r.getServers()

	errCollection := errors.NewErrorCollection()

	for _, server := range servers {
		err := server.Stop(ctx)
		errCollection.Add(err)
	}

	return errCollection.ToError()
}
