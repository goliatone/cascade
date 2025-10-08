package depregistration_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/goliatone/cascade/internal/depregistration"
	"github.com/goliatone/cascade/pkg/testsupport"
)

func TestDetectorContract_Basic(t *testing.T) {
	t.Skip("pending detector implementation")

	detector := requireDetector(t)

	ctx := context.Background()

	if _, err := testsupport.LoadFixture(fixturePath("diff_basic.txt")); err != nil {
		t.Fatalf("load diff fixture: %v", err)
	}

	deltas, err := detector.Detect(ctx, "refs/base", "refs/head")
	if err != nil {
		t.Fatalf("detect: %v", err)
	}

	var want []depregistration.DependencyDelta
	if err := testsupport.LoadGolden(goldenPath("diff_basic.golden.json"), &want); err != nil {
		t.Fatalf("load golden: %v", err)
	}

	if diff := cmp.Diff(want, deltas); diff != "" {
		t.Fatalf("unexpected diff (-want +got):\n%s", diff)
	}
}

func TestDetectorContract_ReplaceIgnored(t *testing.T) {
	t.Skip("pending detector implementation")

	detector := requireDetector(t)

	ctx := context.Background()

	if _, err := testsupport.LoadFixture(fixturePath("diff_replace.txt")); err != nil {
		t.Fatalf("load diff fixture: %v", err)
	}

	deltas, err := detector.Detect(ctx, "refs/base", "refs/head")
	if err != nil {
		t.Fatalf("detect: %v", err)
	}

	var want []depregistration.DependencyDelta
	if err := testsupport.LoadGolden(goldenPath("diff_replace.golden.json"), &want); err != nil {
		t.Fatalf("load golden: %v", err)
	}

	if diff := cmp.Diff(want, deltas); diff != "" {
		t.Fatalf("unexpected diff (-want +got):\n%s", diff)
	}
}

func TestDetectorContract_MultiModule(t *testing.T) {
	t.Skip("pending detector implementation")

	detector := requireDetector(t)

	ctx := context.Background()

	if _, err := testsupport.LoadFixture(fixturePath("diff_multimodule.txt")); err != nil {
		t.Fatalf("load diff fixture: %v", err)
	}

	deltas, err := detector.Detect(ctx, "refs/base", "refs/head")
	if err != nil {
		t.Fatalf("detect: %v", err)
	}

	var want []depregistration.DependencyDelta
	if err := testsupport.LoadGolden(goldenPath("diff_multimodule.golden.json"), &want); err != nil {
		t.Fatalf("load golden: %v", err)
	}

	if diff := cmp.Diff(want, deltas); diff != "" {
		t.Fatalf("unexpected diff (-want +got):\n%s", diff)
	}
}

func requireDetector(t *testing.T) depregistration.Detector {
	t.Helper()
	t.Fatalf("detector implementation not wired for contract tests")
	return nil
}

func goldenPath(name string) string {
	return filepath.Join("testdata", name)
}

func fixturePath(name string) string {
	return filepath.Join("testdata", name)
}
