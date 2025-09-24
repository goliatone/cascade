package manifest

import (
	"errors"
	"fmt"
)

// LoadError represents an error that occurred during manifest loading.
type LoadError struct {
	Path string
	Err  error
}

func (e *LoadError) Error() string {
	return fmt.Sprintf("manifest: failed to load %s: %v", e.Path, e.Err)
}

func (e *LoadError) Unwrap() error {
	return e.Err
}

// ParseError represents an error that occurred during YAML parsing.
type ParseError struct {
	Path string
	Err  error
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("manifest: failed to parse YAML from %s: %v", e.Path, e.Err)
}

func (e *ParseError) Unwrap() error {
	return e.Err
}

// ModuleNotFoundError is returned when a module cannot be found.
type ModuleNotFoundError struct {
	ModuleName string
}

func (e *ModuleNotFoundError) Error() string {
	return fmt.Sprintf("manifest: module not found: %s", e.ModuleName)
}

// ValidationError aggregates multiple validation issues.
type ValidationError struct {
	Issues []string
}

func (e *ValidationError) Error() string {
	if len(e.Issues) == 1 {
		return fmt.Sprintf("manifest: validation failed: %s", e.Issues[0])
	}
	return fmt.Sprintf("manifest: validation failed with %d issues:\n- %s", len(e.Issues), joinWithPrefix(e.Issues, "\n- "))
}

// GenerateError represents an error from the Generate operation.
type GenerateError struct {
	WorkDir string
	Reason  string
}

func (e *GenerateError) Error() string {
	return fmt.Sprintf("manifest: failed to generate for %s: %s", e.WorkDir, e.Reason)
}

// Helper functions for error detection

// IsLoadError returns true if the error is a LoadError.
func IsLoadError(err error) bool {
	var loadErr *LoadError
	return errors.As(err, &loadErr)
}

// IsParseError returns true if the error is a ParseError.
func IsParseError(err error) bool {
	var parseErr *ParseError
	return errors.As(err, &parseErr)
}

// IsModuleNotFound returns true if the error is a ModuleNotFoundError.
func IsModuleNotFound(err error) bool {
	var moduleErr *ModuleNotFoundError
	return errors.As(err, &moduleErr)
}

// IsValidationError returns true if the error is a ValidationError.
func IsValidationError(err error) bool {
	var validationErr *ValidationError
	return errors.As(err, &validationErr)
}

// IsGenerateError returns true if the error is a GenerateError.
func IsGenerateError(err error) bool {
	var generateErr *GenerateError
	return errors.As(err, &generateErr)
}

// GetModuleName extracts the module name from a ModuleNotFoundError.
func GetModuleName(err error) (string, bool) {
	var moduleErr *ModuleNotFoundError
	if errors.As(err, &moduleErr) {
		return moduleErr.ModuleName, true
	}
	return "", false
}

// GetValidationIssues extracts the validation issues from a ValidationError.
func GetValidationIssues(err error) ([]string, bool) {
	var validationErr *ValidationError
	if errors.As(err, &validationErr) {
		return validationErr.Issues, true
	}
	return nil, false
}

// joinWithPrefix is a helper to join strings with a prefix for consistent formatting.
func joinWithPrefix(items []string, prefix string) string {
	if len(items) == 0 {
		return ""
	}
	result := items[0]
	for i := 1; i < len(items); i++ {
		result += prefix + items[i]
	}
	return result
}
