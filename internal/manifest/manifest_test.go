package manifest_test

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/goliatone/cascade/internal/manifest"
	"github.com/goliatone/cascade/pkg/testsupport"
)

func TestLoader_Load_GeneratesExpectedManifest(t *testing.T) {

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

func TestLoader_Load_ErrorCases(t *testing.T) {
	loader := manifest.NewLoader()

	tests := []struct {
		name    string
		path    string
		wantErr string
	}{
		{
			name:    "missing file",
			path:    "testdata/nonexistent.yaml",
			wantErr: "failed to read file",
		},
		{
			name:    "invalid yaml",
			path:    "testdata/invalid.yaml",
			wantErr: "failed to unmarshal YAML",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loader.Load(tt.path)
			if err == nil {
				t.Fatalf("Load() expected error but got none")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Load() error = %v, want to contain %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoader_Generate_NotSupported(t *testing.T) {
	loader := manifest.NewLoader()
	_, err := loader.Generate("/tmp")
	if err == nil {
		t.Fatalf("Generate() expected error but got none")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("Generate() error = %v, want to contain 'not supported'", err)
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
