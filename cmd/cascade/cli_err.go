package main

import "fmt"

// Error types for structured error handling
type CLIError struct {
	Code    int
	Message string
	Cause   error
}

func (e *CLIError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

func (e *CLIError) ExitCode() int {
	return e.Code
}

// Error creation helpers for structured error handling

func newConfigError(message string, cause error) *CLIError {
	return &CLIError{Code: ExitConfigError, Message: message, Cause: cause}
}

func newValidationError(message string, cause error) *CLIError {
	return &CLIError{Code: ExitValidationError, Message: message, Cause: cause}
}

func newFileError(message string, cause error) *CLIError {
	return &CLIError{Code: ExitFileError, Message: message, Cause: cause}
}

func newStateError(message string, cause error) *CLIError {
	return &CLIError{Code: ExitStateError, Message: message, Cause: cause}
}

func newPlanningError(message string, cause error) *CLIError {
	return &CLIError{Code: ExitPlanningError, Message: message, Cause: cause}
}

func newExecutionError(message string, cause error) *CLIError {
	return &CLIError{Code: ExitExecutionError, Message: message, Cause: cause}
}
