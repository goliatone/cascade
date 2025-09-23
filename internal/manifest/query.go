package manifest

import "fmt"

// FindModule returns the module with the provided name.
func FindModule(m *Manifest, name string) (*Module, error) {
	return nil, fmt.Errorf("manifest: FindModule not implemented (%s)", name)
}

// ExpandDefaults applies defaults to a dependent and returns the result.
func ExpandDefaults(d Dependent, defaults Defaults) Dependent {
	return d
}
