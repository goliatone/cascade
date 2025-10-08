package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the dependency-registration configuration schema.
type Config struct {
	// SkipPatterns are regex patterns for modules that should be ignored by the
	// detector (e.g., internal tooling modules).
	SkipPatterns []string `yaml:"skip_patterns"`
	// Workflow identifies the workflow file in the dependency repository to
	// dispatch when `.cascade.yaml` exists. Defaults to `cascade-release.yml`.
	Workflow string `yaml:"workflow"`
	// DryRun toggles notification behaviour; when true, the workflow should
	// detect changes but avoid GitHub mutations.
	DryRun bool `yaml:"dry_run"`
	// Workspaces lists additional go.work entries or module directories to scan
	// when detecting dependency changes in multi-module repositories.
	Workspaces []string `yaml:"workspaces"`
}

// Defaults returns a Config populated with default values.
func Defaults() Config {
	return Config{
		Workflow: "cascade-release.yml",
	}
}

// Load reads configuration from path. If path does not exist, the default
// configuration is returned.
func Load(path string) (Config, error) {
	cfg := Defaults()
	if path == "" {
		return cfg, nil
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return cfg, fmt.Errorf("resolve config path: %w", err)
	}

	bytes, err := os.ReadFile(abs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("read config: %w", err)
	}

	if err := yaml.Unmarshal(bytes, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// Validate ensures the configuration is well formed.
func (c Config) Validate() error {
	for i, pattern := range c.SkipPatterns {
		if pattern == "" {
			return fmt.Errorf("skip_patterns[%d] must not be empty", i)
		}
	}
	return nil
}
