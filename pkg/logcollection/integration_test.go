//go:build test

package logcollection

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/core-tools/hsu-core/pkg/logcollection/config"
)

// TestLogCollectionIntegration demonstrates the complete log collection system
func TestLogCollectionIntegration(t *testing.T) {
	fmt.Println("\nProcess Manager Log Collection Integration Test Starting...")

	// 1. Create structured logger
	logger, err := NewStructuredLogger("zap", InfoLevel)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// 2. Create log collection service
	cfg := config.DefaultLogCollectionConfig()
	service := NewLogCollectionService(cfg, logger)

	ctx := context.Background()
	if err := service.Start(ctx); err != nil {
		t.Fatalf("Failed to start service: %v", err)
	}
	defer service.Stop()

	// 3. Register a process for log collection
	processConfig := config.DefaultProcessLogConfig()
	processID := "integration-test-process"

	if err := service.RegisterProcess(processID, processConfig); err != nil {
		t.Fatalf("Failed to register process: %v", err)
	}

	// 4. Simulate a process that generates logs
	process, stdout, err := createTestProcess(ctx)
	if err != nil {
		t.Fatalf("Failed to create test process: %v", err)
	}
	defer process.Kill()

	// 5. Start log collection from the process
	fmt.Println("Starting log collection from test process...")
	if err := service.CollectFromStream(processID, stdout, StdoutStream); err != nil {
		t.Fatalf("Failed to start log collection: %v", err)
	}

	// 6. Let it collect logs for a few seconds
	fmt.Println("Collecting logs for 3 seconds...")
	time.Sleep(3 * time.Second)

	// 7. Check status
	status, err := service.GetProcessStatus(processID)
	if err != nil {
		t.Fatalf("Failed to get process status: %v", err)
	}

	fmt.Printf("\nLog Collection Results:\n")
	fmt.Printf("   - Lines Processed: %d\n", status.LinesProcessed)
	fmt.Printf("   - Bytes Processed: %d\n", status.BytesProcessed)
	fmt.Printf("   - Active: %t\n", status.Active)

	// Verify we collected some logs
	if status.LinesProcessed == 0 {
		t.Error("Expected to process some log lines")
	}

	if status.BytesProcessed == 0 {
		t.Error("Expected to process some bytes")
	}

	// 8. Check system status
	systemStatus := service.GetSystemStatus()
	fmt.Printf("\nSystem Status:\n")
	fmt.Printf("   - Total Lines: %d\n", systemStatus.TotalLines)
	fmt.Printf("   - Total Bytes: %d\n", systemStatus.TotalBytes)
	fmt.Printf("   - Active Processes: %d\n", systemStatus.ProcessesActive)

	if systemStatus.TotalLines == 0 {
		t.Error("Expected system to have processed some lines")
	}

	fmt.Println("\nIntegration test completed successfully!")
}

// createTestProcess creates a simple process that outputs logs for testing
func createTestProcess(ctx context.Context) (*os.Process, io.ReadCloser, error) {
	var cmd *exec.Cmd

	// Check if we're on Windows
	if isWindows() {
		// Windows PowerShell command - much more reliable
		cmd = exec.CommandContext(ctx, "powershell", "-Command", `
			for ($i = 1; $i -le 10; $i++) {
				$timestamp = Get-Date -Format "yyyy-MM-ddTHH:mm:ssZ"
				Write-Host "$timestamp INFO: Test log message $i from integration test"
				Start-Sleep -Milliseconds 200
			}
		`)
	} else {
		// Unix/Linux command
		cmd = exec.CommandContext(ctx, "sh", "-c", `
			for i in $(seq 1 10); do
				echo "$(date -u +"%Y-%m-%dT%H:%M:%SZ") INFO: Test log message $i from integration test"
				sleep 0.2
			done
		`)
	}

	// Get stdout pipe
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("failed to start command: %w", err)
	}

	return cmd.Process, stdout, nil
}

// isWindows checks if we're running on Windows
func isWindows() bool {
	return os.Getenv("OS") == "Windows_NT" ||
		strings.Contains(strings.ToLower(os.Getenv("PATH")), "windows") ||
		strings.Contains(strings.ToLower(os.Getenv("PATHEXT")), ".exe")
}

// TestRealTimeLogProcessing tests real-time log processing capabilities
func TestRealTimeLogProcessing(t *testing.T) {
	fmt.Println("\n⚡ Real-time Log Processing Test...")

	// Create logger and service
	logger := QuickLogger(InfoLevel)
	cfg := config.LogCollectionConfig{
		Enabled: true,
		GlobalAggregation: config.GlobalAggregationConfig{
			Enabled: true,
			Targets: []config.OutputTargetConfig{
				{Type: "process_manager_stdout", Format: "enhanced_plain"},
			},
		},
		Default: config.DefaultProcessLogConfig(),
		System: config.SystemConfig{
			MaxManagedProcesses: 5,
			FlushInterval:       100 * time.Millisecond,
		},
	}

	service := NewLogCollectionService(cfg, logger)

	ctx := context.Background()
	service.Start(ctx)
	defer service.Stop()

	// Register process
	processConfig := config.ProcessLogConfig{
		Enabled:       true,
		CaptureStdout: true,
		CaptureStderr: true,
		Buffering: config.BufferingConfig{
			Size:          "1MB",
			FlushInterval: 50 * time.Millisecond,
		},
		Processing: config.ProcessingConfig{
			ParseStructured: true,
			AddMetadata:     true,
		},
	}

	service.RegisterProcess("realtime-process", processConfig)

	// Simulate rapid log input
	fmt.Println("📊 Generating rapid log stream...")
	logLines := []string{
		`{"timestamp":"2025-01-20T10:30:45Z","level":"info","message":"Application starting","component":"main"}`,
		`Plain text log without structure`,
		`{"timestamp":"2025-01-20T10:30:46Z","level":"error","message":"Database connection failed","error":"timeout"}`,
		`2025-01-20 10:30:47 WARN: This is a warning message`,
		`{"timestamp":"2025-01-20T10:30:48Z","level":"info","message":"Retrying connection","attempt":2}`,
		`INFO: Application ready to serve requests`,
	}

	for i, line := range logLines {
		reader := strings.NewReader(line + "\n")
		service.CollectFromStream("realtime-process", reader, StdoutStream)

		if i%2 == 0 {
			time.Sleep(10 * time.Millisecond) // Simulate realistic timing
		}
	}

	// Allow processing time
	time.Sleep(200 * time.Millisecond)

	// Check results
	status, err := service.GetProcessStatus("realtime-process")
	if err != nil {
		t.Fatalf("Failed to get status: %v", err)
	}

	fmt.Printf("📈 Real-time Processing Results:\n")
	fmt.Printf("   - Lines: %d (expected 6)\n", status.LinesProcessed)
	fmt.Printf("   - Bytes: %d\n", status.BytesProcessed)

	if status.LinesProcessed < 6 {
		t.Errorf("Expected at least 6 lines, got %d", status.LinesProcessed)
	}

	fmt.Println("✅ Real-time processing test passed!")
}
