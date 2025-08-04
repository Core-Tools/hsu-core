package runtime

import (
	"context"
	"fmt"

	"github.com/core-tools/hsu-core/pkg/logging"
	"github.com/core-tools/hsu-core/pkg/master"
	"github.com/core-tools/hsu-core/pkg/modules"

	"google.golang.org/grpc"
)

type gatewayFactory struct {
	factoryInfoReader modules.GatewayFactoryInfoReader
	workerLifecycle   master.WorkerLifecycle
	logger            logging.Logger
}

func NewGatewayFactory(factoryInfoReader modules.GatewayFactoryInfoReader, workerLifecycle master.WorkerLifecycle, logger logging.Logger) modules.GatewayFactory {
	return &gatewayFactory{
		factoryInfoReader: factoryInfoReader,
		workerLifecycle:   workerLifecycle,
		logger:            logger,
	}
}

func (gf *gatewayFactory) NewGateway(ctx context.Context, moduleID, endpointID string) (interface{}, error) {
	if gf.factoryInfoReader == nil {
		return nil, fmt.Errorf("factory info reader is not set")
	}

	gatewayFactoryInfo, err := gf.factoryInfoReader.GetGatewayFactoryInfo(moduleID, endpointID)
	if err != nil {
		return nil, err
	}

	if gatewayFactoryInfo.Factory.GRPC != nil {
		return gf.safeNewGRPCGateway(ctx, moduleID, gatewayFactoryInfo.Factory.GRPC)
	}

	if gatewayFactoryInfo.Factory.EnableDirect {
		return gatewayFactoryInfo.DirectClosure, nil
	}

	return nil, fmt.Errorf("gateway factory is not registered for module %s and endpoint %s", moduleID, endpointID)
}

func (gf *gatewayFactory) safeNewGRPCGateway(ctx context.Context, moduleID string, gatewayFactory *modules.GRPCGatewayFactory) (interface{}, error) {
	if gf.workerLifecycle == nil {
		return nil, fmt.Errorf("worker lifecycle is not set")
	}

	/*
		err := gf.workerLifecycle.StartWorker(ctx, moduleID) // ensure worker is running
		if err != nil {
			return nil, err
		}
	*/

	workerContext, err := gf.workerLifecycle.GetWorkerContext(moduleID)
	if err != nil {
		return nil, err
	}

	var address string
	if workerContext != nil {
		address = workerContext["server_address"]
	}
	options := gatewayFactory.DialOptions
	if len(options) == 0 {
		options = []grpc.DialOption{
			grpc.WithInsecure(),
		}
	}

	conn, err := grpc.Dial(address, options...)
	if err != nil {
		return nil, err
	}

	return gatewayFactory.FactoryFunc(conn, gf.logger), nil
}
