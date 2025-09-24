package config

// LoadFromFile reads configuration from the provided path.
// Future work will support YAML and JSON inputs.
func LoadFromFile(path string) (*Config, error) {
	_ = path
	return nil, newNotImplemented("LoadFromFile")
}
