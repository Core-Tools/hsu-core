package modulemanagement

import (
	"context"
	"os"
	"os/signal"
	osruntime "runtime"
	"syscall"

	"github.com/core-tools/hsu-core/pkg/logging"
)

func WaitSignals(ctx context.Context, logger logging.Logger) {
	// Enable signal handling
	sig := make(chan os.Signal, 1)
	if osruntime.GOOS == "windows" {
		signal.Notify(sig) // Unix signals not implemented on Windows
	} else {
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	}

	// Wait for graceful shutdown or timeout
	select {
	case receivedSignal := <-sig:
		logger.Infof("Received signal: %v", receivedSignal)
		if osruntime.GOOS == "windows" {
			if receivedSignal != os.Interrupt {
				logger.Errorf("Wrong signal received: got %q, want %q\n", receivedSignal, os.Interrupt)
				os.Exit(42)
			}
		}
	case <-ctx.Done():
		logger.Infof("WaitSignals context done")
	}
}
