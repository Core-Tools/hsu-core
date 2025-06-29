package domain

import (
	"context"
)

type Contract interface {
	Ping(ctx context.Context) error
}
