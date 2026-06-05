package config

import (
	"bytes"
	"encoding/json"
	"fmt"
)

func (l *LocalUpdateHooksConfig) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil
	}
	allowed := map[string]bool{
		"after":          true,
		"after_success":  true,
		"after_failure":  true,
		"always":         true,
		"rules":          true,
		"disabled_rules": true,
	}
	if err := rejectUnknownJSONFields(data, allowed, "local update hooks"); err != nil {
		return err
	}

	type plain LocalUpdateHooksConfig
	var out plain
	if err := json.Unmarshal(data, &out); err != nil {
		return err
	}
	*l = LocalUpdateHooksConfig(out)
	return nil
}

func (r *LocalUpdateHookRule) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil
	}
	allowed := map[string]bool{
		"name":          true,
		"match":         true,
		"after":         true,
		"after_success": true,
		"after_failure": true,
		"always":        true,
	}
	if err := rejectUnknownJSONFields(data, allowed, "local update hook rule"); err != nil {
		return err
	}

	type plain LocalUpdateHookRule
	var out plain
	if err := json.Unmarshal(data, &out); err != nil {
		return err
	}
	*r = LocalUpdateHookRule(out)
	return nil
}

func (m *LocalUpdateHookMatch) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil
	}
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
	if err := rejectUnknownJSONFields(data, allowed, "local update hook match"); err != nil {
		return err
	}

	type plain LocalUpdateHookMatch
	var out plain
	if err := json.Unmarshal(data, &out); err != nil {
		return err
	}
	*m = LocalUpdateHookMatch(out)
	return nil
}

func rejectUnknownJSONFields(data []byte, allowed map[string]bool, label string) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for key := range raw {
		if !allowed[key] {
			return fmt.Errorf("%s contains unsupported field %q", label, key)
		}
	}
	return nil
}
