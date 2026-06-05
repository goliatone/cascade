package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

func (l *LocalUpdateHooksConfig) UnmarshalYAML(value *yaml.Node) error {
	allowed := map[string]bool{
		"after":          true,
		"after_success":  true,
		"after_failure":  true,
		"always":         true,
		"rules":          true,
		"disabled_rules": true,
	}
	if err := rejectUnknownYAMLFields(value, allowed, "local update hooks"); err != nil {
		return err
	}

	type plain LocalUpdateHooksConfig
	var out plain
	if err := value.Decode(&out); err != nil {
		return err
	}
	*l = LocalUpdateHooksConfig(out)
	return nil
}

func (r *LocalUpdateHookRule) UnmarshalYAML(value *yaml.Node) error {
	allowed := map[string]bool{
		"name":          true,
		"match":         true,
		"after":         true,
		"after_success": true,
		"after_failure": true,
		"always":        true,
	}
	if err := rejectUnknownYAMLFields(value, allowed, "local update hook rule"); err != nil {
		return err
	}

	type plain LocalUpdateHookRule
	var out plain
	if err := value.Decode(&out); err != nil {
		return err
	}
	*r = LocalUpdateHookRule(out)
	return nil
}

func (m *LocalUpdateHookMatch) UnmarshalYAML(value *yaml.Node) error {
	allowed := map[string]bool{
		"modules":                 true,
		"module_prefixes":         true,
		"workspaces":              true,
		"workspace_prefixes":      true,
		"module_dirs":             true,
		"module_dir_prefixes":     true,
		"exclude_modules":         true,
		"exclude_module_prefixes": true,
	}
	if err := rejectUnknownYAMLFields(value, allowed, "local update hook match"); err != nil {
		return err
	}

	type plain LocalUpdateHookMatch
	var out plain
	if err := value.Decode(&out); err != nil {
		return err
	}
	*m = LocalUpdateHookMatch(out)
	return nil
}

func rejectUnknownYAMLFields(value *yaml.Node, allowed map[string]bool, label string) error {
	if value == nil || value.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(value.Content)-1; i += 2 {
		key := value.Content[i].Value
		if !allowed[key] {
			return fmt.Errorf("%s contains unsupported field %q", label, key)
		}
	}
	return nil
}
