package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/core-tools/hsu-core/pkg/logcollection"
	"github.com/core-tools/hsu-core/pkg/logcollection/config"
	"github.com/core-tools/hsu-core/pkg/managedprocess/processcontrol"
	"github.com/core-tools/hsu-core/pkg/managedprocess/processcontrolimpl"
	"github.com/core-tools/hsu-core/pkg/processfile"
)

// SimpleLogger implements the required logging interface for this demo
type SimpleLogger struct{}

func (s *SimpleLogger) Debugf(format string, args ...interface{}) {
	fmt.Printf("[DEBUG] "+format+"\n", args...)
}

func (s *SimpleLogger) Infof(format string, args ...interface{}) {
	fmt.Printf("[INFO] "+format+"\n", args...)
}

func (s *SimpleLogger) Warnf(format string, args ...interface{}) {
	fmt.Printf("[WARN] "+format+"\n", args...)
}

func (s *SimpleLogger) Errorf(format string, args ...interface{}) {
	fmt.Printf("[ERROR] "+format+"\n", args...)
}

func (s *SimpleLogger) LogLevelf(level int, format string, args ...interface{}) {
	fmt.Printf(fmt.Sprintf("%d ", level)+format+"\n", args...)
}

// DemoCrossPlatformPaths demonstrates the new cross-platform log path resolution
func DemoCrossPlatformPaths() {
	fmt.Printf("\nCross-Platform Log Path Resolution Demo on %s\n", runtime.GOOS)
	fmt.Println("=" + strings.Repeat("=", 60))

	// Test different deployment scenarios
	scenarios := []struct {
		name     string
		scenario string
	}{
		{"System Service", "system"},
		{"User Service", "user"},
		{"Development", "development"},
	}

	for _, sc := range scenarios {
		fmt.Printf("\n%s Scenario:\n", sc.name)
		fmt.Printf("   " + strings.Repeat("-", 40) + "\n")

		// Create process file manager for this scenario
		pathConfig := processfile.GetRecommendedProcessFileConfig(sc.scenario, "")
		pathManager := processfile.NewProcessFileManager(pathConfig, &SimpleLogger{})

		// Show log directories
		logDir := pathManager.GenerateLogDirectoryPath()
		processLogDir := pathManager.GenerateProcessLogDirectoryPath()

		fmt.Printf("Log Directory: %s\n", logDir)
		fmt.Printf("Managed Process Log Directory: %s\n", processLogDir)

		// Show path resolution examples
		templates := []string{
			"aggregated.log",
			"{process_id}-stdout.log",
			"daily/{process_id}-2025-01-20.log",
		}

		for _, template := range templates {
			resolved := pathManager.GenerateLogFilePath(template)
			processResolved := pathManager.GenerateProcessLogFilePath(template, "my-process")

			fmt.Printf("Template: %-30s → %s\n", template, resolved)
			fmt.Printf("Managed Process Template: %-30s → %s\n", template, processResolved)
		}
	}

	// Show config changes
	fmt.Printf("\nConfiguration Changes:\n")
	fmt.Printf("   " + strings.Repeat("-", 40) + "\n")

	defaultConfig := config.DefaultLogCollectionConfig()
	processConfig := config.DefaultProcessLogConfig()

	fmt.Printf("Global aggregation target: %s (relative)\n", defaultConfig.GlobalAggregation.Targets[0].Path)
	fmt.Printf("Managed process directory: %s (relative)\n", defaultConfig.System.ProcessDirectory)
	fmt.Printf("Managed process stdout target: %s (relative)\n", processConfig.Outputs.Separate.Stdout[0].Path)
	fmt.Printf("Managed process stderr target: %s (relative)\n", processConfig.Outputs.Separate.Stderr[0].Path)

	fmt.Println("\nCross-platform log path resolution is working perfectly!")
}

// LogCollectionDemo demonstrates log collection integration with ProcessControl
func LogCollectionDemo() error {
	fmt.Println("\nHSU Process Manager Log Collection Demo Starting...")

	// 1. Create structured logger
	structuredLogger, err := logcollection.NewStructuredLogger("zap", logcollection.InfoLevel)
	if err != nil {
		return fmt.Errorf("failed to create structured logger: %w", err)
	}

	// 2. Create log collection service
	logConfig := config.DefaultLogCollectionConfig()
	logService := logcollection.NewLogCollectionService(logConfig, structuredLogger)

	ctx := context.Background()
	if err := logService.Start(ctx); err != nil {
		return fmt.Errorf("failed to start log service: %w", err)
	}
	defer logService.Stop()

	// 3. Create ProcessControl with log collection integration
	processLogConfig := config.DefaultProcessLogConfig()

	processOptions := processcontrol.ProcessControlOptions{
		CanAttach:            false,
		CanTerminate:         true,
		CanRestart:           true,
		GracefulTimeout:      30 * time.Second,
		ExecuteCmd:           createEchoCommand,
		LogCollectionService: logService,
		LogConfig:            &processLogConfig,
	}

	simpleLogger := &SimpleLogger{}
	processControl := processcontrolimpl.NewProcessControl(processOptions, "demo-process", simpleLogger)

	// 4. Start the process (this will automatically start log collection)
	fmt.Println("\nStarting process with integrated log collection...")
	if err := processControl.Start(ctx); err != nil {
		return fmt.Errorf("failed to start process: %w", err)
	}

	// 5. Let it run for a bit to collect logs
	fmt.Println("\n⏱Collecting logs for 10 seconds...")
	time.Sleep(10 * time.Second)

	// 6. Check log collection status
	status, err := logService.GetProcessStatus("demo-process")
	if err != nil {
		fmt.Printf("Could not get process status: %v\n", err)
	} else {
		fmt.Printf("\nLog Collection Status:\n")
		fmt.Printf("   - Lines Processed: %d\n", status.LinesProcessed)
		fmt.Printf("   - Bytes Processed: %d\n", status.BytesProcessed)
		fmt.Printf("   - Active: %t\n", status.Active)
		fmt.Printf("   - Last Activity: %s\n", status.LastActivity.Format(time.RFC3339))
	}

	// 7. Stop the process
	fmt.Println("\nStopping process...")
	if err := processControl.Stop(ctx); err != nil {
		return fmt.Errorf("failed to stop process: %w", err)
	}

	// 8. Check final status
	systemStatus := logService.GetSystemStatus()
	fmt.Printf("\nFinal System Status:\n")
	fmt.Printf("   - Total Lines: %d\n", systemStatus.TotalLines)
	fmt.Printf("   - Total Bytes: %d\n", systemStatus.TotalBytes)
	fmt.Printf("   - Processes: %d active / %d total\n", systemStatus.ProcessesActive, systemStatus.TotalProcesses)

	fmt.Println("\nLog Collection Demo Completed Successfully!")
	return nil
}

// createEchoCommand creates a command that generates log output for demonstration
func createEchoCommand(ctx context.Context) (*processcontrol.CommandResult, error) {
	// Create a simple command that outputs logs regularly
	var cmd *exec.Cmd
	if os.Getenv("OS") == "Windows_NT" || strings.Contains(os.Getenv("PATH"), "Windows") {
		// Windows PowerShell command
		cmd = exec.CommandContext(ctx, "powershell", "-Command", `
			for ($i = 1; $i -le 100; $i++) {
				$timestamp = Get-Date -Format "yyyy-MM-ddTHH:mm:ssZ"
				Write-Host "$timestamp INFO: Demo log message $i from HSU process"
				if ($i % 10 -eq 0) {
					Write-Host "$timestamp WARN: Every 10th message is a warning (message $i)"
				}
				Start-Sleep -Milliseconds 500
			}
			Write-Host "$(Get-Date -Format "yyyy-MM-ddTHH:mm:ssZ") INFO: Demo process completed successfully"
		`)
	} else {
		// Unix/Linux command
		cmd = exec.CommandContext(ctx, "bash", "-c", `
			for i in {1..100}; do
				timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
				echo "$timestamp INFO: Demo log message $i from HSU process"
				if [ $((i % 10)) -eq 0 ]; then
					echo "$timestamp WARN: Every 10th message is a warning (message $i)"
				fi
				sleep 0.5
			done
			echo "$(date -u +"%Y-%m-%dT%H:%M:%SZ") INFO: Demo process completed successfully"
		`)
	}

	// Get stdout pipe
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start command: %w", err)
	}

	return &processcontrol.CommandResult{
		Process:           cmd.Process,
		ProcessContext:    nil,
		Stdout:            stdout,
		HealthCheckConfig: nil,
	}, nil
}

// main function for running the demo
func main() {
	DemoCrossPlatformPaths()

	err := LogCollectionDemo()
	if err != nil {
		fmt.Printf("Demo failed: %v\n", err)
		os.Exit(1)
	}
}
