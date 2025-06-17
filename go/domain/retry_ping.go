package domain

import (
	"context"
	"strings"
	"time"

	"github.com/ai-core-tools/hsu-core/go/logging"
)

type RetryPingOptions struct {
	RetryAttempts int
	RetryInterval time.Duration
}

func RetryPing(ctx context.Context, client Contract, options RetryPingOptions, logger logging.Logger) error {
	logger.Infof("Pinging...")

	var err error
	retryAttempts := options.RetryAttempts
	retryInterval := options.RetryInterval
	for retryAttempts > 0 {
		err = client.Ping(ctx)
		if err == nil {
			break
		}

		if !strings.Contains(err.Error(), "connectex: No connection could be made because the target machine actively refused it.") {
			break
		}

		logger.Infof("Ping failed, retrying in %s", retryInterval)

		time.Sleep(retryInterval)

		retryInterval = retryInterval * 2
		retryAttempts--

		logger.Infof("Retrying Ping...")
	}

	if err != nil {
		logger.Errorf("Failed to ping: %v", err)
		return err
	}

	logger.Infof("Ping done")
	return nil
}
