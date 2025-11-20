package logcollection

import (
	"context"
	"io"
	"time"

	"github.com/core-tools/hsu-procman-go/pkg/logcollection/config"
)

// ===== CORE LOG COLLECTION INTERFACES =====

// StructuredLogger provides clean logging interface with complete backend hiding
type StructuredLogger interface {
	// Simple logging (backwards compatible with existing logging.Logger)
	Debugf(format string, args ...interface{})
	Infof(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Errorf(format string, args ...interface{})

	// Structured logging with our own types (no backend exposure)
	LogWithContext(ctx context.Context, level LogLevel, msg string, fields ...LogField)
	LogWithFields(level LogLevel, msg string, fields ...LogField)

	// Fluent interface for building context
	WithFields(fields ...LogField) StructuredLogger
	WithError(err error) StructuredLogger
	WithProcess(processID string) StructuredLogger
	WithContext(ctx context.Context) StructuredLogger
}

// LogCollector handles real-time log collection from managed processes
type LogCollector interface {
	// Stream collection (Phase 1 - Process Control Integration)
	CollectFromStream(processID string, stream io.Reader, streamType StreamType) error
	CollectFromProcess(processID string, stdout, stderr io.Reader) error

	// Log processing
	ProcessLogLine(processID string, line string, metadata LogMetadata) error

	// Output management
	ForwardLogs(targets []LogOutputTarget) error

	// Service lifecycle
	Start(ctx context.Context) error
	Stop() error
}

// LogCollectionService coordinates all log collection activities
type LogCollectionService interface {
	LogCollector

	// Managed process management
	RegisterProcess(processID string, processConfig config.ProcessLogConfig) error
	UnregisterProcess(processID string) error

	// Configuration management
	UpdateConfiguration(config config.LogCollectionConfig) error
	GetConfiguration() config.LogCollectionConfig

	// Status and metrics
	GetProcessStatus(processID string) (*ProcessLogStatus, error)
	GetSystemStatus() *SystemLogStatus
}

// ===== BACKEND ABSTRACTION =====

// LoggerBackend provides internal interface for different logging backends
type LoggerBackend interface {
	// Internal interface - users never see this
	LogWithLevel(level LogLevel, msg string, fields []LogField)
	SetLevel(level LogLevel)
	Sync() error
}

// ===== OUTPUT INTERFACES =====

// LogOutputWriter handles writing logs to various targets
type LogOutputWriter interface {
	Write(entry LogEntry) error
	Flush() error
	Close() error
}

// LogOutputTarget represents a destination for log output
type LogOutputTarget interface {
	GetType() string
	GetConfig() map[string]interface{}
	CreateWriter() (LogOutputWriter, error)
}

// ===== PROCESSING INTERFACES =====

// LogParser attempts to parse structured content from log lines
type LogParser interface {
	Parse(line string) (*StructuredLogEntry, error)
	CanParse(line string) bool
}

// LogEnhancer adds metadata and context to log entries
type LogEnhancer interface {
	Enhance(entry *EnhancedLogEntry) error
}

// LogFilter determines whether log entries should be processed
type LogFilter interface {
	ShouldProcess(entry EnhancedLogEntry) bool
}

// ===== CORE TYPES =====

// LogLevel represents logging levels
type LogLevel int

const (
	DebugLevel LogLevel = iota
	InfoLevel
	WarnLevel
	ErrorLevel
)

func (l LogLevel) String() string {
	switch l {
	case DebugLevel:
		return "debug"
	case InfoLevel:
		return "info"
	case WarnLevel:
		return "warn"
	case ErrorLevel:
		return "error"
	default:
		return "unknown"
	}
}

// StreamType identifies the source stream
type StreamType string

const (
	StdoutStream StreamType = "stdout"
	StderrStream StreamType = "stderr"
)

// LogEntry represents a processed log entry ready for output
type LogEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Level     string                 `json:"level,omitempty"`
	Message   string                 `json:"message"`
	ProcessID string                 `json:"process_id"`
	Stream    StreamType             `json:"stream"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
	Raw       string                 `json:"raw_line"`
	Enhanced  map[string]interface{} `json:"enhanced,omitempty"`
}

// LogMetadata contains contextual information about a log entry
type LogMetadata struct {
	Timestamp time.Time
	ProcessID string
	Stream    StreamType
	LineNum   int64
}

// RawLogEntry represents an unprocessed log line from a process
type RawLogEntry struct {
	ProcessID string
	Stream    StreamType
	Line      string
	Timestamp time.Time
}

// StructuredLogEntry represents parsed structured log content
type StructuredLogEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Fields    map[string]interface{} `json:"fields"`
}

// EnhancedLogEntry combines raw and structured data with enhancements
type EnhancedLogEntry struct {
	Raw        RawLogEntry
	Structured *StructuredLogEntry    // nil if parsing failed
	Enhanced   map[string]interface{} // Additional metadata
}

// ===== STATUS TYPES =====

// ProcessLogStatus provides status information for a specific process
type ProcessLogStatus struct {
	ProcessID      string                  `json:"process_id"`
	Active         bool                    `json:"active"`
	LinesProcessed int64                   `json:"lines_processed"`
	BytesProcessed int64                   `json:"bytes_processed"`
	LastActivity   time.Time               `json:"last_activity"`
	Errors         []string                `json:"errors,omitempty"`
	Config         config.ProcessLogConfig `json:"config"`
}

// SystemLogStatus provides overall log collection system status
type SystemLogStatus struct {
	Active           bool                         `json:"active"`
	ProcessesActive  int                          `json:"processes_active"`
	TotalProcesses   int                          `json:"total_processes"`
	TotalLines       int64                        `json:"total_lines_processed"`
	TotalBytes       int64                        `json:"total_bytes_processed"`
	StartTime        time.Time                    `json:"start_time"`
	LastActivity     time.Time                    `json:"last_activity"`
	ManagedProcesses map[string]*ProcessLogStatus `json:"managed_processes"`
	OutputTargets    []string                     `json:"output_targets"`
}
