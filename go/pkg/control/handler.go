package control

import (
	"context"

	"github.com/core-tools/hsu-core/pkg/domain"
	"github.com/core-tools/hsu-core/pkg/generated/api/proto"
	"github.com/core-tools/hsu-core/pkg/logging"

	"google.golang.org/grpc"
)

func RegisterGRPCServerHandler(server grpc.ServiceRegistrar, handler domain.Contract, logger logging.Logger) {
	proto.RegisterCoreServiceServer(server, &grpcServerHandler{
		handler: handler,
		logger:  logger,
	})
}

type grpcServerHandler struct {
	proto.UnimplementedCoreServiceServer
	handler domain.Contract
	logger  logging.Logger
}

func (h *grpcServerHandler) Ping(ctx context.Context, pingRequest *proto.PingRequest) (*proto.PingResponse, error) {
	err := h.handler.Ping(ctx)
	if err != nil {
		h.logger.Errorf("Ping server handler: %v", err)
		return nil, err
	}
	msg := proto.PingResponse{}
	h.logger.Debugf("Ping server handler done")
	return &msg, nil
}
