package errors

import (
	"errors"
	"fmt"
)

// Error types for better error classification and handling

// ErrorType represents different categories of errors
type ErrorType string

const (
	ErrorTypeValidation  ErrorType = "validation"
	ErrorTypeNotFound    ErrorType = "not_found"
	ErrorTypeConflict    ErrorType = "conflict"
	ErrorTypeProcess     ErrorType = "process"
	ErrorTypeDiscovery   ErrorType = "discovery"
	ErrorTypeHealthCheck ErrorType = "health_check"
	ErrorTypeTimeout     ErrorType = "timeout"
	ErrorTypePermission  ErrorType = "permission"
	ErrorTypeIO          ErrorType = "io"
	ErrorTypeNetwork     ErrorType = "network"
	ErrorTypeInternal    ErrorType = "internal"
	ErrorTypeCancelled   ErrorType = "cancelled"
)

// DomainError represents a structured error with type and context
type DomainError struct {
	Type    ErrorType
	Message string
	Cause   error
	Context map[string]interface{}
}

func (e *DomainError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Type, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Type, e.Message)
}

func (e *DomainError) Unwrap() error {
	return e.Cause
}

// Is checks if the error is of a specific type
func (e *DomainError) Is(target error) bool {
	if other, ok := target.(*DomainError); ok {
		return e.Type == other.Type
	}
	return false
}

// WithContext adds context information to the error
func (e *DomainError) WithContext(key string, value interface{}) *DomainError {
	if e.Context == nil {
		e.Context = make(map[string]interface{})
	}
	e.Context[key] = value
	return e
}

// NewDomainError creates a new domain error
func NewDomainError(errorType ErrorType, message string, cause error) *DomainError {
	return &DomainError{
		Type:    errorType,
		Message: message,
		Cause:   cause,
		Context: make(map[string]interface{}),
	}
}

// Validation errors
func NewValidationError(message string, cause error) *DomainError {
	return NewDomainError(ErrorTypeValidation, message, cause)
}

func NewNotFoundError(message string, cause error) *DomainError {
	return NewDomainError(ErrorTypeNotFound, message, cause)
}

func NewConflictError(message string, cause error) *DomainError {
	return NewDomainError(ErrorTypeConflict, message, cause)
}

// Process errors
func NewProcessError(message string, cause error) *DomainError {
	return NewDomainError(ErrorTypeProcess, message, cause)
}

func NewDiscoveryError(message string, cause error) *DomainError {
	return NewDomainError(ErrorTypeDiscovery, message, cause)
}

func NewHealthCheckError(message string, cause error) *DomainError {
	return NewDomainError(ErrorTypeHealthCheck, message, cause)
}

// System errors
func NewTimeoutError(message string, cause error) *DomainError {
	return NewDomainError(ErrorTypeTimeout, message, cause)
}

func NewPermissionError(message string, cause error) *DomainError {
	return NewDomainError(ErrorTypePermission, message, cause)
}

func NewIOError(message string, cause error) *DomainError {
	return NewDomainError(ErrorTypeIO, message, cause)
}

func NewNetworkError(message string, cause error) *DomainError {
	return NewDomainError(ErrorTypeNetwork, message, cause)
}

func NewInternalError(message string, cause error) *DomainError {
	return NewDomainError(ErrorTypeInternal, message, cause)
}

func NewCancelledError(message string, cause error) *DomainError {
	return NewDomainError(ErrorTypeCancelled, message, cause)
}

// Error checking helpers
func IsValidationError(err error) bool {
	var domainErr *DomainError
	return errors.As(err, &domainErr) && domainErr.Type == ErrorTypeValidation
}

func IsNotFoundError(err error) bool {
	var domainErr *DomainError
	return errors.As(err, &domainErr) && domainErr.Type == ErrorTypeNotFound
}

func IsConflictError(err error) bool {
	var domainErr *DomainError
	return errors.As(err, &domainErr) && domainErr.Type == ErrorTypeConflict
}

func IsProcessError(err error) bool {
	var domainErr *DomainError
	return errors.As(err, &domainErr) && domainErr.Type == ErrorTypeProcess
}

func IsDiscoveryError(err error) bool {
	var domainErr *DomainError
	return errors.As(err, &domainErr) && domainErr.Type == ErrorTypeDiscovery
}

func IsHealthCheckError(err error) bool {
	var domainErr *DomainError
	return errors.As(err, &domainErr) && domainErr.Type == ErrorTypeHealthCheck
}

func IsTimeoutError(err error) bool {
	var domainErr *DomainError
	return errors.As(err, &domainErr) && domainErr.Type == ErrorTypeTimeout
}

func IsPermissionError(err error) bool {
	var domainErr *DomainError
	return errors.As(err, &domainErr) && domainErr.Type == ErrorTypePermission
}

func IsIOError(err error) bool {
	var domainErr *DomainError
	return errors.As(err, &domainErr) && domainErr.Type == ErrorTypeIO
}

func IsNetworkError(err error) bool {
	var domainErr *DomainError
	return errors.As(err, &domainErr) && domainErr.Type == ErrorTypeNetwork
}

func IsInternalError(err error) bool {
	var domainErr *DomainError
	return errors.As(err, &domainErr) && domainErr.Type == ErrorTypeInternal
}

func IsCancelledError(err error) bool {
	var domainErr *DomainError
	return errors.As(err, &domainErr) && domainErr.Type == ErrorTypeCancelled
}

// Error aggregation for bulk operations
type ErrorCollection struct {
	Errors []error
}

func (e *ErrorCollection) Error() string {
	if len(e.Errors) == 0 {
		return "no errors"
	}
	if len(e.Errors) == 1 {
		return e.Errors[0].Error()
	}
	return fmt.Sprintf("%d errors occurred: %v", len(e.Errors), e.Errors[0])
}

func (e *ErrorCollection) Add(err error) {
	if err != nil {
		e.Errors = append(e.Errors, err)
	}
}

func (e *ErrorCollection) HasErrors() bool {
	return len(e.Errors) > 0
}

func (e *ErrorCollection) ToError() error {
	if !e.HasErrors() {
		return nil
	}
	return e
}

// NewErrorCollection creates a new error collection
func NewErrorCollection() *ErrorCollection {
	return &ErrorCollection{
		Errors: make([]error, 0),
	}
}
