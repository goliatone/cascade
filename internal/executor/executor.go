package executor

import "context"

// New returns a stub executor implementation.
func New() Executor {
	return &executor{}
}

type executor struct{}

func (e *executor) Apply(ctx context.Context, input WorkItemContext) (*Result, error) {
	return nil, &NotImplementedError{Operation: "executor.Apply"}
}
