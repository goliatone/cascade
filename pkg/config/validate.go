package config

// Validate inspects the configuration for missing or invalid fields.
// Detailed validation rules will be implemented alongside concrete parsing logic.
func Validate(cfg *Config) error {
	_ = cfg
	return newNotImplemented("Validate")
}
