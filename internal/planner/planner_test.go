package planner_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/goliatone/cascade/internal/manifest"
	"github.com/goliatone/cascade/internal/planner"
	"github.com/goliatone/cascade/pkg/testsupport"
)

func TestPlanner_PlanProducesExpectedWorkItems(t *testing.T) {

	loader := manifest.NewLoader()
	m, err := loader.Load(filepath.Join("..", "manifest", "testdata", "basic.yaml"))
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	target := planner.Target{Module: "github.com/goliatone/go-errors", Version: "v1.2.3"}

	p := planner.New()
	plan, err := p.Plan(context.Background(), m, target)
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}

	var want planner.Plan
	if err := testsupport.LoadGolden(filepath.Join("testdata", "basic_plan.json"), &want); err != nil {
		t.Fatalf("load golden: %v", err)
	}

	if !reflect.DeepEqual(plan, &want) {
		gotJSON, _ := json.MarshalIndent(plan, "", "  ")
		wantJSON, _ := json.MarshalIndent(want, "", "  ")
		t.Fatalf("plan mismatch\n got: %s\nwant: %s", gotJSON, wantJSON)
	}
}
