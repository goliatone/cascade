package planner

import "fmt"

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
