package manifest

// FindModule returns the module with the provided name.
func FindModule(m *Manifest, name string) (*Module, error) {
	for i := range m.Modules {
		if m.Modules[i].Name == name {
			return &m.Modules[i], nil
		}
	}
	return nil, &ModuleNotFoundError{ModuleName: name}
}

// FindModuleByPath returns the module with the provided module path.
func FindModuleByPath(m *Manifest, modulePath string) (*Module, error) {
	for i := range m.Modules {
		if m.Modules[i].Module == modulePath {
			return &m.Modules[i], nil
		}
	}
	return nil, &ModuleNotFoundError{ModuleName: modulePath}
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

	// Add all defaults first
	result = append(result, defaults...)

	// Add dependent commands that aren't already present
	for _, dep := range dependent {
		if !containsCommand(result, dep) {
			result = append(result, dep)
		}
	}

	return result
}

// mergeStrings combines default strings with dependent strings, avoiding duplicates
func mergeStrings(defaults, dependent []string) []string {
	result := make([]string, 0, len(defaults)+len(dependent))

	// Add all defaults first
	result = append(result, defaults...)

	// Add dependent strings that aren't already present
	for _, dep := range dependent {
		if !containsString(result, dep) {
			result = append(result, dep)
		}
	}

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
	if result.GitHubIssues == nil && defaults.GitHubIssues != nil {
		copy := *defaults.GitHubIssues
		if len(copy.Labels) > 0 {
			copy.Labels = append([]string(nil), copy.Labels...)
		}
		result.GitHubIssues = &copy
	} else if result.GitHubIssues != nil && defaults.GitHubIssues != nil {
		if len(result.GitHubIssues.Labels) == 0 && len(defaults.GitHubIssues.Labels) > 0 {
			result.GitHubIssues.Labels = append([]string(nil), defaults.GitHubIssues.Labels...)
		}
	}
	return result
}

// containsCommand checks if a command is already present in the slice
func containsCommand(commands []Command, target Command) bool {
	for _, cmd := range commands {
		if len(cmd.Cmd) != len(target.Cmd) {
			continue
		}
		if cmd.Dir != target.Dir {
			continue
		}
		// Compare command slices
		match := true
		for i, arg := range cmd.Cmd {
			if arg != target.Cmd[i] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// containsString checks if a string is already present in the slice
func containsString(strings []string, target string) bool {
	for _, s := range strings {
		if s == target {
			return true
		}
	}
	return false
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

// HasOriginalPRConfig returns true if the dependent had any PR configuration
// set originally (before defaults were applied).
func HasOriginalPRConfig(d Dependent) bool {
	return d.PR.TitleTemplate != "" ||
		d.PR.BodyTemplate != "" ||
		len(d.PR.Reviewers) > 0 ||
		len(d.PR.TeamReviewers) > 0
}

// ExpandDefaultsWithMetadata applies defaults to a dependent and returns the result
// along with metadata about which fields came from defaults.
func ExpandDefaultsWithMetadata(d Dependent, defaults Defaults) (Dependent, bool) {
	hadOriginalPR := HasOriginalPRConfig(d)
	expanded := ExpandDefaults(d, defaults)
	return expanded, hadOriginalPR
}
