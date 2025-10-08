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
	detector := newTestDetector(t, "diff_basic.txt")

	deltas, err := detector.Detect(context.Background(), "refs/base", "refs/head")
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
	detector := newTestDetector(t, "diff_replace.txt")

	deltas, err := detector.Detect(context.Background(), "refs/base", "refs/head")
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
	detector := newTestDetector(t, "diff_multimodule.txt")

	deltas, err := detector.Detect(context.Background(), "refs/base", "refs/head")
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

func newTestDetector(t *testing.T, diffFixture string) depregistration.Detector {
	t.Helper()

	data, err := testsupport.LoadFixture(fixturePath(diffFixture))
	if err != nil {
		t.Fatalf("load fixture %s: %v", diffFixture, err)
	}

	fetcher := &staticDiffFetcher{data: data}

	return depregistration.NewDiffDetector(fetcher)
}

type staticDiffFetcher struct {
	data []byte
}

func (f *staticDiffFetcher) Diff(ctx context.Context, baseRef, headRef string) ([]byte, error) {
	return append([]byte(nil), f.data...), nil
}

func goldenPath(name string) string {
	return filepath.Join("testdata", name)
}

func fixturePath(name string) string {
	return filepath.Join("testdata", name)
}
