package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ConfigFileLocations returns standard locations where configuration files are searched.
// Search order follows XDG Base Directory Specification with fallbacks.
func ConfigFileLocations() []string {
	home := os.Getenv("HOME")
	xdgConfig := os.Getenv("XDG_CONFIG_HOME")

	if xdgConfig == "" && home != "" {
		xdgConfig = filepath.Join(home, ".config")
	}

	locations := []string{
		".cascade.yaml",
		".cascade.yml",
		".cascade.json",
	}

	if xdgConfig != "" {
		locations = append(locations,
			filepath.Join(xdgConfig, "cascade", "config.yaml"),
			filepath.Join(xdgConfig, "cascade", "config.yml"),
			filepath.Join(xdgConfig, "cascade", "config.json"),
		)
	}

	if home != "" {
		locations = append(locations,
			filepath.Join(home, ".config", "cascade", "config.yaml"),
			filepath.Join(home, ".config", "cascade", "config.yml"),
			filepath.Join(home, ".config", "cascade", "config.json"),
		)
	}

	return locations
}

// DiscoverConfigFile searches for configuration files in standard locations.
// Returns the path to the first configuration file found, or empty string if none found.
func DiscoverConfigFile() (string, error) {
	locations := ConfigFileLocations()

	for _, path := range locations {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", nil // No config file found, but not an error
}

// LoadFromFile reads configuration from the provided path.
// Supports YAML, JSON, and TOML formats based on file extension.
func LoadFromFile(path string) (*Config, error) {
	if path == "" {
		return nil, fmt.Errorf("config file path cannot be empty")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	config := New()
	ext := strings.ToLower(filepath.Ext(path))

	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, config); err != nil {
			return nil, fmt.Errorf("failed to parse YAML config file %s: %w", path, err)
		}
	case ".json":
		if err := json.Unmarshal(data, config); err != nil {
			return nil, fmt.Errorf("failed to parse JSON config file %s: %w", path, err)
		}
	default:
		return nil, fmt.Errorf("unsupported config file format: %s (supported: .yaml, .yml, .json)", ext)
	}

	return config, nil
}

// LoadFromFileOrDiscover loads configuration from the specified path,
// or discovers and loads from standard locations if path is empty.
func LoadFromFileOrDiscover(path string) (*Config, error) {
	if path != "" {
		return LoadFromFile(path)
	}

	discoveredPath, err := DiscoverConfigFile()
	if err != nil {
		return nil, fmt.Errorf("failed to discover config file: %w", err)
	}

	if discoveredPath == "" {
		// No config file found, return empty config (will use defaults/env/flags)
		return New(), nil
	}

	return LoadFromFile(discoveredPath)
}
