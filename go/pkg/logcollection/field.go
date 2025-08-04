package logcollection

import (
	"fmt"
	"time"
)

// ===== FIELD TYPES (COMPLETE BACKEND HIDING) =====

// LogField represents a structured log field - completely independent of any backend
type LogField struct {
	Key   string
	Value interface{}
	Type  FieldType
}

// FieldType identifies how the field should be processed
type FieldType int

const (
	StringField FieldType = iota
	IntField
	Int64Field
	Float64Field
	BoolField
	DurationField
	TimeField
	ErrorField
	ObjectField
	ArrayField
)

// String returns a string representation of the field type
func (ft FieldType) String() string {
	switch ft {
	case StringField:
		return "string"
	case IntField:
		return "int"
	case Int64Field:
		return "int64"
	case Float64Field:
		return "float64"
	case BoolField:
		return "bool"
	case DurationField:
		return "duration"
	case TimeField:
		return "time"
	case ErrorField:
		return "error"
	case ObjectField:
		return "object"
	case ArrayField:
		return "array"
	default:
		return "unknown"
	}
}

// ===== FIELD CONSTRUCTORS =====

// String creates a string field
func String(key, value string) LogField {
	return LogField{Key: key, Value: value, Type: StringField}
}

// Int creates an integer field
func Int(key string, value int) LogField {
	return LogField{Key: key, Value: value, Type: IntField}
}

// Int64 creates an int64 field
func Int64(key string, value int64) LogField {
	return LogField{Key: key, Value: value, Type: Int64Field}
}

// Float64 creates a float64 field
func Float64(key string, value float64) LogField {
	return LogField{Key: key, Value: value, Type: Float64Field}
}

// Bool creates a boolean field
func Bool(key string, value bool) LogField {
	return LogField{Key: key, Value: value, Type: BoolField}
}

// Duration creates a duration field
func Duration(key string, value time.Duration) LogField {
	return LogField{Key: key, Value: value, Type: DurationField}
}

// Time creates a time field
func Time(key string, value time.Time) LogField {
	return LogField{Key: key, Value: value, Type: TimeField}
}

// Error creates an error field (always uses "error" as key)
func Error(err error) LogField {
	return LogField{Key: "error", Value: err, Type: ErrorField}
}

// ErrorWithKey creates an error field with custom key
func ErrorWithKey(key string, err error) LogField {
	return LogField{Key: key, Value: err, Type: ErrorField}
}

// Object creates an object field for complex types
func Object(key string, value interface{}) LogField {
	return LogField{Key: key, Value: value, Type: ObjectField}
}

// Array creates an array field
func Array(key string, value interface{}) LogField {
	return LogField{Key: key, Value: value, Type: ArrayField}
}

// ===== WORKER-SPECIFIC CONVENIENCE FIELDS =====

// Worker creates a worker_id field
func Worker(workerID string) LogField {
	return String("worker_id", workerID)
}

// Stream creates a stream field
func Stream(stream StreamType) LogField {
	return String("stream", string(stream))
}

// Component creates a component field
func Component(component string) LogField {
	return String("component", component)
}

// Operation creates an operation field
func Operation(operation string) LogField {
	return String("operation", operation)
}

// RequestID creates a request_id field
func RequestID(requestID string) LogField {
	return String("request_id", requestID)
}

// PID creates a process ID field
func PID(pid int) LogField {
	return Int("pid", pid)
}

// ===== FIELD UTILITIES =====

// ToMap converts a slice of LogFields to a map
func ToMap(fields []LogField) map[string]interface{} {
	result := make(map[string]interface{}, len(fields))
	for _, field := range fields {
		result[field.Key] = field.Value
	}
	return result
}

// FromMap converts a map to a slice of LogFields
func FromMap(m map[string]interface{}) []LogField {
	fields := make([]LogField, 0, len(m))
	for key, value := range m {
		fields = append(fields, inferField(key, value))
	}
	return fields
}

// inferField attempts to infer the field type from the value
func inferField(key string, value interface{}) LogField {
	switch v := value.(type) {
	case string:
		return String(key, v)
	case int:
		return Int(key, v)
	case int64:
		return Int64(key, v)
	case float64:
		return Float64(key, v)
	case bool:
		return Bool(key, v)
	case time.Duration:
		return Duration(key, v)
	case time.Time:
		return Time(key, v)
	case error:
		return ErrorWithKey(key, v)
	default:
		return Object(key, v)
	}
}

// ===== FIELD VALIDATION =====

// Validate checks if the field has valid key and value
func (f LogField) Validate() error {
	if f.Key == "" {
		return fmt.Errorf("field key cannot be empty")
	}

	if f.Value == nil {
		return fmt.Errorf("field value cannot be nil for key %q", f.Key)
	}

	// Type-specific validation
	switch f.Type {
	case ErrorField:
		if _, ok := f.Value.(error); !ok {
			return fmt.Errorf("error field %q must have error value, got %T", f.Key, f.Value)
		}
	case TimeField:
		if _, ok := f.Value.(time.Time); !ok {
			return fmt.Errorf("time field %q must have time.Time value, got %T", f.Key, f.Value)
		}
	case DurationField:
		if _, ok := f.Value.(time.Duration); !ok {
			return fmt.Errorf("duration field %q must have time.Duration value, got %T", f.Key, f.Value)
		}
	}

	return nil
}

// String returns a string representation of the field
func (f LogField) String() string {
	return fmt.Sprintf("%s=%v", f.Key, f.Value)
}

// ===== FIELD COLLECTIONS =====

// Fields is a convenient type for building field collections
type Fields []LogField

// Add appends a field to the collection
func (f *Fields) Add(field LogField) *Fields {
	*f = append(*f, field)
	return f
}

// AddString adds a string field
func (f *Fields) AddString(key, value string) *Fields {
	return f.Add(String(key, value))
}

// AddInt adds an integer field
func (f *Fields) AddInt(key string, value int) *Fields {
	return f.Add(Int(key, value))
}

// AddError adds an error field
func (f *Fields) AddError(err error) *Fields {
	return f.Add(Error(err))
}

// AddWorker adds a worker field
func (f *Fields) AddWorker(workerID string) *Fields {
	return f.Add(Worker(workerID))
}

// ToSlice returns the fields as a slice
func (f Fields) ToSlice() []LogField {
	return []LogField(f)
}

// NewFields creates a new field collection
func NewFields() *Fields {
	return &Fields{}
}
