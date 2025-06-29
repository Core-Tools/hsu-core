package domain

import (
	"context"

	"github.com/core-tools/hsu-core/go/logging"
)

func NewDefaultHandler(logger logging.Logger) Contract {
	return &defaultHandler{
		logger: logger,
	}
}

type defaultHandler struct {
	logger logging.Logger
}

func (h *defaultHandler) Ping(ctx context.Context) error {
	h.logger.Debugf("Default handler: Ping")
	return nil
}
