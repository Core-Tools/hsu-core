//go:build test

package logcollection

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/core-tools/hsu-procman-go/pkg/logcollection/config"
	"github.com/core-tools/hsu-procman-go/pkg/processfile"
)

// TestCrossPlatformLogPaths demonstrates the new cross-platform log path resolution
func TestCrossPlatformLogPaths(t *testing.T) {
	fmt.Printf("\nCross-Platform Log Path Resolution Test on %s\n", runtime.GOOS)

	// Create structured logger
	logger, err := NewStructuredLogger("zap", InfoLevel)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Test different deployment scenarios
	scenarios := []struct {
		name     string
		scenario string
		context  processfile.ServiceContext
	}{
		{"System Service", "system", processfile.SystemService},
		{"User Service", "user", processfile.UserService},
		{"Session Service", "session", processfile.SessionService},
		{"Development", "development", processfile.UserService},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			fmt.Printf("\nTesting %s scenario:\n", sc.name)

			// Create process file manager for this scenario
			pathConfig := processfile.GetRecommendedProcessFileConfig(sc.scenario, "")
			pathManager := processfile.NewProcessFileManager(pathConfig, &simpleLoggerAdapter{logger})

			// Create log collection service with path manager
			logConfig := config.DefaultLogCollectionConfig()
			service := NewLogCollectionServiceWithPathManager(logConfig, logger, pathManager)

			// Get the service with path manager
			serviceImpl := service.(*logCollectionService)

			// Test log directory resolution
			logDir := pathManager.GenerateLogDirectoryPath()
			processLogDir := pathManager.GenerateProcessLogDirectoryPath()

			fmt.Printf("   Log Directory: %s\n", logDir)
			fmt.Printf("   Managed Process Log Directory: %s\n", processLogDir)

			// Verify paths are OS-appropriate
			switch runtime.GOOS {
			case "windows":
				if !strings.Contains(logDir, "\\") {
					t.Errorf("Windows path should contain backslashes: %s", logDir)
				}
				if sc.context == processfile.SystemService && !strings.Contains(logDir, "ProgramData") {
					t.Errorf("System service on Windows should use ProgramData: %s", logDir)
				}

			case "darwin":
				if strings.Contains(logDir, "\\") {
					t.Errorf("Unix path should not contain backslashes: %s", logDir)
				}
				if sc.context == processfile.SystemService && !strings.Contains(logDir, "/var/log") {
					t.Errorf("System service on macOS should use /var/log: %s", logDir)
				}

			default: // Linux
				if strings.Contains(logDir, "\\") {
					t.Errorf("Unix path should not contain backslashes: %s", logDir)
				}
				if sc.context == processfile.SystemService && !strings.Contains(logDir, "/var/log") {
					t.Errorf("System service on Linux should use /var/log: %s", logDir)
				}
			}

			// Test path resolution for different output types
			testOutputs := []config.OutputTargetConfig{
				{Type: "file", Path: "aggregated.log"},
				{Type: "file", Path: "{process_id}-stdout.log"},
				{Type: "file", Path: "logs/custom.log"},
				{Type: "stdout", Path: "stdout"},
			}

			for _, output := range testOutputs {
				resolvedPath := serviceImpl.resolveOutputPath(output)
				resolvedProcessPath := serviceImpl.resolveProcessOutputPath(output, "test-process")

				fmt.Printf("   Template: %-25s → %s\n", output.Path, resolvedPath)
				fmt.Printf("   Managed Process Template: %-25s → %s\n", output.Path, resolvedProcessPath)

				if output.Type == "file" {
					// For file outputs, paths should be absolute
					if !filepath.IsAbs(resolvedPath) {
						t.Errorf("Resolved file path should be absolute: %s", resolvedPath)
					}
					if !filepath.IsAbs(resolvedProcessPath) && output.Path != "stdout" {
						t.Errorf("Resolved process file path should be absolute: %s", resolvedProcessPath)
					}

					// Managed process-specific paths should contain process ID
					if strings.Contains(output.Path, "{process_id}") {
						if !strings.Contains(resolvedProcessPath, "test-process") {
							t.Errorf("Managed process path should contain process ID: %s", resolvedProcessPath)
						}
					}
				}
			}

			fmt.Printf("   %s scenario completed successfully\n", sc.name)
		})
	}
}

// TestLogConfigTemplates demonstrates that config templates are now platform-agnostic
func TestLogConfigTemplates(t *testing.T) {
	fmt.Println("\n📋 Testing Platform-Agnostic Config Templates")

	// Test default configuration
	defaultConfig := config.DefaultLogCollectionConfig()

	// Verify global aggregation uses relative paths
	if len(defaultConfig.GlobalAggregation.Targets) > 0 {
		target := defaultConfig.GlobalAggregation.Targets[0]
		if filepath.IsAbs(target.Path) {
			t.Errorf("Global aggregation target should use relative path: %s", target.Path)
		}
		fmt.Printf("   Global aggregation target: %s (relative)\n", target.Path)
	}

	// Verify process directory is relative
	if filepath.IsAbs(defaultConfig.System.ProcessDirectory) {
		t.Errorf("Managed process directory should be relative: %s", defaultConfig.System.ProcessDirectory)
	}
	fmt.Printf("   Managed process directory: %s (relative)\n", defaultConfig.System.ProcessDirectory)

	// Test default process configuration
	processConfig := config.DefaultProcessLogConfig()

	// Verify process output paths are relative
	for i, target := range processConfig.Outputs.Separate.Stdout {
		if filepath.IsAbs(target.Path) {
			t.Errorf("Managed process stdout target %d should use relative path: %s", i, target.Path)
		}
		fmt.Printf("   Managed process stdout target: %s (relative)\n", target.Path)
	}

	for i, target := range processConfig.Outputs.Separate.Stderr {
		if filepath.IsAbs(target.Path) {
			t.Errorf("Managed process stderr target %d should use relative path: %s", i, target.Path)
		}
		fmt.Printf("   Managed process stderr target: %s (relative)\n", target.Path)
	}

	fmt.Println("   All config templates are platform-agnostic!")
}

// TestPathResolutionEdgeCases tests edge cases in path resolution
func TestPathResolutionEdgeCases(t *testing.T) {
	fmt.Println("\nTesting Path Resolution Edge Cases")

	logger := QuickLogger(InfoLevel)
	pathConfig := processfile.GetRecommendedProcessFileConfig("development", "test-app")
	pathManager := processfile.NewProcessFileManager(pathConfig, &simpleLoggerAdapter{logger})

	logConfig := config.DefaultLogCollectionConfig()
	service := NewLogCollectionServiceWithPathManager(logConfig, logger, pathManager)
	serviceImpl := service.(*logCollectionService)

	// Test absolute paths (should remain unchanged)
	absoluteOutput := config.OutputTargetConfig{
		Type: "file",
		Path: "/tmp/test.log",
	}
	if runtime.GOOS == "windows" {
		absoluteOutput.Path = "C:\\temp\\test.log"
	}

	resolved := serviceImpl.resolveOutputPath(absoluteOutput)
	if resolved != absoluteOutput.Path {
		t.Errorf("Absolute path should remain unchanged: expected %s, got %s", absoluteOutput.Path, resolved)
	}
	fmt.Printf("   Absolute path preserved: %s\n", resolved)

	// Test non-file outputs (should remain unchanged)
	stdoutOutput := config.OutputTargetConfig{
		Type: "stdout",
		Path: "stdout",
	}

	resolved = serviceImpl.resolveOutputPath(stdoutOutput)
	if resolved != stdoutOutput.Path {
		t.Errorf("Non-file output should remain unchanged: expected %s, got %s", stdoutOutput.Path, resolved)
	}
	fmt.Printf("   Non-file output preserved: %s\n", resolved)

	// Test complex templates with subdirectories
	complexOutput := config.OutputTargetConfig{
		Type: "file",
		Path: "subfolder/deep/nested/{process_id}.log",
	}

	resolved = serviceImpl.resolveProcessOutputPath(complexOutput, "my-process")
	if !strings.Contains(resolved, "my-process") {
		t.Errorf("Complex template should contain process ID: %s", resolved)
	}
	if !strings.Contains(resolved, "subfolder") {
		t.Errorf("Complex template should contain subfolder: %s", resolved)
	}
	fmt.Printf("   Complex template resolved: %s\n", resolved)

	fmt.Println("   All edge cases handled correctly!")
}
