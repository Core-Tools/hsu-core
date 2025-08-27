package processfile

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ProcessFileMockLogger is a simple mock implementation of Logger for testing
type ProcessFileMockLogger struct{}

func (m *ProcessFileMockLogger) LogLevelf(level int, format string, args ...interface{}) {}
func (m *ProcessFileMockLogger) Debugf(format string, args ...interface{})               {}
func (m *ProcessFileMockLogger) Infof(format string, args ...interface{})                {}
func (m *ProcessFileMockLogger) Warnf(format string, args ...interface{})                {}
func (m *ProcessFileMockLogger) Errorf(format string, args ...interface{})               {}

func TestNewProcessFileManager(t *testing.T) {
	config := ProcessFileConfig{
		ServiceContext: SystemService,
		AppName:        "test-app",
		BaseDirectory:  "/tmp/test",
	}
	logger := &ProcessFileMockLogger{}

	manager := NewProcessFileManager(config, logger)

	assert.NotNil(t, manager)
	assert.Equal(t, config.ServiceContext, manager.config.ServiceContext)
	assert.Equal(t, config.AppName, manager.config.AppName)
}

func TestNewProcessFileManager_WithDefaults(t *testing.T) {
	config := ProcessFileConfig{} // Empty config
	logger := &ProcessFileMockLogger{}

	manager := NewProcessFileManager(config, logger)

	assert.NotNil(t, manager)
	assert.Equal(t, DefaultAppName, manager.config.AppName)
	assert.Equal(t, UserService, manager.config.ServiceContext)
}

func TestGeneratePIDFilePath_SystemService(t *testing.T) {
	config := ProcessFileConfig{
		ServiceContext:  SystemService,
		AppName:         "test-app",
		UseSubdirectory: true,
	}

	manager := NewProcessFileManager(config, &ProcessFileMockLogger{})
	path := manager.GeneratePIDFilePath("test-process")

	assert.NotEmpty(t, path)
	assert.Contains(t, path, "test-app")
	assert.Contains(t, path, "test-process.pid")
}

func TestGeneratePIDFilePath_UserService(t *testing.T) {
	config := ProcessFileConfig{
		ServiceContext:  UserService,
		AppName:         "test-app",
		UseSubdirectory: true,
	}

	manager := NewProcessFileManager(config, &ProcessFileMockLogger{})
	path := manager.GeneratePIDFilePath("test-process")

	assert.NotEmpty(t, path)
	assert.Contains(t, path, "test-app")
	assert.Contains(t, path, "test-process.pid")
}

func TestGeneratePIDFilePath_SessionService(t *testing.T) {
	config := ProcessFileConfig{
		ServiceContext:  SessionService,
		AppName:         "test-app",
		UseSubdirectory: false,
	}

	manager := NewProcessFileManager(config, &ProcessFileMockLogger{})
	path := manager.GeneratePIDFilePath("test-process")

	assert.NotEmpty(t, path)
	// very different and platform-dependent path, could not check more
	assert.Contains(t, path, "test-process.pid")
}

func TestGeneratePIDFilePath_WithCustomBaseDirectory(t *testing.T) {
	customPath := "/custom/path"
	if runtime.GOOS == "windows" {
		customPath = "C:\\custom\\path"
	}

	config := ProcessFileConfig{
		BaseDirectory:   customPath,
		ServiceContext:  SystemService,
		AppName:         "test-app",
		UseSubdirectory: true,
	}

	manager := NewProcessFileManager(config, &ProcessFileMockLogger{})
	path := manager.GeneratePIDFilePath("test-process")

	assert.NotEmpty(t, path)
	assert.Contains(t, path, customPath)
	assert.Contains(t, path, "test-process.pid")
}

func TestGeneratePIDFilePath_WithoutSubdirectory(t *testing.T) {
	testPath := "/tmp/test"
	if runtime.GOOS == "windows" {
		testPath = "C:\\tmp\\test"
	}

	config := ProcessFileConfig{
		BaseDirectory:   testPath,
		ServiceContext:  SystemService,
		AppName:         "test-app",
		UseSubdirectory: false,
	}

	manager := NewProcessFileManager(config, &ProcessFileMockLogger{})
	path := manager.GeneratePIDFilePath("test-process")

	assert.NotEmpty(t, path)
	assert.Contains(t, path, testPath)
	assert.Contains(t, path, "test-process.pid")
	assert.NotContains(t, path, "test-app")
}

func TestValidateDirectory_Success(t *testing.T) {
	tempDir := t.TempDir()
	config := ProcessFileConfig{
		BaseDirectory:   tempDir,
		ServiceContext:  UserService,
		AppName:         "test-app",
		UseSubdirectory: false,
	}

	manager := NewProcessFileManager(config, &ProcessFileMockLogger{})
	pidFile := manager.GeneratePIDFilePath("test-process")

	err := ValidateDirectory(pidFile)

	assert.NoError(t, err)
}

func TestValidateDirectory_CreateDirectory(t *testing.T) {
	tempDir := t.TempDir()
	testDir := filepath.Join(tempDir, "non-existent")
	config := ProcessFileConfig{
		BaseDirectory:   testDir,
		ServiceContext:  UserService,
		AppName:         "test-app",
		UseSubdirectory: false,
	}

	manager := NewProcessFileManager(config, &ProcessFileMockLogger{})
	pidFile := manager.GeneratePIDFilePath("test-process")

	err := ValidateDirectory(pidFile)

	assert.NoError(t, err)
	assert.DirExists(t, testDir)
}

func TestValidateDirectory_InvalidPath(t *testing.T) {
	config := ProcessFileConfig{
		BaseDirectory:   "/root/cannot-create",
		ServiceContext:  UserService,
		AppName:         "test-app",
		UseSubdirectory: false,
	}

	manager := NewProcessFileManager(config, &ProcessFileMockLogger{})
	pidFile := manager.GeneratePIDFilePath("test-process")

	err := ValidateDirectory(pidFile)

	if runtime.GOOS != "windows" {
		assert.Error(t, err)
	}
}

func TestGetRecommendedProcessFileConfig(t *testing.T) {
	testCases := []struct {
		name               string
		scenario           string
		appName            string
		expectedContext    ServiceContext
		expectedSubdir     bool
		expectedAppName    string
		expectedHasBaseDir bool
	}{
		{
			name:            "system_service",
			scenario:        "system",
			appName:         "my-app",
			expectedContext: SystemService,
			expectedSubdir:  true,
			expectedAppName: "my-app",
		},
		{
			name:            "user_service",
			scenario:        "user",
			appName:         "my-app",
			expectedContext: UserService,
			expectedSubdir:  true,
			expectedAppName: "my-app",
		},
		{
			name:            "session_service",
			scenario:        "session",
			appName:         "my-app",
			expectedContext: SessionService,
			expectedSubdir:  false,
			expectedAppName: "my-app",
		},
		{
			name:               "development",
			scenario:           "development",
			appName:            "my-app",
			expectedContext:    UserService,
			expectedSubdir:     false,
			expectedAppName:    "my-app",
			expectedHasBaseDir: true,
		},
		{
			name:            "empty_app_name_uses_default",
			scenario:        "system",
			appName:         "",
			expectedContext: SystemService,
			expectedSubdir:  true,
			expectedAppName: DefaultAppName,
		},
		{
			name:            "unknown_scenario_defaults_to_user",
			scenario:        "unknown",
			appName:         "my-app",
			expectedContext: UserService,
			expectedSubdir:  true,
			expectedAppName: "my-app",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := GetRecommendedProcessFileConfig(tc.scenario, tc.appName)

			assert.Equal(t, tc.expectedContext, config.ServiceContext)
			assert.Equal(t, tc.expectedSubdir, config.UseSubdirectory)
			assert.Equal(t, tc.expectedAppName, config.AppName)

			if tc.expectedHasBaseDir {
				assert.NotEmpty(t, config.BaseDirectory)
			}
		})
	}
}

func TestProcessFileManager_MultipleProcesses(t *testing.T) {
	config := ProcessFileConfig{
		BaseDirectory:   t.TempDir(),
		ServiceContext:  UserService,
		AppName:         "test-app",
		UseSubdirectory: false,
	}

	manager := NewProcessFileManager(config, &ProcessFileMockLogger{})

	// Generate paths for multiple managed processes
	process1Path := manager.GeneratePIDFilePath("process-1")
	process2Path := manager.GeneratePIDFilePath("process-2")

	assert.NotEqual(t, process1Path, process2Path)
	assert.Contains(t, process1Path, "process-1.pid")
	assert.Contains(t, process2Path, "process-2.pid")
}

func TestProcessFileManager_PathSeparators(t *testing.T) {
	config := ProcessFileConfig{
		BaseDirectory:   t.TempDir(),
		ServiceContext:  UserService,
		AppName:         "test-app",
		UseSubdirectory: false,
	}

	manager := NewProcessFileManager(config, &ProcessFileMockLogger{})
	path := manager.GeneratePIDFilePath("test-process")

	assert.NotEmpty(t, path)
	assert.Contains(t, path, "test-process.pid")
}

func TestProcessFileManager_ProcessIDValidation(t *testing.T) {
	config := ProcessFileConfig{
		BaseDirectory:   t.TempDir(),
		ServiceContext:  UserService,
		AppName:         "test-app",
		UseSubdirectory: false,
	}

	manager := NewProcessFileManager(config, &ProcessFileMockLogger{})

	// Test with various process IDs
	testCases := []string{
		"simple-process",
		"process_with_underscores",
		"process123",
		"Managed-Process-With-Mixed-Case",
	}

	for _, processID := range testCases {
		t.Run(processID, func(t *testing.T) {
			path := manager.GeneratePIDFilePath(processID)
			assert.NotEmpty(t, path)
			assert.Contains(t, path, processID+".pid")
		})
	}
}

func TestProcessFileManager_WritePIDFile(t *testing.T) {
	tempDir := t.TempDir()
	config := ProcessFileConfig{
		BaseDirectory:   tempDir,
		ServiceContext:  UserService,
		AppName:         "test-app",
		UseSubdirectory: false,
	}

	manager := NewProcessFileManager(config, &ProcessFileMockLogger{})
	processID := "test-process"
	pid := 12345

	err := manager.WritePIDFile(processID, pid)

	assert.NoError(t, err)

	// Verify file was created with correct content
	pidFilePath := manager.GeneratePIDFilePath(processID)
	assert.FileExists(t, pidFilePath)

	content, err := os.ReadFile(pidFilePath)
	assert.NoError(t, err)
	assert.Equal(t, "12345\n", string(content))
}

func TestProcessFileManager_WritePIDFile_InvalidDirectory(t *testing.T) {
	config := ProcessFileConfig{
		BaseDirectory:   "/root/cannot-create",
		ServiceContext:  UserService,
		AppName:         "test-app",
		UseSubdirectory: false,
	}

	manager := NewProcessFileManager(config, &ProcessFileMockLogger{})
	processID := "test-process"
	pid := 12345

	err := manager.WritePIDFile(processID, pid)

	if runtime.GOOS != "windows" {
		assert.Error(t, err)
	}
}

func TestProcessFileManager_WritePIDFile_WithSubdirectory(t *testing.T) {
	tempDir := t.TempDir()
	config := ProcessFileConfig{
		BaseDirectory:   tempDir,
		ServiceContext:  UserService,
		AppName:         "test-app",
		UseSubdirectory: true,
	}

	manager := NewProcessFileManager(config, &ProcessFileMockLogger{})
	processID := "test-process"
	pid := 12345

	err := manager.WritePIDFile(processID, pid)

	assert.NoError(t, err)

	// Verify file was created with correct content
	pidFilePath := manager.GeneratePIDFilePath(processID)
	assert.FileExists(t, pidFilePath)

	content, err := os.ReadFile(pidFilePath)
	assert.NoError(t, err)
	assert.Equal(t, "12345\n", string(content))
}

func TestProcessFileManager_WritePortFile(t *testing.T) {
	tempDir := t.TempDir()
	config := ProcessFileConfig{
		BaseDirectory:   tempDir,
		ServiceContext:  UserService,
		AppName:         "test-app",
		UseSubdirectory: false,
	}

	manager := NewProcessFileManager(config, &ProcessFileMockLogger{})
	processID := "test-process"
	port := 8080

	err := manager.WritePortFile(processID, port)

	assert.NoError(t, err)

	// Verify file was created with correct content
	portFilePath := manager.GeneratePortFilePath(processID)
	assert.FileExists(t, portFilePath)

	content, err := os.ReadFile(portFilePath)
	assert.NoError(t, err)
	assert.Equal(t, "8080\n", string(content))
}

func TestProcessFileManager_ReadPortFile(t *testing.T) {
	tempDir := t.TempDir()
	config := ProcessFileConfig{
		BaseDirectory:   tempDir,
		ServiceContext:  UserService,
		AppName:         "test-app",
		UseSubdirectory: false,
	}

	manager := NewProcessFileManager(config, &ProcessFileMockLogger{})
	processID := "test-process"
	expectedPort := 8080

	// Write port file first
	err := manager.WritePortFile(processID, expectedPort)
	require.NoError(t, err)

	// Read port file
	port, err := manager.ReadPortFile(processID)

	assert.NoError(t, err)
	assert.Equal(t, expectedPort, port)
}

func TestProcessFileManager_ReadPortFile_InvalidFile(t *testing.T) {
	tempDir := t.TempDir()
	config := ProcessFileConfig{
		BaseDirectory:   tempDir,
		ServiceContext:  UserService,
		AppName:         "test-app",
		UseSubdirectory: false,
	}

	manager := NewProcessFileManager(config, &ProcessFileMockLogger{})
	processID := "nonexistent-process"

	// Test reading non-existent port file
	_, err := manager.ReadPortFile(processID)

	assert.Error(t, err)
}

func TestProcessFileManager_ReadPortFile_InvalidContent(t *testing.T) {
	tempDir := t.TempDir()
	config := ProcessFileConfig{
		BaseDirectory:   tempDir,
		ServiceContext:  UserService,
		AppName:         "test-app",
		UseSubdirectory: false,
	}

	manager := NewProcessFileManager(config, &ProcessFileMockLogger{})
	processID := "test-process"

	// Write invalid content to port file
	portFilePath := manager.GeneratePortFilePath(processID)
	err := os.MkdirAll(filepath.Dir(portFilePath), 0755)
	require.NoError(t, err)
	err = os.WriteFile(portFilePath, []byte("invalid-port"), 0644)
	require.NoError(t, err)

	// Test reading invalid port file
	_, err = manager.ReadPortFile(processID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid port in port file")
}

func TestProcessFileManager_GeneratePortFilePath(t *testing.T) {
	testPath := "/tmp/test"
	if runtime.GOOS == "windows" {
		testPath = "C:\\tmp\\test"
	}

	config := ProcessFileConfig{
		BaseDirectory:   testPath,
		ServiceContext:  UserService,
		AppName:         "test-app",
		UseSubdirectory: false,
	}

	manager := NewProcessFileManager(config, &ProcessFileMockLogger{})
	processID := "test-process"

	portFilePath := manager.GeneratePortFilePath(processID)

	assert.NotEmpty(t, portFilePath)
	assert.Contains(t, portFilePath, "test-process.port")
	assert.Contains(t, portFilePath, testPath)
}

func TestProcessFileManager_GeneratePortFilePath_WithSubdirectory(t *testing.T) {
	testPath := "/tmp/test"
	if runtime.GOOS == "windows" {
		testPath = "C:\\tmp\\test"
	}

	config := ProcessFileConfig{
		BaseDirectory:   testPath,
		ServiceContext:  UserService,
		AppName:         "test-app",
		UseSubdirectory: true,
	}

	manager := NewProcessFileManager(config, &ProcessFileMockLogger{})
	processID := "test-process"

	portFilePath := manager.GeneratePortFilePath(processID)

	assert.NotEmpty(t, portFilePath)
	assert.Contains(t, portFilePath, "test-process.port")
	assert.Contains(t, portFilePath, testPath)
	assert.Contains(t, portFilePath, "test-app")
}
