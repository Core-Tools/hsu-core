package resourcelimits

import (
	"context"
	"time"
)

// Callback function for handling resource violations
type ResourceViolationCallback func(policy ResourcePolicy, violation *ResourceViolation)

// ResourceLimitManager provides a unified interface for managing resource limits
type ResourceLimitManager interface {
	// Start begins resource limit management
	Start(ctx context.Context) error

	// Stop stops resource limit management
	Stop()

	// GetViolations returns recent resource violations
	GetViolations() []*ResourceViolation

	// SetViolationCallback sets callback for limit violations
	SetViolationCallback(callback ResourceViolationCallback)
}

// ResourcePolicy defines what action to take when limits are violated
type ResourcePolicy string

const (
	ResourcePolicyNone             ResourcePolicy = "none"              // No action
	ResourcePolicyLog              ResourcePolicy = "log"               // Log violation only
	ResourcePolicyAlert            ResourcePolicy = "alert"             // Send alert/notification
	ResourcePolicyThrottle         ResourcePolicy = "throttle"          // Suspend/resume process
	ResourcePolicyGracefulShutdown ResourcePolicy = "graceful_shutdown" // SIGTERM then SIGKILL
	ResourcePolicyImmediateKill    ResourcePolicy = "immediate_kill"    // SIGKILL immediately
	ResourcePolicyRestart          ResourcePolicy = "restart"           // Restart with same/adjusted limits
	ResourcePolicyRestartAdjusted  ResourcePolicy = "restart_adjusted"  // Restart with increased limits
)

// ResourceLimitType represents different types of resource limits
type ResourceLimitType string

const (
	ResourceLimitTypeMemory  ResourceLimitType = "memory"
	ResourceLimitTypeCPU     ResourceLimitType = "cpu"
	ResourceLimitTypeIO      ResourceLimitType = "io"
	ResourceLimitTypeNetwork ResourceLimitType = "network"
	ResourceLimitTypeProcess ResourceLimitType = "process"
)

// ResourceViolation represents a resource limit violation
type ResourceViolation struct {
	LimitType    ResourceLimitType `json:"limit_type"`
	CurrentValue interface{}       `json:"current_value"`
	LimitValue   interface{}       `json:"limit_value"`
	Severity     ViolationSeverity `json:"severity"`
	Timestamp    time.Time         `json:"timestamp"`
	Message      string            `json:"message"`
}

// ViolationSeverity indicates how severe a resource violation is
type ViolationSeverity string

const (
	ViolationSeverityWarning  ViolationSeverity = "warning"
	ViolationSeverityCritical ViolationSeverity = "critical"
)

// Callback function for handling resource usage updates
type ResourceUsageCallback func(usage *ResourceUsage)

// ResourceMonitor provides real-time resource usage monitoring
type ResourceMonitor interface {
	// Start begins resource monitoring
	Start(ctx context.Context) error

	// Stop stops resource monitoring
	Stop()

	// GetCurrentUsage returns current resource usage
	GetCurrentUsage() (*ResourceUsage, error)

	// GetUsageHistory returns historical usage data
	GetUsageHistory(since time.Time) []*ResourceUsage

	// SetUsageCallback sets callback for resource usage updates
	SetUsageCallback(callback ResourceUsageCallback)
}

// ResourceUsage represents current resource usage
type ResourceUsage struct {
	Timestamp time.Time `json:"timestamp"`

	// Memory usage
	MemoryRSS     int64   `json:"memory_rss"`     // Resident Set Size
	MemoryVirtual int64   `json:"memory_virtual"` // Virtual memory
	MemoryPercent float64 `json:"memory_percent"` // % of system memory

	// CPU usage
	CPUPercent float64 `json:"cpu_percent"` // % CPU usage
	CPUTime    float64 `json:"cpu_time"`    // Total CPU time in seconds

	// I/O usage
	IOReadBytes  int64 `json:"io_read_bytes"`  // Bytes read
	IOWriteBytes int64 `json:"io_write_bytes"` // Bytes written
	IOReadOps    int64 `json:"io_read_ops"`    // Read operations
	IOWriteOps   int64 `json:"io_write_ops"`   // Write operations

	// Process/File descriptor usage
	OpenFileDescriptors int `json:"open_file_descriptors"`
	ChildProcesses      int `json:"child_processes"`

	// Network usage (if available)
	NetworkBytesReceived int64 `json:"network_bytes_received"`
	NetworkBytesSent     int64 `json:"network_bytes_sent"`
}

// ResourceEnforcer applies and enforces resource limits
type ResourceEnforcer interface {
	// ApplyLimits applies resource limits to a process
	ApplyLimits(pid int, limits *ResourceLimits) error

	// SupportsLimitType checks if a limit type is supported on current platform
	SupportsLimitType(limitType ResourceLimitType) bool
}

// ResourceViolationsChecker checks if resource limits are violated
type ResourceViolationChecker interface {
	// CheckViolations checks if resource limits are violated
	CheckViolations(usage *ResourceUsage, limits *ResourceLimits) []*ResourceViolation
}

// ResourceLimits provides comprehensive resource limits with policies and monitoring
type ResourceLimits struct {
	// Process priority (standalone field)
	Priority int `yaml:"priority,omitempty"` // Process priority

	// CPU shares for Linux cgroups (standalone field)
	CPUShares int `yaml:"cpu_shares,omitempty"` // CPU weight (Linux cgroups)

	// Advanced limits with policies and monitoring
	Memory     *MemoryLimits             `yaml:"memory,omitempty"`
	CPU        *CPULimits                `yaml:"cpu,omitempty"`
	IO         *IOLimits                 `yaml:"io,omitempty"`
	Process    *ProcessLimits            `yaml:"process,omitempty"`
	Monitoring *ResourceMonitoringConfig `yaml:"monitoring,omitempty"`
}

// MemoryLimits defines memory-specific limits and policies (consolidated)
type MemoryLimits struct {
	// Basic memory limits (moved from top level)
	MaxRSS     int64 `yaml:"max_rss,omitempty"`     // Max RSS in bytes (replaces old Memory field)
	MaxVirtual int64 `yaml:"max_virtual,omitempty"` // Max virtual memory
	MaxSwap    int64 `yaml:"max_swap,omitempty"`    // Max memory + swap (replaces MemorySwap)

	// Policy and monitoring
	WarningThreshold float64        `yaml:"warning_threshold,omitempty"` // Warning threshold (0-100%)
	Policy           ResourcePolicy `yaml:"policy,omitempty"`            // Action to take
}

// CPULimits defines CPU-specific limits and policies (consolidated)
type CPULimits struct {
	// Basic CPU limits (moved from top level)
	MaxCores   float64       `yaml:"max_cores,omitempty"`   // Number of CPU cores (replaces old CPU field)
	MaxPercent float64       `yaml:"max_percent,omitempty"` // Max CPU percentage
	MaxTime    time.Duration `yaml:"max_time,omitempty"`    // Max total CPU time

	// Policy and monitoring
	WarningThreshold float64        `yaml:"warning_threshold,omitempty"` // Warning threshold (0-100%)
	Policy           ResourcePolicy `yaml:"policy,omitempty"`            // Action to take
}

// IOLimits defines I/O-specific limits and policies (consolidated)
type IOLimits struct {
	// Basic I/O limits (moved from top level)
	Weight      int   `yaml:"weight,omitempty"`        // I/O priority weight (replaces IOWeight)
	MaxReadBPS  int64 `yaml:"max_read_bps,omitempty"`  // Read bandwidth limit (replaces IOReadBPS)
	MaxWriteBPS int64 `yaml:"max_write_bps,omitempty"` // Write bandwidth limit (replaces IOWriteBPS)

	// Advanced I/O limits
	MaxReadOps       int64          `yaml:"max_read_ops,omitempty"`      // Operations per second
	MaxWriteOps      int64          `yaml:"max_write_ops,omitempty"`     // Operations per second
	WarningThreshold float64        `yaml:"warning_threshold,omitempty"` // Warning threshold (0-100%)
	Policy           ResourcePolicy `yaml:"policy,omitempty"`            // Action to take
}

// ProcessLimits defines process/file descriptor limits and policies (consolidated)
type ProcessLimits struct {
	// Basic process limits (moved from top level)
	MaxProcesses       int `yaml:"max_processes,omitempty"`        // Maximum number of processes (replaces MaxProcesses)
	MaxFileDescriptors int `yaml:"max_file_descriptors,omitempty"` // Max open FDs (replaces MaxOpenFiles)
	MaxChildProcesses  int `yaml:"max_child_processes,omitempty"`  // Max child processes

	// Policy and monitoring
	WarningThreshold float64        `yaml:"warning_threshold,omitempty"` // Warning threshold (0-100%)
	Policy           ResourcePolicy `yaml:"policy,omitempty"`            // Action to take
}

// ResourceMonitoringConfig defines monitoring behavior
type ResourceMonitoringConfig struct {
	Enabled          bool          `yaml:"enabled"`                     // Enable monitoring
	Interval         time.Duration `yaml:"interval,omitempty"`          // Monitoring interval
	HistoryRetention time.Duration `yaml:"history_retention,omitempty"` // How long to keep history
	AlertingEnabled  bool          `yaml:"alerting_enabled,omitempty"`  // Enable alerting
}
