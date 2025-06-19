package control

import (
	"context"

	"github.com/core-tools/hsu-core/go/api/proto"
	"github.com/core-tools/hsu-core/go/domain"
	"github.com/core-tools/hsu-core/go/logging"
	"google.golang.org/grpc"
)

func NewGRPCClientGateway(grpcClientConnection grpc.ClientConnInterface, logger logging.Logger) domain.Contract {
	grpcClient := proto.NewCoreServiceClient(grpcClientConnection)
	return &grpcClientGateway{
		grpcClient: grpcClient,
		logger:     logger,
	}
}

type grpcClientGateway struct {
	grpcClient proto.CoreServiceClient
	logger     logging.Logger
}

func (gw *grpcClientGateway) Ping(ctx context.Context) error {
	_, err := gw.grpcClient.Ping(ctx, &proto.PingRequest{})
	if err != nil {
		gw.logger.Errorf("Ping client gateway: %v", err)
		return err
	}
	gw.logger.Debugf("Ping client gateway done")
	return nil
}
