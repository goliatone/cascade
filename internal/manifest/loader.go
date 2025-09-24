package manifest

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Loader exposes manifest loading behaviours.
type Loader interface {
	Load(path string) (*Manifest, error)
	Generate(workdir string) (*Manifest, error)
}

// NewLoader returns a stub loader implementation.
func NewLoader() Loader {
	return &loader{}
}

type loader struct{}

func (l *loader) Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("manifest: failed to read file %s: %w", path, err)
	}

	var manifest Manifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("manifest: failed to unmarshal YAML from %s: %w", path, err)
	}

	// ensure main slices are initialized for stability
	if manifest.Modules == nil {
		manifest.Modules = []Module{}
	}

	return &manifest, nil
}

func (l *loader) Generate(workdir string) (*Manifest, error) {
	return nil, fmt.Errorf("manifest: Generate not supported")
}
