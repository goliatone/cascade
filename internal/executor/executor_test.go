package executor_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/goliatone/cascade/internal/executor"
	"github.com/goliatone/cascade/internal/manifest"
	"github.com/goliatone/cascade/internal/planner"
)

func TestExecutor_ApplyProducesExpectedResult(t *testing.T) {
	t.Skip("executor implementation pending")

	loader := manifest.NewLoader()
	m, err := loader.Load(filepath.Join("..", "planner", "testdata", "basic.yaml"))
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	p := planner.New()
	plan, err := p.Plan(context.Background(), m, planner.Target{Module: "github.com/goliatone/go-errors", Version: "v1.2.3"})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	exec := executor.New()
	_, err = exec.Apply(context.Background(), executor.WorkItemContext{Item: plan.Items[0]})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
}
