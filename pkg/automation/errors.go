package automation

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrInvalidWorkflow = errors.New("invalid automation workflow")
	ErrWatchFailed     = errors.New("watch failed")
	ErrExecutionFailed = errors.New("execution failed")
)

type ValidationError struct {
	Problems []string
}

func (e *ValidationError) Error() string {
	if e == nil || len(e.Problems) == 0 {
		return ErrInvalidWorkflow.Error()
	}
	return fmt.Sprintf("%s: %s", ErrInvalidWorkflow, strings.Join(e.Problems, "; "))
}

func (e *ValidationError) Unwrap() error {
	return ErrInvalidWorkflow
}

func newValidationError(problems []string) error {
	if len(problems) == 0 {
		return nil
	}
	return &ValidationError{Problems: problems}
}
