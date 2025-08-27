package logcollection

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/core-tools/hsu-core/pkg/errors"
	"github.com/core-tools/hsu-core/pkg/logcollection/config"
	"github.com/core-tools/hsu-core/pkg/processfile"
)

// ===== MAIN LOG COLLECTION SERVICE =====

// logCollectionService implements the LogCollectionService interface
type logCollectionService struct {
	// Configuration
	config config.LogCollectionConfig

	// Core components
	logger      StructuredLogger
	processes   map[string]*processLogCollector
	outputs     []LogOutputWriter
	pathManager *processfile.ProcessFileManager // NEW: For resolving log paths

	// State management
	mu      sync.RWMutex
	running int32 // atomic
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup

	// Metrics
	totalLines int64 // atomic
	totalBytes int64 // atomic
	startTime  time.Time
}

// NewLogCollectionService creates a new log collection service
func NewLogCollectionService(config config.LogCollectionConfig, logger StructuredLogger) LogCollectionService {
	return NewLogCollectionServiceWithPathManager(config, logger, nil)
}

// NewLogCollectionServiceWithPathManager creates a new log collection service with custom path manager
func NewLogCollectionServiceWithPathManager(config config.LogCollectionConfig, logger StructuredLogger, pathManager *processfile.ProcessFileManager) LogCollectionService {
	ctx, cancel := context.WithCancel(context.Background())

	// Create default path manager if none provided
	if pathManager == nil {
		pathManager = processfile.NewProcessFileManager(processfile.ProcessFileConfig{}, &simpleLoggerAdapter{logger})
	}

	return &logCollectionService{
		config:      config,
		logger:      logger,
		processes:   make(map[string]*processLogCollector),
		outputs:     make([]LogOutputWriter, 0),
		pathManager: pathManager,
		ctx:         ctx,
		cancel:      cancel,
	}
}

// resolveOutputPath resolves a relative path template to an absolute path
func (s *logCollectionService) resolveOutputPath(outputConfig config.OutputTargetConfig) string {
	if outputConfig.Type != "file" {
		return outputConfig.Path
	}

	// Check if path is already absolute
	if filepath.IsAbs(outputConfig.Path) {
		return outputConfig.Path
	}

	// Resolve relative path using path manager
	return s.pathManager.GenerateLogFilePath(outputConfig.Path)
}

// resolveProcessOutputPath resolves a process-specific path template to an absolute path
func (s *logCollectionService) resolveProcessOutputPath(outputConfig config.OutputTargetConfig, processID string) string {
	if outputConfig.Type != "file" {
		return outputConfig.Path
	}

	// Check if path is already absolute
	if filepath.IsAbs(outputConfig.Path) {
		return outputConfig.Path
	}

	// Resolve relative path using path manager for process logs
	return s.pathManager.GenerateProcessLogFilePath(outputConfig.Path, processID)
}

// ===== SERVICE LIFECYCLE =====

// Start starts the log collection service
func (s *logCollectionService) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&s.running, 0, 1) {
		return errors.NewValidationError("log collection service already running", nil)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.startTime = time.Now()

	// Initialize global output writers
	if err := s.initializeOutputs(); err != nil {
		atomic.StoreInt32(&s.running, 0)
		return errors.NewInternalError("failed to initialize outputs", err)
	}

	s.logger.WithFields(
		String("component", "log_collection"),
		Int("max_managed_processes", s.config.System.MaxManagedProcesses),
	).Infof("Log collection service started")

	return nil
}

// Stop stops the log collection service
func (s *logCollectionService) Stop() error {
	if !atomic.CompareAndSwapInt32(&s.running, 1, 0) {
		return errors.NewValidationError("log collection service not running", nil)
	}

	s.cancel()

	s.mu.Lock()
	defer s.mu.Unlock()

	// Stop all process collectors
	for processID, process := range s.processes {
		if err := process.stop(); err != nil {
			s.logger.WithError(err).Warnf("Error stopping process collector: %s", processID)
		}
	}

	// Close all outputs
	for _, output := range s.outputs {
		if err := output.Close(); err != nil {
			s.logger.WithError(err).Warnf("Error closing output writer")
		}
	}

	// Wait for all goroutines to complete
	s.wg.Wait()

	s.logger.WithFields(
		Duration("uptime", time.Since(s.startTime)),
		Int64("total_lines", atomic.LoadInt64(&s.totalLines)),
		Int64("total_bytes", atomic.LoadInt64(&s.totalBytes)),
	).Infof("Log collection service stopped")

	return nil
}

// ===== PROCESS MANAGEMENT =====

// RegisterProcess registers a new process for log collection
func (s *logCollectionService) RegisterProcess(processID string, processConfig config.ProcessLogConfig) error {
	if atomic.LoadInt32(&s.running) == 0 {
		return errors.NewValidationError("log collection service not running", nil)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.processes[processID]; exists {
		return errors.NewConflictError("process already registered", nil).WithContext("process_id", processID)
	}

	process := newProcessLogCollector(processID, processConfig, s.logger.WithProcess(processID), s)
	s.processes[processID] = process

	s.logger.WithFields(
		ManagedProcess(processID),
		Bool("capture_stdout", processConfig.CaptureStdout),
		Bool("capture_stderr", processConfig.CaptureStderr),
	).Infof("Managed process registered for log collection")

	return nil
}

// UnregisterProcess unregisters a process from log collection
func (s *logCollectionService) UnregisterProcess(processID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	process, exists := s.processes[processID]
	if !exists {
		return errors.NewNotFoundError("process not registered", nil).WithContext("process_id", processID)
	}

	if err := process.stop(); err != nil {
		s.logger.WithError(err).Warnf("Error stopping process collector: %s", processID)
	}

	delete(s.processes, processID)

	s.logger.WithProcess(processID).Infof("Managed process unregistered from log collection")

	return nil
}

// ===== LOG COLLECTION =====

// CollectFromStream collects logs from a single stream
func (s *logCollectionService) CollectFromStream(processID string, stream io.Reader, streamType StreamType) error {
	process, err := s.getProcess(processID)
	if err != nil {
		return err
	}

	return process.collectFromStream(stream, streamType)
}

// CollectFromProcess collects logs from both stdout and stderr of a process
func (s *logCollectionService) CollectFromProcess(processID string, stdout, stderr io.Reader) error {
	process, err := s.getProcess(processID)
	if err != nil {
		return err
	}

	return process.collectFromProcess(stdout, stderr)
}

// ProcessLogLine processes a single log line
func (s *logCollectionService) ProcessLogLine(processID string, line string, metadata LogMetadata) error {
	process, err := s.getProcess(processID)
	if err != nil {
		return err
	}

	return process.processLogLine(line, metadata)
}

// ForwardLogs forwards logs to external targets
func (s *logCollectionService) ForwardLogs(targets []LogOutputTarget) error {
	// Implementation will be added in Phase 3
	return errors.NewInternalError("external forwarding not yet implemented", nil)
}

// ===== CONFIGURATION MANAGEMENT =====

// UpdateConfiguration updates the service configuration
func (s *logCollectionService) UpdateConfiguration(newConfig config.LogCollectionConfig) error {
	if err := newConfig.Validate(); err != nil {
		return errors.NewValidationError("invalid configuration", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.config = newConfig

	s.logger.Infof("Log collection configuration updated")

	return nil
}

// GetConfiguration returns the current configuration
func (s *logCollectionService) GetConfiguration() config.LogCollectionConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.config
}

// ===== STATUS AND METRICS =====

// GetProcessStatus returns the status of a specific process
func (s *logCollectionService) GetProcessStatus(processID string) (*ProcessLogStatus, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	process, exists := s.processes[processID]
	if !exists {
		return nil, errors.NewNotFoundError("process not registered", nil).WithContext("process_id", processID)
	}

	return process.getStatus(), nil
}

// GetSystemStatus returns the overall system status
func (s *logCollectionService) GetSystemStatus() *SystemLogStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	processes := make(map[string]*ProcessLogStatus)
	activeProcesses := 0

	for processID, process := range s.processes {
		status := process.getStatus()
		processes[processID] = status
		if status.Active {
			activeProcesses++
		}
	}

	outputTargets := make([]string, len(s.outputs))
	for i, output := range s.outputs {
		outputTargets[i] = fmt.Sprintf("%T", output)
	}

	return &SystemLogStatus{
		Active:           atomic.LoadInt32(&s.running) == 1,
		ProcessesActive:  activeProcesses,
		TotalProcesses:   len(s.processes),
		TotalLines:       atomic.LoadInt64(&s.totalLines),
		TotalBytes:       atomic.LoadInt64(&s.totalBytes),
		StartTime:        s.startTime,
		LastActivity:     time.Now(), // TODO: Track actual last activity
		ManagedProcesses: processes,
		OutputTargets:    outputTargets,
	}
}

// ===== INTERNAL HELPERS =====

// getProcess retrieves a process collector safely
func (s *logCollectionService) getProcess(processID string) (*processLogCollector, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	process, exists := s.processes[processID]
	if !exists {
		return nil, errors.NewNotFoundError("process not registered", nil).WithContext("process_id", processID)
	}

	return process, nil
}

// initializeOutputs initializes global output writers
func (s *logCollectionService) initializeOutputs() error {
	if !s.config.GlobalAggregation.Enabled {
		return nil
	}

	s.outputs = make([]LogOutputWriter, 0, len(s.config.GlobalAggregation.Targets))

	for _, targetConfig := range s.config.GlobalAggregation.Targets {
		// Resolve path template to absolute path for file outputs
		resolvedConfig := targetConfig
		if targetConfig.Type == "file" {
			resolvedConfig.Path = s.resolveOutputPath(targetConfig)
		}

		writer, err := createOutputWriter(resolvedConfig)
		if err != nil {
			return errors.NewInternalError("failed to create output writer", err).WithContext("output_type", targetConfig.Type)
		}
		s.outputs = append(s.outputs, writer)
	}

	return nil
}

// recordMetrics records metrics for a processed log line
func (s *logCollectionService) recordMetrics(lineLength int) {
	atomic.AddInt64(&s.totalLines, 1)
	atomic.AddInt64(&s.totalBytes, int64(lineLength))
}

// ===== PROCESS LOG COLLECTOR =====

// processLogCollector handles log collection for a specific process
type processLogCollector struct {
	processID string
	config    config.ProcessLogConfig
	logger    StructuredLogger
	service   *logCollectionService

	// State
	mu             sync.RWMutex
	active         bool
	linesProcessed int64
	bytesProcessed int64
	lastActivity   time.Time
	errors         []string

	// Channels and goroutines
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// newProcessLogCollector creates a new process log collector
func newProcessLogCollector(processID string, config config.ProcessLogConfig, logger StructuredLogger, service *logCollectionService) *processLogCollector {
	return &processLogCollector{
		processID: processID,
		config:    config,
		logger:    logger,
		service:   service,
		stopCh:    make(chan struct{}),
		errors:    make([]string, 0),
	}
}

// collectFromStream collects logs from a single stream
func (w *processLogCollector) collectFromStream(stream io.Reader, streamType StreamType) error {
	if !w.config.Enabled {
		return nil
	}

	// Check if we should capture this stream type
	if streamType == StdoutStream && !w.config.CaptureStdout {
		return nil
	}
	if streamType == StderrStream && !w.config.CaptureStderr {
		return nil
	}

	w.mu.Lock()
	w.active = true
	w.mu.Unlock()

	w.wg.Add(1)
	go w.streamReader(stream, streamType)

	return nil
}

// collectFromProcess collects logs from both stdout and stderr
func (w *processLogCollector) collectFromProcess(stdout, stderr io.Reader) error {
	if !w.config.Enabled {
		return nil
	}

	errorCollection := errors.NewErrorCollection()

	if w.config.CaptureStdout && stdout != nil {
		if streamErr := w.collectFromStream(stdout, StdoutStream); streamErr != nil {
			wrappedErr := errors.NewInternalError("stdout collection failed", streamErr).WithContext("process_id", w.processID)
			errorCollection.Add(wrappedErr)
		}
	}

	if w.config.CaptureStderr && stderr != nil {
		if streamErr := w.collectFromStream(stderr, StderrStream); streamErr != nil {
			wrappedErr := errors.NewInternalError("stderr collection failed", streamErr).WithContext("process_id", w.processID)
			errorCollection.Add(wrappedErr)
		}
	}

	return errorCollection.ToError()
}

// streamReader reads from a stream and processes log lines
func (w *processLogCollector) streamReader(stream io.Reader, streamType StreamType) {
	defer w.wg.Done()

	scanner := bufio.NewScanner(stream)
	lineNum := int64(0)

	for scanner.Scan() {
		select {
		case <-w.stopCh:
			return
		default:
		}

		line := scanner.Text()
		lineNum++

		metadata := LogMetadata{
			Timestamp: time.Now(),
			ProcessID: w.processID,
			Stream:    streamType,
			LineNum:   lineNum,
		}

		if err := w.processLogLine(line, metadata); err != nil {
			w.logger.WithError(err).Warnf("Failed to process log line")
			w.recordError(fmt.Sprintf("Failed to process line %d: %v", lineNum, err))
		}
	}

	if err := scanner.Err(); err != nil {
		w.logger.WithError(err).Warnf("Error reading from stream")
		w.recordError(fmt.Sprintf("Stream reading error: %v", err))
	}

	w.mu.Lock()
	w.active = false
	w.mu.Unlock()
}

// processLogLine processes a single log line
func (w *processLogCollector) processLogLine(line string, metadata LogMetadata) error {
	// Update metrics
	w.mu.Lock()
	w.linesProcessed++
	w.bytesProcessed += int64(len(line))
	w.lastActivity = time.Now()
	w.mu.Unlock()

	// Record service-level metrics
	w.service.recordMetrics(len(line))

	// Create raw log entry
	rawEntry := RawLogEntry{
		ProcessID: metadata.ProcessID,
		Stream:    metadata.Stream,
		Line:      line,
		Timestamp: metadata.Timestamp,
	}

	// Note: Advanced log processing pipeline planned for Phase 4
	// Planned features: filtering, parsing, enhancement, structured metadata extraction
	// For now, just create a basic log entry
	logEntry := LogEntry{
		Timestamp: rawEntry.Timestamp,
		Message:   rawEntry.Line,
		ProcessID: rawEntry.ProcessID,
		Stream:    rawEntry.Stream,
		Raw:       rawEntry.Line,
	}

	// Write to outputs (Phase 1: Basic file output)
	return w.writeToOutputs(logEntry)
}

// writeToOutputs writes the log entry to configured outputs
func (w *processLogCollector) writeToOutputs(entry LogEntry) error {
	// Write to global aggregated outputs
	for _, output := range w.service.outputs {
		if err := output.Write(entry); err != nil {
			w.logger.WithError(err).Warnf("Failed to write to global output")
			w.recordError(fmt.Sprintf("Global output write error: %v", err))
		}
	}

	// Note: Managed process-specific log files planned for Phase 4
	// Feature: Individual log files per managed process with configurable rotation and retention

	return nil
}

// stop stops the process log collector
func (w *processLogCollector) stop() error {
	close(w.stopCh)
	w.wg.Wait()

	w.mu.Lock()
	w.active = false
	w.mu.Unlock()

	return nil
}

// getStatus returns the current status of the process
func (w *processLogCollector) getStatus() *ProcessLogStatus {
	w.mu.RLock()
	defer w.mu.RUnlock()

	// Copy errors to avoid data races
	errorsCopy := make([]string, len(w.errors))
	copy(errorsCopy, w.errors)

	return &ProcessLogStatus{
		ProcessID:      w.processID,
		Active:         w.active,
		LinesProcessed: w.linesProcessed,
		BytesProcessed: w.bytesProcessed,
		LastActivity:   w.lastActivity,
		Errors:         errorsCopy,
		Config:         w.config,
	}
}

// recordError records an error for status reporting
func (w *processLogCollector) recordError(errMsg string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.errors = append(w.errors, fmt.Sprintf("%s: %s", time.Now().Format(time.RFC3339), errMsg))

	// Keep only the last 10 errors
	if len(w.errors) > 10 {
		w.errors = w.errors[len(w.errors)-10:]
	}
}

// ===== TEMPORARY OUTPUT WRITER (Phase 1) =====

// createOutputWriter creates an output writer for a target
func createOutputWriter(targetConfig config.OutputTargetConfig) (LogOutputWriter, error) {
	switch targetConfig.Type {
	case "stdout", "process_manager_stdout":
		return &stdoutWriter{}, nil
	case "file":
		return &fileWriter{
			path: targetConfig.Path,
		}, nil
	default:
		return &stdoutWriter{}, nil // Fallback to stdout for unknown types
	}
}

// stdoutWriter is a basic output writer for console output
type stdoutWriter struct{}

func (s *stdoutWriter) Write(entry LogEntry) error {
	fmt.Printf("[%s][%s][%s] %s\n",
		entry.Timestamp.Format(time.RFC3339),
		entry.ProcessID,
		entry.Stream,
		entry.Message,
	)
	return nil
}

func (s *stdoutWriter) Flush() error {
	return nil
}

func (s *stdoutWriter) Close() error {
	return nil
}

// fileWriter writes logs to files with automatic directory creation
type fileWriter struct {
	path   string
	file   *os.File
	writer *bufio.Writer
	mutex  sync.Mutex
}

func (f *fileWriter) Write(entry LogEntry) error {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	// Ensure file is open
	if err := f.ensureFileOpen(); err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	// Write log entry
	logLine := fmt.Sprintf("[%s][%s][%s] %s\n",
		entry.Timestamp.Format(time.RFC3339),
		entry.ProcessID,
		entry.Stream,
		entry.Message,
	)

	if _, err := f.writer.WriteString(logLine); err != nil {
		return fmt.Errorf("failed to write log entry: %w", err)
	}

	return nil
}

func (f *fileWriter) Flush() error {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if f.writer != nil {
		return f.writer.Flush()
	}
	return nil
}

func (f *fileWriter) Close() error {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if f.writer != nil {
		f.writer.Flush()
	}
	if f.file != nil {
		return f.file.Close()
	}
	return nil
}

// ensureFileOpen creates the directory and opens the file if not already open
func (f *fileWriter) ensureFileOpen() error {
	if f.file != nil {
		return nil // Already open
	}

	// Create directory structure
	dir := filepath.Dir(f.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory %s: %w", dir, err)
	}

	// Open file for writing (create if not exists, append if exists)
	file, err := os.OpenFile(f.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file %s: %w", f.path, err)
	}

	f.file = file
	f.writer = bufio.NewWriter(file)
	return nil
}

// ===== LOGGER ADAPTER =====

// simpleLoggerAdapter adapts StructuredLogger to logging.Logger interface for ProcessFileManager
type simpleLoggerAdapter struct {
	logger StructuredLogger
}

func (s *simpleLoggerAdapter) Debugf(format string, args ...interface{}) {
	s.logger.Debugf(format, args...)
}

func (s *simpleLoggerAdapter) Infof(format string, args ...interface{}) {
	s.logger.Infof(format, args...)
}

func (s *simpleLoggerAdapter) Warnf(format string, args ...interface{}) {
	s.logger.Warnf(format, args...)
}

func (s *simpleLoggerAdapter) Errorf(format string, args ...interface{}) {
	s.logger.Errorf(format, args...)
}

func (s *simpleLoggerAdapter) LogLevelf(level int, format string, args ...interface{}) {
	// Convert int level to our LogLevel and delegate
	switch level {
	case 0: // Debug
		s.logger.Debugf(format, args...)
	case 1: // Info
		s.logger.Infof(format, args...)
	case 2: // Warn
		s.logger.Warnf(format, args...)
	case 3: // Error
		s.logger.Errorf(format, args...)
	default:
		s.logger.Infof(format, args...)
	}
}
