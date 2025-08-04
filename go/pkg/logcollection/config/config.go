package config

import (
	"fmt"
	"time"
)

// ===== MAIN CONFIGURATION =====

// LogCollectionConfig defines the overall log collection system configuration
type LogCollectionConfig struct {
	Enabled           bool                    `yaml:"enabled"`
	GlobalAggregation GlobalAggregationConfig `yaml:"global_aggregation"`
	Enhancement       EnhancementConfig       `yaml:"enhancement"`
	DefaultWorker     WorkerLogConfig         `yaml:"default_worker"`
	System            SystemConfig            `yaml:"system"`
}

// GlobalAggregationConfig defines system-wide log aggregation settings
type GlobalAggregationConfig struct {
	Enabled bool                 `yaml:"enabled"`
	Targets []OutputTargetConfig `yaml:"targets"`
}

// ===== WORKER CONFIGURATION =====

// WorkerLogConfig defines log collection settings for individual workers
type WorkerLogConfig struct {
	Enabled       bool             `yaml:"enabled"`
	CaptureStdout bool             `yaml:"capture_stdout"`
	CaptureStderr bool             `yaml:"capture_stderr"`
	Buffering     BufferingConfig  `yaml:"buffering"`
	Processing    ProcessingConfig `yaml:"processing"`
	Outputs       OutputConfig     `yaml:"outputs"`
}

// ===== BUFFERING CONFIGURATION =====

// BufferingConfig controls log buffering behavior
type BufferingConfig struct {
	Size          string        `yaml:"size"`           // e.g., "1MB", "512KB"
	FlushInterval time.Duration `yaml:"flush_interval"` // e.g., "5s", "1m"
	MaxLines      int           `yaml:"max_lines"`      // Max lines to buffer
}

// ===== PROCESSING CONFIGURATION =====

// ProcessingConfig defines how logs should be processed
type ProcessingConfig struct {
	ParseStructured bool           `yaml:"parse_structured"`
	AddMetadata     bool           `yaml:"add_metadata"`
	Filters         FilterConfig   `yaml:"filters"`
	Parsers         []ParserConfig `yaml:"parsers"`
}

// FilterConfig defines log filtering rules
type FilterConfig struct {
	ExcludePatterns []string `yaml:"exclude_patterns"`
	IncludePatterns []string `yaml:"include_patterns"`
	LevelFilter     string   `yaml:"level_filter"` // minimum level: "debug", "info", "warn", "error"
}

// ParserConfig defines log parsing configuration
type ParserConfig struct {
	Type    string                 `yaml:"type"`   // "json", "logfmt", "timestamp_extraction"
	Config  map[string]interface{} `yaml:"config"` // Parser-specific config
	Enabled bool                   `yaml:"enabled"`
}

// ===== OUTPUT CONFIGURATION =====

// OutputConfig defines where logs should be sent
type OutputConfig struct {
	Separate   SeparateOutputConfig `yaml:"separate"`
	Forwarding []OutputTargetConfig `yaml:"forwarding"`
}

// SeparateOutputConfig defines separate stream outputs
type SeparateOutputConfig struct {
	Stdout []OutputTargetConfig `yaml:"stdout"`
	Stderr []OutputTargetConfig `yaml:"stderr"`
}

// OutputTargetConfig defines a specific output destination
type OutputTargetConfig struct {
	Type           string                 `yaml:"type"`             // "file", "stdout", "stderr", "syslog", "elasticsearch"
	Path           string                 `yaml:"path,omitempty"`   // For file targets
	Format         string                 `yaml:"format"`           // "plain", "enhanced_plain", "json", "structured_json"
	Prefix         string                 `yaml:"prefix,omitempty"` // Log line prefix
	Rotation       RotationConfig         `yaml:"rotation,omitempty"`
	Authentication AuthenticationConfig   `yaml:"authentication,omitempty"`
	Config         map[string]interface{} `yaml:"config,omitempty"` // Target-specific config
}

// RotationConfig defines log rotation settings
type RotationConfig struct {
	MaxSize  string        `yaml:"max_size"`  // e.g., "100MB"
	MaxFiles int           `yaml:"max_files"` // Number of files to keep
	MaxAge   time.Duration `yaml:"max_age"`   // e.g., "7d", "30d"
}

// AuthenticationConfig defines authentication for external targets
type AuthenticationConfig struct {
	Type     string `yaml:"type"` // "basic", "token", "certificate"
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`
	Token    string `yaml:"token,omitempty"`
	CertFile string `yaml:"cert_file,omitempty"`
	KeyFile  string `yaml:"key_file,omitempty"`
}

// ===== ENHANCEMENT CONFIGURATION =====

// EnhancementConfig defines log enhancement settings
type EnhancementConfig struct {
	Enabled   bool             `yaml:"enabled"`
	Parsers   []ParserConfig   `yaml:"parsers"`
	Metadata  MetadataConfig   `yaml:"metadata"`
	Enrichers []EnricherConfig `yaml:"enrichers"`
}

// MetadataConfig defines what metadata to add to logs
type MetadataConfig struct {
	AddMasterID   bool `yaml:"add_master_id"`
	AddHostname   bool `yaml:"add_hostname"`
	AddTimestamp  bool `yaml:"add_timestamp"`
	AddSequence   bool `yaml:"add_sequence"`    // Sequential log numbering
	AddLineNumber bool `yaml:"add_line_number"` // Line number within worker stream
}

// EnricherConfig defines log enrichment rules
type EnricherConfig struct {
	Type    string                 `yaml:"type"` // "regex", "json_extract", "geo_ip"
	Config  map[string]interface{} `yaml:"config"`
	Enabled bool                   `yaml:"enabled"`
}

// ===== SYSTEM CONFIGURATION =====

// SystemConfig defines system-level log collection settings
type SystemConfig struct {
	WorkerDirectory string        `yaml:"worker_directory"` // Base directory for worker log files
	BufferSize      string        `yaml:"buffer_size"`      // Global buffer size
	FlushInterval   time.Duration `yaml:"flush_interval"`   // Global flush interval
	MaxWorkers      int           `yaml:"max_workers"`      // Maximum concurrent workers
	Metrics         MetricsConfig `yaml:"metrics"`
}

// MetricsConfig defines metrics collection for log system
type MetricsConfig struct {
	Enabled  bool          `yaml:"enabled"`
	Interval time.Duration `yaml:"interval"`
	Targets  []string      `yaml:"targets"` // Where to send metrics
}

// ===== BACKEND CONFIGURATION =====

// LoggerBackendConfig defines the underlying logging backend
type LoggerBackendConfig struct {
	Backend string `yaml:"backend"` // "zap", "logrus", "slog"
	Level   string `yaml:"level"`   // "debug", "info", "warn", "error"
	Format  string `yaml:"format"`  // "json", "console"
	Output  string `yaml:"output"`  // "stdout", "stderr", file path
}

// ===== AGGREGATION MODES =====

// AggregationMode defines how logs should be aggregated
type AggregationMode string

const (
	AggregationModeDisabled  AggregationMode = "disabled"
	AggregationModeSeparate  AggregationMode = "separate"
	AggregationModeAggregate AggregationMode = "aggregate"
	AggregationModeBoth      AggregationMode = "both"
)

// ===== VALIDATION =====

// Validate checks if the configuration is valid
func (c *LogCollectionConfig) Validate() error {
	if !c.Enabled {
		return nil // No validation needed if disabled
	}

	// Validate global aggregation
	if c.GlobalAggregation.Enabled {
		if len(c.GlobalAggregation.Targets) == 0 {
			return fmt.Errorf("global aggregation enabled but no targets configured")
		}

		for i, target := range c.GlobalAggregation.Targets {
			if err := target.Validate(); err != nil {
				return fmt.Errorf("global aggregation target %d: %w", i, err)
			}
		}
	}

	// Validate default worker config
	if err := c.DefaultWorker.Validate(); err != nil {
		return fmt.Errorf("default worker config: %w", err)
	}

	// Validate system config
	if err := c.System.Validate(); err != nil {
		return fmt.Errorf("system config: %w", err)
	}

	return nil
}

// Validate checks if the worker configuration is valid
func (w *WorkerLogConfig) Validate() error {
	if !w.Enabled {
		return nil
	}

	if !w.CaptureStdout && !w.CaptureStderr {
		return fmt.Errorf("at least one of capture_stdout or capture_stderr must be enabled")
	}

	// Validate output targets
	allTargets := append(w.Outputs.Separate.Stdout, w.Outputs.Separate.Stderr...)
	allTargets = append(allTargets, w.Outputs.Forwarding...)

	for i, target := range allTargets {
		if err := target.Validate(); err != nil {
			return fmt.Errorf("output target %d: %w", i, err)
		}
	}

	return nil
}

// Validate checks if the output target configuration is valid
func (o *OutputTargetConfig) Validate() error {
	if o.Type == "" {
		return fmt.Errorf("output target type cannot be empty")
	}

	validTypes := map[string]bool{
		"file": true, "stdout": true, "stderr": true,
		"syslog": true, "elasticsearch": true, "master_stdout": true,
	}

	if !validTypes[o.Type] {
		return fmt.Errorf("invalid output target type: %s", o.Type)
	}

	if o.Type == "file" && o.Path == "" {
		return fmt.Errorf("file output target must have path specified")
	}

	return nil
}

// Validate checks if the system configuration is valid
func (s *SystemConfig) Validate() error {
	if s.MaxWorkers < 0 {
		return fmt.Errorf("max_workers cannot be negative")
	}

	if s.FlushInterval < 0 {
		return fmt.Errorf("flush_interval cannot be negative")
	}

	return nil
}

// ===== DEFAULT CONFIGURATIONS =====

// DefaultLogCollectionConfig returns a sensible default configuration
func DefaultLogCollectionConfig() LogCollectionConfig {
	return LogCollectionConfig{
		Enabled: true,
		GlobalAggregation: GlobalAggregationConfig{
			Enabled: true,
			Targets: []OutputTargetConfig{
				{
					Type:   "file",
					Path:   "aggregated.log", // Relative template - will be resolved at runtime
					Format: "enhanced_plain",
				},
			},
		},
		Enhancement: EnhancementConfig{
			Enabled: true,
			Metadata: MetadataConfig{
				AddMasterID:   true,
				AddHostname:   true,
				AddTimestamp:  true,
				AddSequence:   true,
				AddLineNumber: true,
			},
		},
		DefaultWorker: DefaultWorkerLogConfig(),
		System: SystemConfig{
			WorkerDirectory: "workers", // Relative template - will be resolved at runtime
			BufferSize:      "1MB",
			FlushInterval:   5 * time.Second,
			MaxWorkers:      100,
			Metrics: MetricsConfig{
				Enabled:  true,
				Interval: 30 * time.Second,
			},
		},
	}
}

// DefaultWorkerLogConfig returns a sensible default worker configuration
func DefaultWorkerLogConfig() WorkerLogConfig {
	return WorkerLogConfig{
		Enabled:       true,
		CaptureStdout: true,
		CaptureStderr: true,
		Buffering: BufferingConfig{
			Size:          "512KB",
			FlushInterval: 5 * time.Second,
			MaxLines:      1000,
		},
		Processing: ProcessingConfig{
			ParseStructured: true,
			AddMetadata:     true,
			Filters: FilterConfig{
				LevelFilter: "debug", // Capture all levels by default
			},
		},
		Outputs: OutputConfig{
			Separate: SeparateOutputConfig{
				Stdout: []OutputTargetConfig{
					{
						Type:   "file",
						Path:   "{worker_id}-stdout.log", // Relative template - will be resolved at runtime
						Format: "enhanced_plain",
						Rotation: RotationConfig{
							MaxSize:  "100MB",
							MaxFiles: 10,
							MaxAge:   7 * 24 * time.Hour, // 7 days
						},
					},
				},
				Stderr: []OutputTargetConfig{
					{
						Type:   "file",
						Path:   "{worker_id}-stderr.log", // Relative template - will be resolved at runtime
						Format: "enhanced_plain",
						Rotation: RotationConfig{
							MaxSize:  "100MB",
							MaxFiles: 10,
							MaxAge:   7 * 24 * time.Hour, // 7 days
						},
					},
				},
			},
		},
	}
}
