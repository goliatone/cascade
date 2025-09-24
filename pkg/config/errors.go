package config

import "fmt"

// NotImplementedError signals unfinished configuration behaviours.
type NotImplementedError struct {
	Operation string
}

func (e *NotImplementedError) Error() string {
	return fmt.Sprintf("config: %s not implemented", e.Operation)
}

// newNotImplemented returns a typed not implemented error helper.
func newNotImplemented(op string) error {
	return &NotImplementedError{Operation: op}
}
