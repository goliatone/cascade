package manifest

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const manifestFileName = ".cascade.yaml"

// LoadDependentManifest loads a dependent repository's manifest if present.
// Returns nil when the manifest file does not exist.
func LoadDependentManifest(ctx context.Context, repoPath string) (*Manifest, error) {
	_ = ctx // reserved for future logging/metrics needs

	if repoPath == "" {
		return nil, nil
	}

	manifestPath := filepath.Join(repoPath, manifestFileName)
	if _, err := os.Stat(manifestPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat dependent manifest %s: %w", manifestPath, err)
	}

	loader := NewLoader()
	manifest, err := loader.Load(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("load dependent manifest %s: %w", manifestPath, err)
	}

	return manifest, nil
}

// LoadDependentOverrides looks for a dependent repository's manifest and returns any
// override the repo declares for the provided module path. If no override exists,
// the function returns nil.
func LoadDependentOverrides(ctx context.Context, repoPath, modulePath string) (*DependentConfig, error) {
	if modulePath == "" {
		return nil, nil
	}

	manifest, err := LoadDependentManifest(ctx, repoPath)
	if err != nil || manifest == nil {
		return nil, err
	}

	if manifest.Dependents == nil {
		return nil, nil
	}

	config, ok := manifest.Dependents[modulePath]
	if !ok {
		return nil, nil
	}

	override := config
	return &override, nil
}
