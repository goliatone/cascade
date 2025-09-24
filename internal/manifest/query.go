package manifest

import "fmt"

// FindModule returns the module with the provided name.
func FindModule(m *Manifest, name string) (*Module, error) {
	for i := range m.Modules {
		if m.Modules[i].Name == name {
			return &m.Modules[i], nil
		}
	}
	return nil, &ModuleNotFoundError{ModuleName: name}
}

// ModuleNotFoundError is returned when a module cannot be found.
type ModuleNotFoundError struct {
	ModuleName string
}

func (e *ModuleNotFoundError) Error() string {
	return fmt.Sprintf("module not found: %s", e.ModuleName)
}

// ExpandDefaults applies defaults to a dependent and returns the result.
func ExpandDefaults(d Dependent, defaults Defaults) Dependent {
	result := d

	// Apply default scalar values when the dependent leaves them empty
	if result.Branch == "" {
		result.Branch = defaults.Branch
	}

	// Merge slice fields by appending defaults first, then dependent-specific entries
	if result.Tests == nil {
		result.Tests = make([]Command, 0)
	}
	if len(defaults.Tests) > 0 {
		// Only add defaults that aren't already in dependent's tests
		result.Tests = mergeCommands(defaults.Tests, result.Tests)
	}

	if result.ExtraCommands == nil {
		result.ExtraCommands = make([]Command, 0)
	}
	if len(defaults.ExtraCommands) > 0 {
		result.ExtraCommands = mergeCommands(defaults.ExtraCommands, result.ExtraCommands)
	}

	if result.Labels == nil {
		result.Labels = make([]string, 0)
	}
	if len(defaults.Labels) > 0 {
		result.Labels = mergeStrings(defaults.Labels, result.Labels)
	}

	// Merge nested structs without overwriting explicit dependent values
	result.Notifications = mergeNotifications(defaults.Notifications, result.Notifications)
	result.PR = mergePRConfig(defaults.PR, result.PR)

	return result
}

// mergeCommands combines default commands with dependent commands, avoiding duplicates
func mergeCommands(defaults, dependent []Command) []Command {
	result := make([]Command, 0, len(defaults)+len(dependent))
	result = append(result, defaults...)
	result = append(result, dependent...)
	return result
}

// mergeStrings combines default strings with dependent strings, avoiding duplicates
func mergeStrings(defaults, dependent []string) []string {
	result := make([]string, 0, len(defaults)+len(dependent))
	result = append(result, defaults...)
	result = append(result, dependent...)
	return result
}

// mergeNotifications merges notification settings, preferring dependent values
func mergeNotifications(defaults, dependent Notifications) Notifications {
	result := dependent
	if result.SlackChannel == "" {
		result.SlackChannel = defaults.SlackChannel
	}
	if result.Webhook == "" {
		result.Webhook = defaults.Webhook
	}
	return result
}

// mergePRConfig merges PR configuration, preferring dependent values
func mergePRConfig(defaults, dependent PRConfig) PRConfig {
	result := dependent
	if result.TitleTemplate == "" {
		result.TitleTemplate = defaults.TitleTemplate
	}
	if result.BodyTemplate == "" {
		result.BodyTemplate = defaults.BodyTemplate
	}
	if result.Reviewers == nil && len(defaults.Reviewers) > 0 {
		result.Reviewers = make([]string, len(defaults.Reviewers))
		copy(result.Reviewers, defaults.Reviewers)
	}
	if result.TeamReviewers == nil && len(defaults.TeamReviewers) > 0 {
		result.TeamReviewers = make([]string, len(defaults.TeamReviewers))
		copy(result.TeamReviewers, defaults.TeamReviewers)
	}
	return result
}
