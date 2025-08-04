//go:build test

package logcollection

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/core-tools/hsu-core/pkg/logcollection/config"
)

// ExampleUsage demonstrates the complete Phase 1 functionality
func ExampleUsage() {
	// ===== 1. CREATE LOGGER WITH COMPLETE BACKEND HIDING =====

	// Users never import zap, logrus, or any backend!
	logger, err := NewStructuredLogger("zap", InfoLevel)
	if err != nil {
		panic(err)
	}

	// ===== 2. USE CLEAN API WITH OUR FIELD TYPES =====

	// Simple logging (backwards compatible)
	logger.Infof("Master starting with %d workers", 3)

	// Structured logging with OUR types - no backend exposure!
	logger.LogWithFields(InfoLevel, "Worker operation completed",
		Worker("web-server"),                // Our field constructor
		String("operation", "restart"),      // Our field constructor
		Duration("duration", 2*time.Second), // Our field constructor
		Int("attempt", 3),                   // Our field constructor
	)

	// Fluent interface
	workerLogger := logger.
		WithWorker("database-service").
		WithFields(Component("health-check"))

	workerLogger.LogWithFields(WarnLevel, "Health check failed",
		Error(fmt.Errorf("connection timeout")), // Our field constructor
		Int("retry_count", 5),
	)

	// ===== 3. CREATE LOG COLLECTION SERVICE =====

	cfg := config.DefaultLogCollectionConfig()
	service := NewLogCollectionService(cfg, logger)

	ctx := context.Background()
	if err := service.Start(ctx); err != nil {
		panic(err)
	}
	defer service.Stop()

	// ===== 4. REGISTER WORKER FOR LOG COLLECTION =====

	workerConfig := config.DefaultWorkerLogConfig()
	if err := service.RegisterWorker("web-server", workerConfig); err != nil {
		panic(err)
	}

	// ===== 5. SIMULATE PROCESS LOG COLLECTION =====

	// Simulate stdout stream from a process
	stdout := strings.NewReader(`2025-01-20 10:30:45 INFO: Database connection established
2025-01-20 10:30:46 INFO: Server listening on port 8080
2025-01-20 10:30:47 WARN: High memory usage detected
2025-01-20 10:30:48 ERROR: Failed to process request`)

	// Simulate stderr stream
	stderr := strings.NewReader(`2025-01-20 10:30:49 ERROR: Database connection lost
2025-01-20 10:30:50 ERROR: Retrying connection...`)

	// Collect logs from both streams (Phase 1 - Process Control Integration)
	if err := service.CollectFromProcess("web-server", stdout, stderr); err != nil {
		panic(err)
	}

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// ===== 6. CHECK STATUS =====

	workerStatus, err := service.GetWorkerStatus("web-server")
	if err != nil {
		panic(err)
	}

	fmt.Printf("Worker processed %d lines, %d bytes\n",
		workerStatus.LinesProcessed, workerStatus.BytesProcessed)

	systemStatus := service.GetSystemStatus()
	fmt.Printf("System: %d active workers, %d total lines processed\n",
		systemStatus.WorkersActive, systemStatus.TotalLines)
}

// TestPhase1Integration demonstrates integration with process control
func TestPhase1Integration(t *testing.T) {
	// Create logger with development settings
	logger, err := DevelopmentLogger()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Log with structured fields - completely backend-agnostic
	logger.LogWithFields(InfoLevel, "Starting Phase 1 integration test",
		String("phase", "1"),
		String("test_name", "integration"),
		Bool("process_control", true),
	)

	// Create service with minimal config
	cfg := config.LogCollectionConfig{
		Enabled: true,
		GlobalAggregation: config.GlobalAggregationConfig{
			Enabled: true,
			Targets: []config.OutputTargetConfig{
				{
					Type:   "master_stdout",
					Format: "enhanced_plain",
				},
			},
		},
		DefaultWorker: config.DefaultWorkerLogConfig(),
		System: config.SystemConfig{
			MaxWorkers:    10,
			FlushInterval: 1 * time.Second,
		},
	}

	service := NewLogCollectionService(cfg, logger)

	ctx := context.Background()
	if err := service.Start(ctx); err != nil {
		t.Fatalf("Failed to start service: %v", err)
	}
	defer service.Stop()

	// Register worker
	workerConfig := config.WorkerLogConfig{
		Enabled:       true,
		CaptureStdout: true,
		CaptureStderr: true,
		Buffering: config.BufferingConfig{
			Size:          "1MB",
			FlushInterval: 100 * time.Millisecond,
		},
		Processing: config.ProcessingConfig{
			ParseStructured: true,
			AddMetadata:     true,
		},
	}

	if err := service.RegisterWorker("test-worker", workerConfig); err != nil {
		t.Fatalf("Failed to register worker: %v", err)
	}

	// Simulate log collection from process streams
	testLogs := `{"timestamp":"2025-01-20T10:30:45Z","level":"info","message":"Application started"}
Plain text log line without structure
{"timestamp":"2025-01-20T10:30:46Z","level":"error","message":"Database error","error":"connection refused"}
Another plain text line with INFO: prefix
`

	// Test single stream collection
	reader := strings.NewReader(testLogs)
	if err := service.CollectFromStream("test-worker", reader, StdoutStream); err != nil {
		t.Fatalf("Failed to collect from stream: %v", err)
	}

	// Allow processing time
	time.Sleep(200 * time.Millisecond)

	// Verify status
	status, err := service.GetWorkerStatus("test-worker")
	if err != nil {
		t.Fatalf("Failed to get worker status: %v", err)
	}

	if status.LinesProcessed == 0 {
		t.Error("Expected processed lines > 0")
	}

	if status.BytesProcessed == 0 {
		t.Error("Expected processed bytes > 0")
	}

	logger.LogWithFields(InfoLevel, "Phase 1 integration test completed successfully",
		Int64("lines_processed", status.LinesProcessed),
		Int64("bytes_processed", status.BytesProcessed),
		Bool("success", true),
	)
}

// BenchmarkLogCollection tests performance of our log collection
func BenchmarkLogCollection(b *testing.B) {
	logger := QuickLogger(InfoLevel)
	cfg := config.DefaultLogCollectionConfig()
	service := NewLogCollectionService(cfg, logger)

	ctx := context.Background()
	service.Start(ctx)
	defer service.Stop()

	service.RegisterWorker("bench-worker", config.DefaultWorkerLogConfig())

	logLine := "2025-01-20 10:30:45 INFO: This is a test log line for benchmarking\n"

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			reader := strings.NewReader(logLine)
			service.CollectFromStream("bench-worker", reader, StdoutStream)
		}
	})
}

// TestBackendAbstraction verifies that users never see backend types
func TestBackendAbstraction(t *testing.T) {
	// This test verifies that our abstraction is complete
	// Users can work with logs without any knowledge of zap, logrus, etc.

	// Create logger - user doesn't know it's using zap internally
	logger, err := NewStructuredLogger("zap", DebugLevel)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Use only OUR field types - no backend imports needed
	fields := []LogField{
		String("user_id", "12345"),
		Duration("request_duration", 150*time.Millisecond),
		Bool("success", true),
		Int("status_code", 200),
	}

	// Log with context
	ctx := context.WithValue(context.Background(), "request_id", "req-67890")
	logger.LogWithContext(ctx, InfoLevel, "Request processed", fields...)

	// Chain with fluent interface
	enrichedLogger := logger.
		WithFields(Component("api")).
		WithFields(String("version", "1.0.0"))

	enrichedLogger.LogWithFields(DebugLevel, "Debug information logged",
		Object("metadata", map[string]interface{}{
			"source": "test",
			"type":   "backend_abstraction",
		}),
	)

	// Test error handling
	testErr := fmt.Errorf("test error for abstraction verification")
	logger.WithError(testErr).Errorf("Error occurred during test")

	// SUCCESS: If this compiles and runs, our abstraction is working!
}
