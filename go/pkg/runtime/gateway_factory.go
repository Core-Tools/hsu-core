package runtime

import (
	"context"

	"github.com/core-tools/hsu-core/pkg/errors"
	"github.com/core-tools/hsu-core/pkg/logging"
	"github.com/core-tools/hsu-core/pkg/modules"
	"github.com/core-tools/hsu-core/pkg/processmanager"

	"google.golang.org/grpc"
)

type gatewayFactory struct {
	factoryInfoReader modules.GatewayFactoryInfoReader
	workerLifecycle   processmanager.ProcessLifecycle
	logger            logging.Logger
}

func NewGatewayFactory(factoryInfoReader modules.GatewayFactoryInfoReader, workerLifecycle processmanager.ProcessLifecycle, logger logging.Logger) modules.GatewayFactory {
	gf := &gatewayFactory{
		factoryInfoReader: factoryInfoReader,
		workerLifecycle:   workerLifecycle,
		logger:            logger,
	}
	logger.Infof("Gateway factory created (has worker lifecycle: %t)", workerLifecycle != nil)
	return gf
}

func (gf *gatewayFactory) NewGateway(ctx context.Context, moduleID, endpointID string) (interface{}, error) {
	if moduleID == "" {
		return nil, errors.NewValidationError("module ID cannot be empty", nil)
	}

	if gf.factoryInfoReader == nil {
		return nil, errors.NewInternalError("factory info reader is not configured", nil)
	}

	gf.logger.Debugf("Creating gateway for module: %s, endpoint: %s", moduleID, endpointID)

	gatewayFactoryInfo, err := gf.factoryInfoReader.GetGatewayFactoryInfo(moduleID, endpointID)
	if err != nil {
		return nil, errors.NewNotFoundError("failed to get gateway factory info", err).
			WithContext("module_id", moduleID).
			WithContext("endpoint_id", endpointID)
	}

	if gatewayFactoryInfo.Factory.GRPC != nil {
		gf.logger.Debugf("Creating gRPC gateway for module: %s", moduleID)
		return gf.safeNewGRPCGateway(ctx, moduleID, gatewayFactoryInfo.Factory.GRPC)
	}

	if gatewayFactoryInfo.Factory.EnableDirect {
		gf.logger.Debugf("Using direct closure for module: %s", moduleID)
		return gatewayFactoryInfo.DirectClosure, nil
	}

	return nil, errors.NewValidationError("no valid gateway configuration found", nil).
		WithContext("module_id", moduleID).
		WithContext("endpoint_id", endpointID).
		WithContext("has_grpc", gatewayFactoryInfo.Factory.GRPC != nil).
		WithContext("enable_direct", gatewayFactoryInfo.Factory.EnableDirect)
}

func (gf *gatewayFactory) safeNewGRPCGateway(ctx context.Context, moduleID string, gatewayConfig *modules.GRPCGatewayFactory) (interface{}, error) {
	if gf.workerLifecycle == nil {
		return nil, errors.NewInternalError("worker lifecycle is not configured for gRPC gateway creation", nil).
			WithContext("module_id", moduleID)
	}

	if gatewayConfig == nil {
		return nil, errors.NewValidationError("gRPC gateway configuration is nil", nil).
			WithContext("module_id", moduleID)
	}

	if gatewayConfig.FactoryFunc == nil {
		return nil, errors.NewValidationError("gRPC gateway factory function is nil", nil).
			WithContext("module_id", moduleID)
	}

	// Get worker context to determine server address
	workerContext, err := gf.workerLifecycle.GetWorkerContext(moduleID)
	if err != nil {
		return nil, errors.NewProcessError("failed to get worker context", err).
			WithContext("module_id", moduleID)
	}

	var address string
	if workerContext != nil {
		address = workerContext["server_address"]
	}

	if address == "" {
		return nil, errors.NewProcessError("server address not available in worker context", nil).
			WithContext("module_id", moduleID).
			WithContext("worker_context_nil", workerContext == nil)
	}

	gf.logger.Debugf("Connecting to gRPC server for module %s at address: %s", moduleID, address)

	// Setup dial options
	options := gatewayConfig.DialOptions
	if len(options) == 0 {
		options = []grpc.DialOption{
			grpc.WithInsecure(),
		}
		gf.logger.Debugf("Using default insecure gRPC dial options for module: %s", moduleID)
	}

	// Create gRPC connection
	conn, err := grpc.Dial(address, options...)
	if err != nil {
		return nil, errors.NewIOError("failed to establish gRPC connection", err).
			WithContext("module_id", moduleID).
			WithContext("address", address)
	}

	gf.logger.Infof("gRPC gateway created successfully for module %s at %s", moduleID, address)

	// Create and return the gateway
	gateway := gatewayConfig.FactoryFunc(conn, gf.logger)
	if gateway == nil {
		conn.Close() // Clean up connection if factory returns nil
		return nil, errors.NewInternalError("gateway factory function returned nil", nil).
			WithContext("module_id", moduleID)
	}

	return gateway, nil
}
