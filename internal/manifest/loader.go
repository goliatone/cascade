package manifest

import (
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
		return nil, &LoadError{Path: path, Err: err}
	}

	var manifest Manifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, &ParseError{Path: path, Err: err}
	}

	// normalize the manifest after unmarshaling to ensure non nil slices
	normalizeManifest(&manifest)

	return &manifest, nil
}

func (l *loader) Generate(workdir string) (*Manifest, error) {
	return nil, &GenerateError{WorkDir: workdir, Reason: "not supported"}
}

// normalizeManifest ensures all slices are non nil after YAML unmarshaling
// so that validation doesn't reject manifests that omit optional fields.
func normalizeManifest(m *Manifest) {
	if m.Modules == nil {
		m.Modules = []Module{}
	}

	for i := range m.Modules {
		module := &m.Modules[i]

		if module.Dependents == nil {
			module.Dependents = []Dependent{}
		}

		for j := range module.Dependents {
			dependent := &module.Dependents[j]

			if dependent.Tests == nil {
				dependent.Tests = []Command{}
			}
			if dependent.ExtraCommands == nil {
				dependent.ExtraCommands = []Command{}
			}
			if dependent.Labels == nil {
				dependent.Labels = []string{}
			}
		}
	}

	if m.Defaults.Tests == nil {
		m.Defaults.Tests = []Command{}
	}

	if m.Defaults.ExtraCommands == nil {
		m.Defaults.ExtraCommands = []Command{}
	}

	if m.Defaults.Labels == nil {
		m.Defaults.Labels = []string{}
	}

	// Leave PR reviewer slices as-is (nil signals defaults should apply).
}
