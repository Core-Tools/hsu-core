package logcollection

import (
	"context"
	"fmt"
)

// ===== PUBLIC FACTORY FUNCTIONS =====

// NewStructuredLogger creates a new structured logger with the specified backend
func NewStructuredLogger(backendType string, level LogLevel) (StructuredLogger, error) {
	switch backendType {
	case "zap", "":
		zapConfig := DefaultZapConfig()
		zapConfig.Level = level.String()
		return NewZapAdapter(zapConfig)
	case "logrus":
		// TODO: Implement in Phase 2
		return nil, fmt.Errorf("logrus backend not yet implemented")
	case "slog":
		// TODO: Implement in Phase 2
		return nil, fmt.Errorf("slog backend not yet implemented")
	default:
		return nil, fmt.Errorf("unknown backend type: %s", backendType)
	}
}

// NewStructuredLoggerWithConfig creates a new structured logger with detailed configuration
func NewStructuredLoggerWithConfig(cfg LoggerConfig) (StructuredLogger, error) {
	switch cfg.Backend {
	case "zap", "":
		zapConfig := ZapConfig{
			Level:      cfg.Level.String(),
			Format:     cfg.Format,
			Output:     cfg.Output,
			Caller:     cfg.Caller,
			Stacktrace: cfg.Stacktrace,
		}
		return NewZapAdapter(zapConfig)
	case "logrus":
		// TODO: Implement in Phase 2
		return nil, fmt.Errorf("logrus backend not yet implemented")
	case "slog":
		// TODO: Implement in Phase 2
		return nil, fmt.Errorf("slog backend not yet implemented")
	default:
		return nil, fmt.Errorf("unknown backend type: %s", cfg.Backend)
	}
}

// ===== CONFIGURATION TYPES FOR FACTORY =====

// LoggerConfig defines configuration for creating a structured logger
type LoggerConfig struct {
	Backend    string   `yaml:"backend"`    // "zap", "logrus", "slog"
	Level      LogLevel `yaml:"level"`      // Debug, Info, Warn, Error
	Format     string   `yaml:"format"`     // "json", "console"
	Output     string   `yaml:"output"`     // "stdout", "stderr", file path
	Caller     bool     `yaml:"caller"`     // Include caller information
	Stacktrace bool     `yaml:"stacktrace"` // Include stacktrace on errors
}

// DefaultLoggerConfig returns a sensible default logger configuration
func DefaultLoggerConfig() LoggerConfig {
	return LoggerConfig{
		Backend:    "zap",
		Level:      InfoLevel,
		Format:     "json",
		Output:     "stdout",
		Caller:     true,
		Stacktrace: true,
	}
}

// ===== CONVENIENCE FUNCTIONS =====

// QuickLogger creates a logger with minimal configuration for development
func QuickLogger(level LogLevel) StructuredLogger {
	logger, err := NewStructuredLogger("zap", level)
	if err != nil {
		// Fallback to a basic implementation if zap fails
		panic(fmt.Sprintf("failed to create quick logger: %v", err))
	}
	return logger
}

// ProductionLogger creates a logger optimized for production use
func ProductionLogger(outputPath string) (StructuredLogger, error) {
	cfg := LoggerConfig{
		Backend:    "zap",
		Level:      InfoLevel,
		Format:     "json",
		Output:     outputPath,
		Caller:     false, // Reduce overhead in production
		Stacktrace: true,  // Keep stacktraces for errors
	}

	return NewStructuredLoggerWithConfig(cfg)
}

// DevelopmentLogger creates a logger optimized for development
func DevelopmentLogger() (StructuredLogger, error) {
	cfg := LoggerConfig{
		Backend:    "zap",
		Level:      DebugLevel,
		Format:     "console",
		Output:     "stdout",
		Caller:     true,
		Stacktrace: true,
	}

	return NewStructuredLoggerWithConfig(cfg)
}

// ===== INTEGRATION HELPERS =====

// CreateLoggerForWorker creates a logger instance specifically for a worker
func CreateLoggerForWorker(workerID string, baseLogger StructuredLogger) StructuredLogger {
	return baseLogger.WithWorker(workerID).WithFields(
		Component("worker"),
		String("worker_type", "process"),
	)
}

// CreateLoggerForMaster creates a logger instance specifically for the master
func CreateLoggerForMaster(masterID string, baseLogger StructuredLogger) StructuredLogger {
	return baseLogger.WithFields(
		String("master_id", masterID),
		Component("master"),
		String("component_type", "process_manager"),
	)
}

// CreateLoggerForComponent creates a logger instance for a specific component
func CreateLoggerForComponent(component string, baseLogger StructuredLogger) StructuredLogger {
	return baseLogger.WithFields(
		Component(component),
		String("subsystem", "master"),
	)
}

// ===== VALIDATION HELPERS =====

// ValidateLoggerConfig validates a logger configuration
func ValidateLoggerConfig(cfg LoggerConfig) error {
	// Validate backend
	validBackends := map[string]bool{
		"zap": true, "logrus": true, "slog": true,
	}
	if !validBackends[cfg.Backend] {
		return fmt.Errorf("invalid backend: %s", cfg.Backend)
	}

	// Validate format
	validFormats := map[string]bool{
		"json": true, "console": true,
	}
	if !validFormats[cfg.Format] {
		return fmt.Errorf("invalid format: %s", cfg.Format)
	}

	// Validate output
	if cfg.Output == "" {
		return fmt.Errorf("output cannot be empty")
	}

	return nil
}

// ===== MIGRATION HELPERS =====

// WrapExistingLogger wraps an existing simple logger to provide structured capabilities
func WrapExistingLogger(simpleLogger SimpleLogger) StructuredLogger {
	// This would wrap the existing logging.Logger interface
	// Implementation details depend on the existing interface
	return &simpleLoggerWrapper{logger: simpleLogger}
}

// SimpleLogger represents the existing simple logging interface
type SimpleLogger interface {
	Debugf(format string, args ...interface{})
	Infof(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
}

// simpleLoggerWrapper adapts a simple logger to the structured interface
type simpleLoggerWrapper struct {
	logger SimpleLogger
}

func (w *simpleLoggerWrapper) Debugf(format string, args ...interface{}) {
	w.logger.Debugf(format, args...)
}

func (w *simpleLoggerWrapper) Infof(format string, args ...interface{}) {
	w.logger.Infof(format, args...)
}

func (w *simpleLoggerWrapper) Warnf(format string, args ...interface{}) {
	w.logger.Warnf(format, args...)
}

func (w *simpleLoggerWrapper) Errorf(format string, args ...interface{}) {
	w.logger.Errorf(format, args...)
}

func (w *simpleLoggerWrapper) LogWithContext(ctx context.Context, level LogLevel, msg string, fields ...LogField) {
	// Convert to simple format for now
	fieldsStr := ""
	if len(fields) > 0 {
		fieldsStr = " " + formatFieldsForSimple(fields)
	}

	switch level {
	case DebugLevel:
		w.logger.Debugf("%s%s", msg, fieldsStr)
	case InfoLevel:
		w.logger.Infof("%s%s", msg, fieldsStr)
	case WarnLevel:
		w.logger.Warnf("%s%s", msg, fieldsStr)
	case ErrorLevel:
		w.logger.Errorf("%s%s", msg, fieldsStr)
	}
}

func (w *simpleLoggerWrapper) LogWithFields(level LogLevel, msg string, fields ...LogField) {
	w.LogWithContext(nil, level, msg, fields...)
}

func (w *simpleLoggerWrapper) WithFields(fields ...LogField) StructuredLogger {
	// For wrapped simple loggers, we can't really maintain context
	// Return the same wrapper - this is a limitation
	return w
}

func (w *simpleLoggerWrapper) WithError(err error) StructuredLogger {
	return w
}

func (w *simpleLoggerWrapper) WithWorker(workerID string) StructuredLogger {
	return w
}

func (w *simpleLoggerWrapper) WithContext(ctx context.Context) StructuredLogger {
	return w
}

// formatFieldsForSimple converts LogFields to a simple string representation
func formatFieldsForSimple(fields []LogField) string {
	result := "["
	for i, field := range fields {
		if i > 0 {
			result += " "
		}
		result += field.String()
	}
	result += "]"
	return result
}
