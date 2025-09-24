package executor

import "context"

// goOperations implements GoOperations using the system go tool.
type goOperations struct{}

// NewGoOperations creates a stub GoOperations implementation.
func NewGoOperations() GoOperations {
	return &goOperations{}
}

func (g *goOperations) Get(ctx context.Context, repoPath, module, version string) error {
	return &NotImplementedError{Operation: "go.Get"}
}

func (g *goOperations) Tidy(ctx context.Context, repoPath string) error {
	return &NotImplementedError{Operation: "go.Tidy"}
}
