package manifest

import "fmt"

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
	return nil, fmt.Errorf("manifest: Load not implemented (%s)", path)
}

func (l *loader) Generate(workdir string) (*Manifest, error) {
	return nil, fmt.Errorf("manifest: Generate not implemented (%s)", workdir)
}
