package manifest_test

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/goliatone/cascade/internal/manifest"
	"github.com/goliatone/cascade/pkg/testsupport"
)

func TestLoader_Load_GeneratesExpectedManifest(t *testing.T) {
	t.Skip("pending implementation")

	loader := manifest.NewLoader()
	manifestPath := filepath.Join("testdata", "basic.yaml")
	got, err := loader.Load(manifestPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	var want manifest.Manifest
	if err := testsupport.LoadGolden(filepath.Join("testdata", "basic_manifest.json"), &want); err != nil {
		t.Fatalf("load golden: %v", err)
	}

	if !reflect.DeepEqual(got, &want) {
		gotJSON, _ := json.MarshalIndent(got, "", "  ")
		wantJSON, _ := json.MarshalIndent(want, "", "  ")
		t.Fatalf("manifest mismatch\n got: %s\nwant: %s", gotJSON, wantJSON)
	}
}

func TestValidate_BasicManifest(t *testing.T) {
	t.Skip("pending implementation")

	var m manifest.Manifest
	if err := testsupport.LoadGolden(filepath.Join("testdata", "basic_manifest.json"), &m); err != nil {
		t.Fatalf("load golden: %v", err)
	}

	if err := manifest.Validate(&m); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
}

func TestFindModule_ReturnsMatch(t *testing.T) {
	t.Skip("pending implementation")

	var m manifest.Manifest
	if err := testsupport.LoadGolden(filepath.Join("testdata", "basic_manifest.json"), &m); err != nil {
		t.Fatalf("load golden: %v", err)
	}

	module, err := manifest.FindModule(&m, "go-errors")
	if err != nil {
		t.Fatalf("FindModule error: %v", err)
	}

	if module.Module != "github.com/goliatone/go-errors" {
		t.Fatalf("unexpected module path: %s", module.Module)
	}
}
