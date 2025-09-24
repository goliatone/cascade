package broker_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/goliatone/cascade/internal/broker"
	"github.com/goliatone/cascade/internal/executor"
	"github.com/goliatone/cascade/internal/manifest"
	"github.com/goliatone/cascade/internal/planner"
	"github.com/goliatone/cascade/pkg/testsupport"
)

func TestBroker_EnsurePRProducesExpectedPayload(t *testing.T) {
	loader := manifest.NewLoader()
	m, err := loader.Load(filepath.Join("..", "manifest", "testdata", "basic.yaml"))
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	plannerSvc := planner.New()
	plan, err := plannerSvc.Plan(context.Background(), m, planner.Target{Module: "github.com/goliatone/go-errors", Version: "v1.2.3"})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	execResult := &executor.Result{Status: executor.StatusCompleted}

	// Use stub implementation for consistent golden file output
	b := broker.NewStub()
	pr, err := b.EnsurePR(context.Background(), plan.Items[0], execResult)
	if err != nil {
		t.Fatalf("EnsurePR: %v", err)
	}

	got, err := json.MarshalIndent(pr, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// Add trailing newline to match golden file format
	got = append(got, '\n')

	wantBytes, err := testsupport.LoadFixture(filepath.Join("testdata", "basic_pr.json"))
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}

	if string(got) != string(wantBytes) {
		t.Fatalf("pull request mismatch\n got: %s\nwant: %s", got, string(wantBytes))
	}
}
