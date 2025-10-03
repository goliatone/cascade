package planner

import "github.com/goliatone/cascade/internal/manifest"

// convertModuleConfig transforms a ModuleConfig into a DependentConfig so it can be merged
// with the same precedence logic as dependent overrides.
func convertModuleConfig(module *manifest.ModuleConfig) *manifest.DependentConfig {
	if module == nil {
		return nil
	}

	cfg := &manifest.DependentConfig{
		Branch:        module.Branch,
		Tests:         cloneCommands(module.Tests),
		ExtraCommands: cloneCommands(module.ExtraCommands),
		Labels:        cloneStrings(module.Labels),
		Notifications: cloneNotifications(module.Notifications),
		PR:            clonePRConfig(module.PR),
		Env:           cloneEnv(module.Env),
		Timeout:       module.Timeout,
	}

	return cfg
}

// applyDependentConfig merges the provided config onto the base dependent, giving precedence
// to the override values when present.
func applyDependentConfig(base manifest.Dependent, cfg *manifest.DependentConfig) manifest.Dependent {
	if cfg == nil {
		return base
	}

	if cfg.Branch != "" {
		base.Branch = cfg.Branch
	}

	if len(cfg.Tests) > 0 {
		base.Tests = cloneCommands(cfg.Tests)
	}

	if len(cfg.ExtraCommands) > 0 {
		base.ExtraCommands = cloneCommands(cfg.ExtraCommands)
	}

	if len(cfg.Labels) > 0 {
		base.Labels = cloneStrings(cfg.Labels)
	}

	if !isZeroNotifications(cfg.Notifications) {
		base.Notifications = applyNotificationOverrides(base.Notifications, cfg.Notifications)
	}

	base.PR = applyPROverrides(base.PR, cfg.PR)

	if len(cfg.Env) > 0 {
		base.Env = mergeEnv(base.Env, cfg.Env)
	}

	if cfg.Timeout > 0 {
		base.Timeout = cfg.Timeout
	}

	if cfg.Canary {
		base.Canary = true
	}

	if cfg.Skip {
		base.Skip = true
	}

	return base
}

func cloneCommands(cmds []manifest.Command) []manifest.Command {
	if len(cmds) == 0 {
		return nil
	}

	cloned := make([]manifest.Command, len(cmds))
	for i, cmd := range cmds {
		cloned[i] = manifest.Command{
			Cmd: append([]string(nil), cmd.Cmd...),
			Dir: cmd.Dir,
		}
	}
	return cloned
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}

func cloneEnv(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for k, v := range values {
		cloned[k] = v
	}
	return cloned
}

func mergeEnv(base map[string]string, override map[string]string) map[string]string {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}

	result := make(map[string]string, len(base)+len(override))
	for k, v := range base {
		result[k] = v
	}
	for k, v := range override {
		result[k] = v
	}
	return result
}

func cloneNotifications(n manifest.Notifications) manifest.Notifications {
	copy := manifest.Notifications{
		SlackChannel: n.SlackChannel,
		OnFailure:    n.OnFailure,
		OnSuccess:    n.OnSuccess,
		Webhook:      n.Webhook,
	}
	if n.GitHubIssues != nil {
		issues := *n.GitHubIssues
		if len(issues.Labels) > 0 {
			issues.Labels = cloneStrings(issues.Labels)
		}
		copy.GitHubIssues = &issues
	}
	return copy
}

func isZeroNotifications(n manifest.Notifications) bool {
	return n.SlackChannel == "" &&
		!n.OnFailure &&
		!n.OnSuccess &&
		n.Webhook == "" &&
		n.GitHubIssues == nil
}

func applyNotificationOverrides(base, override manifest.Notifications) manifest.Notifications {
	result := base
	if override.SlackChannel != "" {
		result.SlackChannel = override.SlackChannel
	}
	if override.Webhook != "" {
		result.Webhook = override.Webhook
	}
	if override.OnFailure {
		result.OnFailure = true
	}
	if override.OnSuccess {
		result.OnSuccess = true
	}
	if override.GitHubIssues != nil {
		issues := *override.GitHubIssues
		if len(issues.Labels) > 0 {
			issues.Labels = cloneStrings(issues.Labels)
		}
		result.GitHubIssues = &issues
	}
	return result
}

func clonePRConfig(cfg manifest.PRConfig) manifest.PRConfig {
	copy := manifest.PRConfig{
		TitleTemplate: cfg.TitleTemplate,
		BodyTemplate:  cfg.BodyTemplate,
	}
	if len(cfg.Reviewers) > 0 {
		copy.Reviewers = cloneStrings(cfg.Reviewers)
	}
	if len(cfg.TeamReviewers) > 0 {
		copy.TeamReviewers = cloneStrings(cfg.TeamReviewers)
	}
	return copy
}

func applyPROverrides(base, override manifest.PRConfig) manifest.PRConfig {
	result := base
	if override.TitleTemplate != "" {
		result.TitleTemplate = override.TitleTemplate
	}
	if override.BodyTemplate != "" {
		result.BodyTemplate = override.BodyTemplate
	}
	if len(override.Reviewers) > 0 {
		result.Reviewers = cloneStrings(override.Reviewers)
	}
	if len(override.TeamReviewers) > 0 {
		result.TeamReviewers = cloneStrings(override.TeamReviewers)
	}
	return result
}
