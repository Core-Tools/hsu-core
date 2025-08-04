package logcollection

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// ===== ZAP BACKEND ADAPTER =====

// ZapAdapter provides a Zap backend implementation that hides zap types from users
type ZapAdapter struct {
	logger *zap.Logger
	sugar  *zap.SugaredLogger
}

// NewZapAdapter creates a new Zap backend adapter
func NewZapAdapter(config ZapConfig) (*ZapAdapter, error) {
	zapLogger, err := createZapLogger(config)
	if err != nil {
		return nil, err
	}

	return &ZapAdapter{
		logger: zapLogger,
		sugar:  zapLogger.Sugar(),
	}, nil
}

// ===== STRUCTURED LOGGER IMPLEMENTATION =====

// Debugf implements simple logging interface
func (z *ZapAdapter) Debugf(format string, args ...interface{}) {
	z.sugar.Debugf(format, args...)
}

// Infof implements simple logging interface
func (z *ZapAdapter) Infof(format string, args ...interface{}) {
	z.sugar.Infof(format, args...)
}

// Warnf implements simple logging interface
func (z *ZapAdapter) Warnf(format string, args ...interface{}) {
	z.sugar.Warnf(format, args...)
}

// Errorf implements simple logging interface
func (z *ZapAdapter) Errorf(format string, args ...interface{}) {
	z.sugar.Errorf(format, args...)
}

// LogWithContext implements structured logging with context
func (z *ZapAdapter) LogWithContext(ctx context.Context, level LogLevel, msg string, fields ...LogField) {
	zapFields := z.convertFields(fields)

	// Add context fields if available
	if ctx != nil {
		if requestID := ctx.Value("request_id"); requestID != nil {
			zapFields = append(zapFields, zap.Any("request_id", requestID))
		}
		if userID := ctx.Value("user_id"); userID != nil {
			zapFields = append(zapFields, zap.Any("user_id", userID))
		}
	}

	z.logAtLevel(level, msg, zapFields...)
}

// LogWithFields implements structured logging
func (z *ZapAdapter) LogWithFields(level LogLevel, msg string, fields ...LogField) {
	zapFields := z.convertFields(fields)
	z.logAtLevel(level, msg, zapFields...)
}

// WithFields creates a new logger with additional fields
func (z *ZapAdapter) WithFields(fields ...LogField) StructuredLogger {
	zapFields := z.convertFields(fields)
	newLogger := z.logger.With(zapFields...)

	return &ZapAdapter{
		logger: newLogger,
		sugar:  newLogger.Sugar(),
	}
}

// WithError creates a new logger with an error field
func (z *ZapAdapter) WithError(err error) StructuredLogger {
	return z.WithFields(Error(err))
}

// WithWorker creates a new logger with a worker field
func (z *ZapAdapter) WithWorker(workerID string) StructuredLogger {
	return z.WithFields(Worker(workerID))
}

// WithContext creates a new logger with context fields
func (z *ZapAdapter) WithContext(ctx context.Context) StructuredLogger {
	if ctx == nil {
		return z
	}

	fields := make([]LogField, 0, 2)

	if requestID := ctx.Value("request_id"); requestID != nil {
		if id, ok := requestID.(string); ok {
			fields = append(fields, String("request_id", id))
		}
	}

	if userID := ctx.Value("user_id"); userID != nil {
		if id, ok := userID.(string); ok {
			fields = append(fields, String("user_id", id))
		}
	}

	if len(fields) == 0 {
		return z
	}

	return z.WithFields(fields...)
}

// ===== BACKEND INTERFACE IMPLEMENTATION =====

// LogWithLevel implements the LoggerBackend interface
func (z *ZapAdapter) LogWithLevel(level LogLevel, msg string, fields []LogField) {
	zapFields := z.convertFields(fields)
	z.logAtLevel(level, msg, zapFields...)
}

// SetLevel sets the minimum logging level
func (z *ZapAdapter) SetLevel(level LogLevel) {
	// Zap loggers are immutable, so we'd need to recreate
	// For now, this is a no-op - level is set during creation
}

// Sync flushes any buffered log entries
func (z *ZapAdapter) Sync() error {
	return z.logger.Sync()
}

// ===== INTERNAL CONVERSION METHODS =====

// convertFields converts our LogField types to zap.Field types (internal only)
func (z *ZapAdapter) convertFields(fields []LogField) []zap.Field {
	zapFields := make([]zap.Field, len(fields))

	for i, field := range fields {
		zapFields[i] = z.convertSingleField(field)
	}

	return zapFields
}

// convertSingleField converts a single LogField to zap.Field
func (z *ZapAdapter) convertSingleField(field LogField) zap.Field {
	switch field.Type {
	case StringField:
		return zap.String(field.Key, field.Value.(string))
	case IntField:
		return zap.Int(field.Key, field.Value.(int))
	case Int64Field:
		return zap.Int64(field.Key, field.Value.(int64))
	case Float64Field:
		return zap.Float64(field.Key, field.Value.(float64))
	case BoolField:
		return zap.Bool(field.Key, field.Value.(bool))
	case DurationField:
		return zap.Duration(field.Key, field.Value.(time.Duration))
	case TimeField:
		return zap.Time(field.Key, field.Value.(time.Time))
	case ErrorField:
		if err, ok := field.Value.(error); ok {
			return zap.Error(err)
		}
		return zap.String(field.Key, "invalid error field")
	case ObjectField:
		return zap.Any(field.Key, field.Value)
	case ArrayField:
		return zap.Any(field.Key, field.Value)
	default:
		// Fallback to Any for unknown types
		return zap.Any(field.Key, field.Value)
	}
}

// logAtLevel logs at the specified level
func (z *ZapAdapter) logAtLevel(level LogLevel, msg string, fields ...zap.Field) {
	switch level {
	case DebugLevel:
		z.logger.Debug(msg, fields...)
	case InfoLevel:
		z.logger.Info(msg, fields...)
	case WarnLevel:
		z.logger.Warn(msg, fields...)
	case ErrorLevel:
		z.logger.Error(msg, fields...)
	default:
		z.logger.Info(msg, fields...)
	}
}

// ===== ZAP CONFIGURATION =====

// ZapConfig defines Zap-specific configuration
type ZapConfig struct {
	Level      string `json:"level"`      // "debug", "info", "warn", "error"
	Format     string `json:"format"`     // "json", "console"
	Output     string `json:"output"`     // "stdout", "stderr", file path
	Caller     bool   `json:"caller"`     // Include caller information
	Stacktrace bool   `json:"stacktrace"` // Include stacktrace on errors
}

// createZapLogger creates a zap logger from configuration
func createZapLogger(config ZapConfig) (*zap.Logger, error) {
	// Parse level, in zap v1.27.0 use zapcore.ParseLevel(config.Level)
	level, err := getLevelFromString(config.Level)
	if err != nil {
		level = zapcore.InfoLevel
	}

	// Create encoder config
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "timestamp"
	encoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder
	encoderConfig.LevelKey = "level"
	encoderConfig.EncodeLevel = zapcore.LowercaseLevelEncoder

	// Create encoder
	var encoder zapcore.Encoder
	switch config.Format {
	case "console":
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	default: // "json" or anything else
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	}

	// Create writer sync
	var writeSyncer zapcore.WriteSyncer
	switch config.Output {
	case "stdout", "":
		writeSyncer = zapcore.AddSync(zapcore.Lock(zapcore.AddSync(zapcore.NewMultiWriteSyncer(zapcore.AddSync(os.Stdout)))))
	case "stderr":
		writeSyncer = zapcore.AddSync(zapcore.Lock(zapcore.AddSync(zapcore.NewMultiWriteSyncer(zapcore.AddSync(os.Stderr)))))
	default:
		// File output - for now just use stdout, file handling will come later
		writeSyncer = zapcore.AddSync(zapcore.Lock(zapcore.AddSync(zapcore.NewMultiWriteSyncer(zapcore.AddSync(os.Stdout)))))
	}

	// Create core
	core := zapcore.NewCore(encoder, writeSyncer, level)

	// Create logger options
	opts := []zap.Option{}
	if config.Caller {
		opts = append(opts, zap.AddCaller())
	}
	if config.Stacktrace {
		opts = append(opts, zap.AddStacktrace(zapcore.ErrorLevel))
	}

	// Create logger
	logger := zap.New(core, opts...)

	return logger, nil
}

// And older version (v1.20.0) of zapcore.ParseLevel(levelStr string) (v1.27.0)
func getLevelFromString(levelStr string) (zapcore.Level, error) {
	switch levelStr {
	case "debug":
		return zap.DebugLevel, nil
	case "info":
		return zap.InfoLevel, nil
	case "warn":
		return zap.WarnLevel, nil
	case "error":
		return zap.ErrorLevel, nil
	case "fatal":
		return zap.FatalLevel, nil
	case "dpanic":
		return zap.DPanicLevel, nil
	case "panic":
		return zap.PanicLevel, nil
	default:
		return -1, fmt.Errorf("invalid log level: %s", levelStr)
	}
}

// DefaultZapConfig returns a sensible default Zap configuration
func DefaultZapConfig() ZapConfig {
	return ZapConfig{
		Level:      "info",
		Format:     "json",
		Output:     "stdout",
		Caller:     true,
		Stacktrace: true,
	}
}
