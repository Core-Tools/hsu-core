package errors

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDomainError_Creation(t *testing.T) {
	cause := errors.New("underlying error")

	err := NewValidationError("test validation error", cause)

	assert.Equal(t, ErrorTypeValidation, err.Type)
	assert.Equal(t, "test validation error", err.Message)
	assert.Equal(t, cause, err.Cause)
	assert.NotNil(t, err.Context)
}

func TestDomainError_WithContext(t *testing.T) {
	err := NewProcessError("test error", nil)

	err = err.WithContext("worker_id", "test-worker")
	err = err.WithContext("pid", 12345)

	assert.Equal(t, "test-worker", err.Context["worker_id"])
	assert.Equal(t, 12345, err.Context["pid"])
}

func TestDomainError_ErrorMessage(t *testing.T) {
	tests := []struct {
		name     string
		error    *DomainError
		expected string
	}{
		{
			name:     "error without cause",
			error:    NewValidationError("test message", nil),
			expected: "validation: test message",
		},
		{
			name:     "error with cause",
			error:    NewProcessError("test message", errors.New("cause")),
			expected: "process: test message: cause",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.error.Error())
		})
	}
}

func TestDomainError_TypeChecking(t *testing.T) {
	validationErr := NewValidationError("validation error", nil)
	processErr := NewProcessError("process error", nil)

	// Test type checking functions
	assert.True(t, IsValidationError(validationErr))
	assert.False(t, IsValidationError(processErr))

	assert.True(t, IsProcessError(processErr))
	assert.False(t, IsProcessError(validationErr))

	// Test with wrapped errors
	wrappedErr := errors.New("wrapped")
	assert.False(t, IsValidationError(wrappedErr))
}

func TestDomainError_Unwrap(t *testing.T) {
	cause := errors.New("underlying error")
	err := NewProcessError("test error", cause)

	unwrapped := errors.Unwrap(err)
	assert.Equal(t, cause, unwrapped)
}

func TestErrorCollection(t *testing.T) {
	collection := NewErrorCollection()

	// Test empty collection
	assert.False(t, collection.HasErrors())
	assert.Nil(t, collection.ToError())

	// Add some errors
	collection.Add(NewValidationError("error 1", nil))
	collection.Add(NewProcessError("error 2", nil))
	collection.Add(nil) // Should be ignored

	assert.True(t, collection.HasErrors())
	assert.Equal(t, 2, len(collection.Errors))

	// Test error message
	err := collection.ToError()
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "2 errors occurred")
}

func TestErrorCollection_SingleError(t *testing.T) {
	collection := NewErrorCollection()
	collection.Add(NewValidationError("single error", nil))

	err := collection.ToError()
	require.NotNil(t, err)
	assert.Equal(t, "validation: single error", err.Error())
}

func TestAllErrorTypes(t *testing.T) {
	// Test that all error type constructors work
	errorTypes := []struct {
		name        string
		constructor func(string, error) *DomainError
		checker     func(error) bool
		errorType   ErrorType
	}{
		{"validation", NewValidationError, IsValidationError, ErrorTypeValidation},
		{"not_found", NewNotFoundError, IsNotFoundError, ErrorTypeNotFound},
		{"conflict", NewConflictError, IsConflictError, ErrorTypeConflict},
		{"process", NewProcessError, IsProcessError, ErrorTypeProcess},
		{"discovery", NewDiscoveryError, IsDiscoveryError, ErrorTypeDiscovery},
		{"health_check", NewHealthCheckError, IsHealthCheckError, ErrorTypeHealthCheck},
		{"timeout", NewTimeoutError, IsTimeoutError, ErrorTypeTimeout},
		{"permission", NewPermissionError, IsPermissionError, ErrorTypePermission},
		{"io", NewIOError, IsIOError, ErrorTypeIO},
		{"network", NewNetworkError, IsNetworkError, ErrorTypeNetwork},
		{"internal", NewInternalError, IsInternalError, ErrorTypeInternal},
		{"cancelled", NewCancelledError, IsCancelledError, ErrorTypeCancelled},
	}

	for _, tt := range errorTypes {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.constructor("test message", nil)
			assert.Equal(t, tt.errorType, err.Type)
			assert.True(t, tt.checker(err))
		})
	}
}
