package config

import (
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/goliatone/cascade/pkg/testsupport"
)

func TestLoadConfigFixtures(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
		golden  string
		wantErr bool
	}{
		{
			name:    "defaults when missing",
			fixture: "missing.yaml",
			golden:  "missing.golden.json",
		},
		{
			name:    "basic config",
			fixture: "basic.yaml",
			golden:  "basic.golden.json",
		},
		{
			name:    "dry run config",
			fixture: "dry_run.yaml",
			golden:  "dry_run.golden.json",
		},
		{
			name:    "invalid skip pattern",
			fixture: "invalid_skip.yaml",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join("testdata", tc.fixture)

			cfg, err := Load(path)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("load config: %v", err)
			}

			var want Config
			if err := testsupport.LoadGolden(goldenPath(tc.golden), &want); err != nil {
				t.Fatalf("load golden: %v", err)
			}

			if diff := cmp.Diff(want, cfg); diff != "" {
				t.Fatalf("unexpected config (-want +got):\n%s", diff)
			}
		})
	}
}

func goldenPath(name string) string {
	return filepath.Join("testdata", name)
}
