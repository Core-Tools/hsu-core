package processmanager

import (
	"context"
	"fmt"

	"github.com/core-tools/hsu-core/pkg/logcollection"
	"github.com/core-tools/hsu-core/pkg/logcollection/config"
	"github.com/core-tools/hsu-core/pkg/logging"
	"github.com/core-tools/hsu-core/pkg/processfile"
)

// LogCollectionIntegration handles log collection setup and management for the process manager service
type LogCollectionIntegration struct {
	service     logcollection.LogCollectionService
	pathManager *processfile.ProcessFileManager
	logger      logging.Logger
	enabled     bool
}

// NewLogCollectionIntegration creates a new log collection integration
func NewLogCollectionIntegration(logConfig *config.LogCollectionConfig, logger logging.Logger) (*LogCollectionIntegration, error) {
	integration := &LogCollectionIntegration{
		logger:  logger,
		enabled: false,
	}

	// Check if log collection is configured in config file
	if logConfig != nil && logConfig.Enabled {
		// Use config from file
		logger.Infof("Using log collection configuration from config file")
	} else if logConfig == nil {
		// No log collection section in config - use defaults
		defaultConfig := config.DefaultLogCollectionConfig()
		logConfig = &defaultConfig
		logger.Infof("No log_collection section found in config - using default log collection configuration")
	} else {
		// Log collection is explicitly disabled in config
		logger.Debugf("Log collection is explicitly disabled in configuration")
		return integration, nil
	}

	// Create path manager for log files
	pathManager := processfile.NewProcessFileManager(processfile.ProcessFileConfig{}, &loggerAdapter{logger})
	integration.pathManager = pathManager

	// Create structured logger for log collection service
	structuredLogger, err := logcollection.NewStructuredLogger("zap", logcollection.InfoLevel)
	if err != nil {
		return nil, fmt.Errorf("failed to create structured logger: %w", err)
	}

	// Create log collection service with path manager
	service := logcollection.NewLogCollectionServiceWithPathManager(
		*logConfig,
		structuredLogger,
		pathManager,
	)

	integration.service = service
	integration.enabled = true

	logger.Infof("Log collection integration initialized successfully (enabled: %t)", integration.enabled)

	// Log configuration summary for debugging
	logger.Infof("Log collection config summary:")
	logger.Infof("  - Global aggregation enabled: %t", logConfig.GlobalAggregation.Enabled)
	logger.Infof("  - Default worker capture stdout: %t", logConfig.DefaultWorker.CaptureStdout)
	logger.Infof("  - Default worker capture stderr: %t", logConfig.DefaultWorker.CaptureStderr)
	logger.Infof("  - Managed process directory template: %s", logConfig.System.WorkerDirectory)
	if len(logConfig.GlobalAggregation.Targets) > 0 {
		logger.Infof("  - Global aggregation targets: %d configured", len(logConfig.GlobalAggregation.Targets))
		for i, target := range logConfig.GlobalAggregation.Targets {
			logger.Infof("    [%d] Type: %s, Format: %s", i, target.Type, target.Format)
			if target.Path != "" {
				logger.Infof("        Path: %s", target.Path)
			}
		}
	}

	return integration, nil
}

// Start starts the log collection service
func (l *LogCollectionIntegration) Start(ctx context.Context) error {
	if !l.enabled {
		return nil
	}

	if err := l.service.Start(ctx); err != nil {
		return fmt.Errorf("failed to start log collection service: %w", err)
	}

	l.logger.Infof("Log collection service started")
	return nil
}

// Stop stops the log collection service
func (l *LogCollectionIntegration) Stop() error {
	if !l.enabled {
		return nil
	}

	if err := l.service.Stop(); err != nil {
		return fmt.Errorf("failed to stop log collection service: %w", err)
	}

	l.logger.Infof("Log collection service stopped")
	return nil
}

// GetLogCollectionService returns the log collection service for use by managed processes
func (l *LogCollectionIntegration) GetLogCollectionService() logcollection.LogCollectionService {
	if !l.enabled {
		return nil
	}
	return l.service
}

// GetWorkerLogConfig returns the appropriate log configuration for a worker
func (l *LogCollectionIntegration) GetWorkerLogConfig(workerID string, workerConfig WorkerConfig) *config.WorkerLogConfig {
	if !l.enabled {
		return nil
	}

	// Check if worker has specific log collection configuration
	// For now, we'll look for this in the worker config structure
	// TODO: Add worker-specific log config parsing when needed

	// Use default worker log configuration
	defaultConfig := config.DefaultWorkerLogConfig()
	return &defaultConfig
}

// IsEnabled returns whether log collection is enabled
func (l *LogCollectionIntegration) IsEnabled() bool {
	return l.enabled
}

// GetPathManager returns the path manager for log file path resolution
func (l *LogCollectionIntegration) GetPathManager() *processfile.ProcessFileManager {
	return l.pathManager
}

// ===== ADAPTER =====

// loggerAdapter adapts the process manager logging.Logger to the processfile logging interface
type loggerAdapter struct {
	logger logging.Logger
}

func (l *loggerAdapter) Debugf(format string, args ...interface{}) {
	l.logger.Debugf(format, args...)
}

func (l *loggerAdapter) Infof(format string, args ...interface{}) {
	l.logger.Infof(format, args...)
}

func (l *loggerAdapter) Warnf(format string, args ...interface{}) {
	l.logger.Warnf(format, args...)
}

func (l *loggerAdapter) Errorf(format string, args ...interface{}) {
	l.logger.Errorf(format, args...)
}

func (l *loggerAdapter) LogLevelf(level int, format string, args ...interface{}) {
	switch level {
	case 0:
		l.logger.Debugf(format, args...)
	case 1:
		l.logger.Infof(format, args...)
	case 2:
		l.logger.Warnf(format, args...)
	case 3:
		l.logger.Errorf(format, args...)
	default:
		l.logger.Infof(format, args...)
	}
}
