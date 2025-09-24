package planner

import (
	"errors"
	"fmt"
)

// NotImplementedError signals unimplemented planner behaviour.
type NotImplementedError struct {
	Reason string
}

func (e *NotImplementedError) Error() string {
	return fmt.Sprintf("planner: %s", e.Reason)
}

// TargetNotFoundError is returned when the target module cannot be found in the manifest.
type TargetNotFoundError struct {
	ModuleName string
}

func (e *TargetNotFoundError) Error() string {
	return fmt.Sprintf("planner: target module not found: %s", e.ModuleName)
}

// InvalidTargetError is returned when target fields are invalid or empty.
type InvalidTargetError struct {
	Field string
}

func (e *InvalidTargetError) Error() string {
	return fmt.Sprintf("planner: invalid target: %s field is empty", e.Field)
}

// PlanningError wraps generic planning failures with target context.
type PlanningError struct {
	Target Target
	Err    error
}

func (e *PlanningError) Error() string {
	return fmt.Sprintf("planner: planning failed for %s@%s: %v", e.Target.Module, e.Target.Version, e.Err)
}

func (e *PlanningError) Unwrap() error {
	return e.Err
}

// Helper predicates for error detection using errors.As

// IsTargetNotFound returns true if err is a TargetNotFoundError.
func IsTargetNotFound(err error) bool {
	var target *TargetNotFoundError
	return errors.As(err, &target)
}

// IsInvalidTarget returns true if err is an InvalidTargetError.
func IsInvalidTarget(err error) bool {
	var invalid *InvalidTargetError
	return errors.As(err, &invalid)
}

// IsPlanningError returns true if err is a PlanningError.
func IsPlanningError(err error) bool {
	var planning *PlanningError
	return errors.As(err, &planning)
}
