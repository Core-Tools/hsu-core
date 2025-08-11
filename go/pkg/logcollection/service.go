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
	workers     map[string]*workerLogCollector
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
		workers:     make(map[string]*workerLogCollector),
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

// resolveWorkerOutputPath resolves a worker-specific path template to an absolute path
func (s *logCollectionService) resolveWorkerOutputPath(outputConfig config.OutputTargetConfig, workerID string) string {
	if outputConfig.Type != "file" {
		return outputConfig.Path
	}

	// Check if path is already absolute
	if filepath.IsAbs(outputConfig.Path) {
		return outputConfig.Path
	}

	// Resolve relative path using path manager for worker logs
	return s.pathManager.GenerateWorkerLogFilePath(outputConfig.Path, workerID)
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
		Int("max_workers", s.config.System.MaxWorkers),
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

	// Stop all worker collectors
	for workerID, worker := range s.workers {
		if err := worker.stop(); err != nil {
			s.logger.WithError(err).Warnf("Error stopping worker collector: %s", workerID)
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

// ===== WORKER MANAGEMENT =====

// RegisterWorker registers a new worker for log collection
func (s *logCollectionService) RegisterWorker(workerID string, workerConfig config.WorkerLogConfig) error {
	if atomic.LoadInt32(&s.running) == 0 {
		return errors.NewValidationError("log collection service not running", nil)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.workers[workerID]; exists {
		return errors.NewConflictError("worker already registered", nil).WithContext("worker_id", workerID)
	}

	worker := newWorkerLogCollector(workerID, workerConfig, s.logger.WithWorker(workerID), s)
	s.workers[workerID] = worker

	s.logger.WithFields(
		Worker(workerID),
		Bool("capture_stdout", workerConfig.CaptureStdout),
		Bool("capture_stderr", workerConfig.CaptureStderr),
	).Infof("Worker registered for log collection")

	return nil
}

// UnregisterWorker unregisters a worker from log collection
func (s *logCollectionService) UnregisterWorker(workerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	worker, exists := s.workers[workerID]
	if !exists {
		return errors.NewNotFoundError("worker not registered", nil).WithContext("worker_id", workerID)
	}

	if err := worker.stop(); err != nil {
		s.logger.WithError(err).Warnf("Error stopping worker collector: %s", workerID)
	}

	delete(s.workers, workerID)

	s.logger.WithWorker(workerID).Infof("Worker unregistered from log collection")

	return nil
}

// ===== LOG COLLECTION =====

// CollectFromStream collects logs from a single stream
func (s *logCollectionService) CollectFromStream(workerID string, stream io.Reader, streamType StreamType) error {
	worker, err := s.getWorker(workerID)
	if err != nil {
		return err
	}

	return worker.collectFromStream(stream, streamType)
}

// CollectFromProcess collects logs from both stdout and stderr of a process
func (s *logCollectionService) CollectFromProcess(workerID string, stdout, stderr io.Reader) error {
	worker, err := s.getWorker(workerID)
	if err != nil {
		return err
	}

	return worker.collectFromProcess(stdout, stderr)
}

// ProcessLogLine processes a single log line
func (s *logCollectionService) ProcessLogLine(workerID string, line string, metadata LogMetadata) error {
	worker, err := s.getWorker(workerID)
	if err != nil {
		return err
	}

	return worker.processLogLine(line, metadata)
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

// GetWorkerStatus returns the status of a specific worker
func (s *logCollectionService) GetWorkerStatus(workerID string) (*WorkerLogStatus, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	worker, exists := s.workers[workerID]
	if !exists {
		return nil, errors.NewNotFoundError("worker not registered", nil).WithContext("worker_id", workerID)
	}

	return worker.getStatus(), nil
}

// GetSystemStatus returns the overall system status
func (s *logCollectionService) GetSystemStatus() *SystemLogStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	workers := make(map[string]*WorkerLogStatus)
	activeWorkers := 0

	for workerID, worker := range s.workers {
		status := worker.getStatus()
		workers[workerID] = status
		if status.Active {
			activeWorkers++
		}
	}

	outputTargets := make([]string, len(s.outputs))
	for i, output := range s.outputs {
		outputTargets[i] = fmt.Sprintf("%T", output)
	}

	return &SystemLogStatus{
		Active:        atomic.LoadInt32(&s.running) == 1,
		WorkersActive: activeWorkers,
		TotalWorkers:  len(s.workers),
		TotalLines:    atomic.LoadInt64(&s.totalLines),
		TotalBytes:    atomic.LoadInt64(&s.totalBytes),
		StartTime:     s.startTime,
		LastActivity:  time.Now(), // TODO: Track actual last activity
		Workers:       workers,
		OutputTargets: outputTargets,
	}
}

// ===== INTERNAL HELPERS =====

// getWorker retrieves a worker collector safely
func (s *logCollectionService) getWorker(workerID string) (*workerLogCollector, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	worker, exists := s.workers[workerID]
	if !exists {
		return nil, errors.NewNotFoundError("worker not registered", nil).WithContext("worker_id", workerID)
	}

	return worker, nil
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

// ===== WORKER LOG COLLECTOR =====

// workerLogCollector handles log collection for a specific worker
type workerLogCollector struct {
	workerID string
	config   config.WorkerLogConfig
	logger   StructuredLogger
	service  *logCollectionService

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

// newWorkerLogCollector creates a new worker log collector
func newWorkerLogCollector(workerID string, config config.WorkerLogConfig, logger StructuredLogger, service *logCollectionService) *workerLogCollector {
	return &workerLogCollector{
		workerID: workerID,
		config:   config,
		logger:   logger,
		service:  service,
		stopCh:   make(chan struct{}),
		errors:   make([]string, 0),
	}
}

// collectFromStream collects logs from a single stream
func (w *workerLogCollector) collectFromStream(stream io.Reader, streamType StreamType) error {
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
func (w *workerLogCollector) collectFromProcess(stdout, stderr io.Reader) error {
	if !w.config.Enabled {
		return nil
	}

	errorCollection := errors.NewErrorCollection()

	if w.config.CaptureStdout && stdout != nil {
		if streamErr := w.collectFromStream(stdout, StdoutStream); streamErr != nil {
			wrappedErr := errors.NewInternalError("stdout collection failed", streamErr).WithContext("worker_id", w.workerID)
			errorCollection.Add(wrappedErr)
		}
	}

	if w.config.CaptureStderr && stderr != nil {
		if streamErr := w.collectFromStream(stderr, StderrStream); streamErr != nil {
			wrappedErr := errors.NewInternalError("stderr collection failed", streamErr).WithContext("worker_id", w.workerID)
			errorCollection.Add(wrappedErr)
		}
	}

	return errorCollection.ToError()
}

// streamReader reads from a stream and processes log lines
func (w *workerLogCollector) streamReader(stream io.Reader, streamType StreamType) {
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
			WorkerID:  w.workerID,
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
func (w *workerLogCollector) processLogLine(line string, metadata LogMetadata) error {
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
		WorkerID:  metadata.WorkerID,
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
		WorkerID:  rawEntry.WorkerID,
		Stream:    rawEntry.Stream,
		Raw:       rawEntry.Line,
	}

	// Write to outputs (Phase 1: Basic file output)
	return w.writeToOutputs(logEntry)
}

// writeToOutputs writes the log entry to configured outputs
func (w *workerLogCollector) writeToOutputs(entry LogEntry) error {
	// Write to global aggregated outputs
	for _, output := range w.service.outputs {
		if err := output.Write(entry); err != nil {
			w.logger.WithError(err).Warnf("Failed to write to global output")
			w.recordError(fmt.Sprintf("Global output write error: %v", err))
		}
	}

	// Note: Worker-specific log files planned for Phase 4
	// Feature: Individual log files per worker with configurable rotation and retention

	return nil
}

// stop stops the worker log collector
func (w *workerLogCollector) stop() error {
	close(w.stopCh)
	w.wg.Wait()

	w.mu.Lock()
	w.active = false
	w.mu.Unlock()

	return nil
}

// getStatus returns the current status of the worker
func (w *workerLogCollector) getStatus() *WorkerLogStatus {
	w.mu.RLock()
	defer w.mu.RUnlock()

	// Copy errors to avoid data races
	errorsCopy := make([]string, len(w.errors))
	copy(errorsCopy, w.errors)

	return &WorkerLogStatus{
		WorkerID:       w.workerID,
		Active:         w.active,
		LinesProcessed: w.linesProcessed,
		BytesProcessed: w.bytesProcessed,
		LastActivity:   w.lastActivity,
		Errors:         errorsCopy,
		Config:         w.config,
	}
}

// recordError records an error for status reporting
func (w *workerLogCollector) recordError(errMsg string) {
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
		entry.WorkerID,
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
		entry.WorkerID,
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
