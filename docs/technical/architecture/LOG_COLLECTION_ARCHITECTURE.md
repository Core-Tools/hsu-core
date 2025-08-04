# 🔥 **HSU Master Log Collection System - Architecture Design** 

## 🎯 **Design Philosophy: Smart Hybrid Approach**

**Vision**: Centralized log collection that handles the reality of external processes while providing modern structured logging capabilities for internal systems and external integrations.

---

## 🏗️ **Core Architecture Decisions**

### **✅ 1. Configurable Aggregation Strategy**

| Mode | Description | Use Case |
|------|-------------|----------|
| **`separate`** | Each worker's logs to dedicated outputs | Development, debugging, process-specific analysis |
| **`aggregate`** | All worker logs merged into master streams | Production monitoring, centralized dashboards |
| **`both`** | Dual output: separate files + aggregated stream | Best of both worlds, performance overhead |
| **`disabled`** | No log collection | Attached processes without file access |

### **✅ 2. Flexible Output Target Architecture**

```yaml
# Example: Comprehensive output configuration
workers:
  - id: "web-server"
    type: "managed"
    unit:
      managed:
        control:
          log_collection:
            enabled: true
            capture_stdout: true
            capture_stderr: true
            aggregation:
              mode: "both"        # separate + aggregate
              targets:
                aggregate:
                  - type: "master_stdout"
                    format: "enhanced_plain"
                    prefix: "[{worker_id}]"
                  - type: "file"
                    path: "/var/log/hsu-master/aggregated.log"
                    format: "structured_json"
                  - type: "elasticsearch"
                    endpoint: "http://localhost:9200"
                    index: "hsu-master-logs"
                separate:
                  stdout:
                    - type: "file"  
                      path: "/var/log/hsu-master/workers/{worker_id}-stdout.log"
                      format: "enhanced_plain"
                  stderr:
                    - type: "file"
                      path: "/var/log/hsu-master/workers/{worker_id}-stderr.log"
                      format: "enhanced_plain"
                    - type: "syslog"
                      facility: "local0"
                      severity: "error"
```

### **✅ 3. Recommended Directory Structure**

```
/var/log/hsu-master/
├── master.log                    # Master process logs
├── aggregated.log                # All worker logs combined (plain)
├── aggregated.json               # All worker logs combined (structured)
└── workers/
    ├── web-server-stdout.log     # Individual worker stdout
    ├── web-server-stderr.log     # Individual worker stderr
    ├── web-server-combined.log   # Both streams combined
    ├── database-service-stdout.log
    └── api-gateway-stderr.log
```

---

## 🎭 **Backend Library Abstraction - Complete Hiding Strategy**

### **🔧 The Interface Design Challenge**

**Question**: Can we completely hide backend libraries (zap, logrus, etc.) behind our interface?

**Answer**: ✅ **YES - With Proper Abstraction Design!**

### **❌ Problem: Leaky Abstraction**
```go
// BAD: This exposes zap.Field, breaking abstraction
type StructuredLogger interface {
    LogWithContext(ctx context.Context, level LogLevel, msg string, fields ...zap.Field) // ❌ Exposes zap
}
```

### **✅ Solution: Complete Backend Hiding**

```go
// ===== CLEAN ABSTRACTION LAYER =====

// Our own field type - completely independent of backend
type LogField struct {
    Key   string
    Value interface{}
    Type  FieldType
}

type FieldType int

const (
    StringField FieldType = iota
    IntField
    BoolField
    DurationField
    ErrorField
    TimeField
    FloatField
    ObjectField
)

// Convenient field constructors (inspired by zap but independent)
func StringField(key, value string) LogField {
    return LogField{Key: key, Value: value, Type: StringField}
}

func IntField(key string, value int) LogField {
    return LogField{Key: key, Value: value, Type: IntField}
}

func ErrorField(err error) LogField {
    return LogField{Key: "error", Value: err, Type: ErrorField}
}

func DurationField(key string, duration time.Duration) LogField {
    return LogField{Key: key, Value: duration, Type: DurationField}
}

// Worker-specific convenience methods
func WorkerField(workerID string) LogField {
    return StringField("worker_id", workerID)
}

func StreamField(stream string) LogField {
    return StringField("stream", stream)
}

// ===== COMPLETELY HIDDEN BACKEND INTERFACE =====

type LogLevel int

const (
    DebugLevel LogLevel = iota
    InfoLevel
    WarnLevel
    ErrorLevel
)

// Clean interface - no backend library types exposed
type StructuredLogger interface {
    // Simple logging (backwards compatible)
    Debugf(format string, args ...interface{})
    Infof(format string, args ...interface{})
    Warnf(format string, args ...interface{})
    Errorf(format string, args ...interface{})
    
    // Structured logging with our own types
    LogWithContext(ctx context.Context, level LogLevel, msg string, fields ...LogField)
    LogWithFields(level LogLevel, msg string, fields ...LogField)
    
    // Fluent interface for building context
    WithFields(fields ...LogField) StructuredLogger
    WithError(err error) StructuredLogger
    WithWorker(workerID string) StructuredLogger
    WithContext(ctx context.Context) StructuredLogger
}

// Log collection specific interface
type LogCollector interface {
    CollectFromStream(workerID string, stream io.Reader, streamType StreamType) error
    ProcessLogLine(workerID string, line string, metadata LogMetadata) error
    ForwardLogs(targets []LogOutputTarget) error
    
    // Structured log processing
    EnhanceLogEntry(entry RawLogEntry) EnhancedLogEntry
    ParseStructuredLog(line string) (*StructuredLogEntry, error)
}
```

### **🔄 Backend Adapter Pattern**

```go
// ===== BACKEND ADAPTERS (HIDDEN FROM USERS) =====

// Convert our fields to zap fields (internal conversion)
type ZapAdapter struct {
    logger *zap.Logger
}

func (z *ZapAdapter) convertFields(fields []LogField) []zap.Field {
    zapFields := make([]zap.Field, len(fields))
    for i, field := range fields {
        switch field.Type {
        case StringField:
            zapFields[i] = zap.String(field.Key, field.Value.(string))
        case IntField:
            zapFields[i] = zap.Int(field.Key, field.Value.(int))
        case ErrorField:
            zapFields[i] = zap.Error(field.Value.(error))
        case DurationField:
            zapFields[i] = zap.Duration(field.Key, field.Value.(time.Duration))
        case BoolField:
            zapFields[i] = zap.Bool(field.Key, field.Value.(bool))
        case TimeField:
            zapFields[i] = zap.Time(field.Key, field.Value.(time.Time))
        case FloatField:
            zapFields[i] = zap.Float64(field.Key, field.Value.(float64))
        case ObjectField:
            zapFields[i] = zap.Any(field.Key, field.Value)
        }
    }
    return zapFields
}

func (z *ZapAdapter) LogWithFields(level LogLevel, msg string, fields ...LogField) {
    zapFields := z.convertFields(fields)
    
    switch level {
    case DebugLevel:
        z.logger.Debug(msg, zapFields...)
    case InfoLevel:
        z.logger.Info(msg, zapFields...)
    case WarnLevel:
        z.logger.Warn(msg, zapFields...)
    case ErrorLevel:
        z.logger.Error(msg, zapFields...)
    }
}

// Similar adapters for other backends
type LogrusAdapter struct {
    logger *logrus.Logger
}

type SlogAdapter struct {
    logger *slog.Logger
}
```

### **🏭 Factory Pattern for Backend Selection**

```go
// ===== BACKEND FACTORY (COMPLETELY HIDDEN) =====

type LoggerBackend string

const (
    ZapBackend    LoggerBackend = "zap"
    LogrusBackend LoggerBackend = "logrus"
    SlogBackend   LoggerBackend = "slog"
)

type LoggerConfig struct {
    Backend LoggerBackend `yaml:"backend"`
    Level   LogLevel      `yaml:"level"`
    Format  string        `yaml:"format"` // "json", "console"
    Output  string        `yaml:"output"` // "stdout", "file"
}

// Factory function - users never see the backend types
func NewStructuredLogger(config LoggerConfig) StructuredLogger {
    switch config.Backend {
    case ZapBackend:
        zapLogger := createZapLogger(config) // Internal function
        return &ZapAdapter{logger: zapLogger}
    case LogrusBackend:
        logrusLogger := createLogrusLogger(config) // Internal function
        return &LogrusAdapter{logger: logrusLogger}
    case SlogBackend:
        slogLogger := createSlogLogger(config) // Internal function
        return &SlogAdapter{logger: slogLogger}
    default:
        // Default to zap
        zapLogger := createZapLogger(config)
        return &ZapAdapter{logger: zapLogger}
    }
}
```

---

## 🎯 **Usage Examples - Clean API**

### **✅ Simple Logging (Backwards Compatible)**
```go
logger := NewStructuredLogger(LoggerConfig{Backend: ZapBackend})

logger.Infof("Worker %s started successfully", workerID)
logger.Errorf("Failed to start worker %s: %v", workerID, err)
```

### **✅ Structured Logging (No Backend Exposure)**
```go
logger := NewStructuredLogger(LoggerConfig{Backend: ZapBackend})

// Using our own field types - no zap import needed!
logger.LogWithFields(InfoLevel, "Worker operation completed",
    WorkerField("web-server"),
    StringField("operation", "restart"),
    DurationField("duration", 2*time.Second),
    IntField("attempt", 3),
)

// Fluent interface
contextLogger := logger.
    WithWorker("database-service").
    WithFields(StringField("component", "health-check"))

contextLogger.LogWithFields(WarnLevel, "Health check failed",
    ErrorField(err),
    IntField("retry_count", 5),
)
```

### **✅ Log Collection Integration**
```go
// Log collector also uses clean interface
collector := NewLogCollector(LogCollectionConfig{
    Logger: logger,  // Any StructuredLogger implementation
    Targets: []LogOutputTarget{
        {Type: "file", Path: "/var/log/aggregated.log"},
        {Type: "elasticsearch", Endpoint: "http://localhost:9200"},
    },
})

// Process logs from workers
collector.ProcessLogLine("web-server", 
    "2025-01-20 10:30:45 INFO: Database connection established",
    LogMetadata{
        Timestamp: time.Now(),
        Stream:    "stdout",
        WorkerID:  "web-server",
    },
)
```

---

## 🔄 **Log Enhancement Pipeline**

### **📥 Input Processing**
```go
type RawLogEntry struct {
    WorkerID  string
    Stream    string // "stdout" | "stderr"
    Line      string
    Timestamp time.Time
}

type StructuredLogEntry struct {
    Timestamp time.Time              `json:"timestamp"`
    Level     string                 `json:"level"`
    Message   string                 `json:"message"`
    Fields    map[string]interface{} `json:"fields"`
    WorkerID  string                 `json:"worker_id"`
    Stream    string                 `json:"stream"`
}

type EnhancedLogEntry struct {
    Raw        RawLogEntry
    Structured *StructuredLogEntry // nil if parsing failed
    Enhanced   map[string]interface{} // Additional metadata
}
```

### **🔄 Processing Pipeline**
```go
type LogEnhancementPipeline struct {
    parsers   []LogParser
    enhancers []LogEnhancer
    filters   []LogFilter
}

func (p *LogEnhancementPipeline) Process(raw RawLogEntry) EnhancedLogEntry {
    enhanced := EnhancedLogEntry{Raw: raw}
    
    // 1. Try to parse structured content
    for _, parser := range p.parsers {
        if structured := parser.Parse(raw.Line); structured != nil {
            enhanced.Structured = structured
            break
        }
    }
    
    // 2. Add enhancement metadata
    enhanced.Enhanced = make(map[string]interface{})
    for _, enhancer := range p.enhancers {
        enhancer.Enhance(&enhanced)
    }
    
    // 3. Apply filters
    for _, filter := range p.filters {
        if !filter.ShouldProcess(enhanced) {
            enhanced.Enhanced["filtered"] = true
            break
        }
    }
    
    return enhanced
}
```

---

## 📊 **Configuration Schema**

### **🎛️ Master Configuration**
```yaml
master:
  logging:
    backend: "zap"           # "zap" | "logrus" | "slog"
    level: "info"            # "debug" | "info" | "warn" | "error"
    format: "json"           # "json" | "console"
    output: "/var/log/hsu-master/master.log"
  
  log_collection:
    enabled: true
    global_aggregation:
      enabled: true
      targets:
        - type: "file"
          path: "/var/log/hsu-master/aggregated.log"
          format: "enhanced_plain"
        - type: "file"
          path: "/var/log/hsu-master/aggregated.json"
          format: "structured_json"
    
    enhancement:
      enabled: true
      parsers:
        - type: "json"
        - type: "logfmt"
        - type: "timestamp_extraction"
      
      metadata:
        add_master_id: true
        add_hostname: true
        add_timestamp: true
```

### **🔧 Worker-Specific Configuration**
```yaml
workers:
  - id: "web-server"
    type: "managed"
    unit:
      managed:
        control:
          log_collection:
            enabled: true
            capture_stdout: true
            capture_stderr: true
            
            buffering:
              size: "1MB"
              flush_interval: "5s"
            
            processing:
              parse_structured: true
              add_metadata: true
              filters:
                exclude_patterns:
                  - "^DEBUG:"
                  - "health check.*OK"
                include_patterns:
                  - "ERROR:"
                  - "WARN:"
            
            outputs:
              separate:
                stdout:
                  - type: "file"
                    path: "/var/log/hsu-master/workers/web-server-stdout.log"
                    rotation:
                      max_size: "100MB"
                      max_files: 10
                      max_age: "7d"
                stderr:
                  - type: "file"
                    path: "/var/log/hsu-master/workers/web-server-stderr.log"
                  - type: "syslog"
                    facility: "local0"
                    severity: "error"
              
              forwarding:
                - type: "elasticsearch"
                  endpoint: "http://elasticsearch:9200"
                  index: "hsu-master-logs"
                  authentication:
                    type: "basic"
                    username: "admin"
                    password: "${ELASTICSEARCH_PASSWORD}"
```

---

## 🚀 **Implementation Phases**

### **Phase 1: Foundation (2-3 days)**
- ✅ Clean interface design with complete backend hiding
- ✅ **Process Control Integration** - Connect to worker stdout/stderr streams
- ✅ **Basic Stream Collection** - Real-time log streaming from managed processes
- ✅ Basic file output and aggregation (separate/aggregate modes)
- ✅ Zap adapter implementation
- ✅ Simple log enhancement (metadata addition)
- ✅ **Core LogCollector Service** - Central log collection coordination

### **Phase 2: Enhancement (1-2 days)**
- ✅ Structured log parsing (JSON, logfmt)
- ✅ Advanced filtering and routing
- ✅ Multiple output target support
- ✅ Log rotation and retention
- ✅ **File-based Log Monitoring** - Support for unmanaged/attached processes

### **Phase 3: Integration (1 day)**
- ✅ External system forwarding (Elasticsearch, Syslog)
- ✅ Performance optimization
- ✅ Configuration validation
- ✅ **Production Features** - Metrics, health monitoring, advanced error handling

---

## 🎯 **Key Benefits of This Architecture**

| Benefit | Description |
|---------|-------------|
| **🔒 Complete Abstraction** | Users never import zap/logrus - only our interfaces |
| **🔄 Backend Flexibility** | Can switch logging backends without code changes |
| **📈 Performance** | Efficient field conversion only when needed |
| **🎯 Backwards Compatibility** | Existing simple logging continues to work |
| **🚀 Future-Proof** | Easy to add new backends or enhance interfaces |
| **🧪 Testability** | Clean interfaces make mocking trivial |

## ✨ **Summary**

**YES** - We can completely hide backend libraries behind our interface! The key is:

1. **Our own field types** (`LogField` instead of `zap.Field`)
2. **Adapter pattern** for backend conversion (internal)
3. **Factory pattern** for backend selection (hidden)
4. **Clean interfaces** with no external dependencies

Users import only our logging package and never see zap, logrus, or any other backend library. The abstraction is complete and the API is clean! 🎉 