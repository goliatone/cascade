package hooks

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/goliatone/cascade/pkg/config"
)

type ruleCandidate struct {
	rule   config.LocalUpdateHookRule
	source RuleSource
}

type disabledCandidate struct {
	source RuleSource
}

// ResolveLocalUpdatePlan builds the local update hook execution plan from
// unconditional merged hooks plus named rules from ordered file layers.
func ResolveLocalUpdatePlan(local config.LocalUpdateHooksConfig, hookCtx Context, layers []config.ConfigLayer) LocalUpdatePlan {
	plan := NormalizeLocalUpdate(local)
	candidates, disabled := collectRuleCandidates(local, layers)

	for _, candidate := range candidates {
		if disabledBy, ok := disabled[strings.TrimSpace(candidate.rule.Name)]; ok && disabledBy.source.Order >= candidate.source.Order {
			plan.DisabledRules = append(plan.DisabledRules, DisabledRule{
				Name:       strings.TrimSpace(candidate.rule.Name),
				Source:     candidate.source,
				DisabledBy: disabledBy.source,
			})
			continue
		}

		if ok, reason := ruleMatches(candidate.rule.Match, hookCtx, candidate.source.BaseDir); ok {
			matched := matchedRule(candidate)
			plan.MatchedRules = append(plan.MatchedRules, matched)
			plan.AfterSuccess = append(plan.AfterSuccess, matched.AfterSuccess...)
			plan.AfterFailure = append(plan.AfterFailure, matched.AfterFailure...)
			plan.Always = append(plan.Always, matched.Always...)
		} else {
			plan.SkippedRules = append(plan.SkippedRules, RuleMatch{
				Name:   strings.TrimSpace(candidate.rule.Name),
				Source: candidate.source,
				Reason: reason,
			})
		}
	}

	return plan
}

func collectRuleCandidates(local config.LocalUpdateHooksConfig, layers []config.ConfigLayer) ([]ruleCandidate, map[string]disabledCandidate) {
	if len(layers) == 0 {
		source := RuleSource{Order: 0}
		candidates := make([]ruleCandidate, 0, len(local.Rules))
		for _, rule := range local.Rules {
			candidates = append(candidates, ruleCandidate{rule: rule, source: source})
		}
		disabled := map[string]disabledCandidate{}
		for _, name := range local.DisabledRules {
			disabled[strings.TrimSpace(name)] = disabledCandidate{source: source}
		}
		return candidates, disabled
	}

	byName := map[string]ruleCandidate{}
	var order []string
	disabled := map[string]disabledCandidate{}

	for layerOrder, layer := range layers {
		if layer.Config == nil {
			continue
		}
		source := RuleSource{
			Scope:   layer.Scope,
			Path:    layer.Path,
			BaseDir: layer.BaseDir,
			Order:   layerOrder,
		}
		layerLocal := layer.Config.Hooks.Update.Local
		for _, rule := range layerLocal.Rules {
			name := strings.TrimSpace(rule.Name)
			if _, exists := byName[name]; !exists {
				order = append(order, name)
			}
			byName[name] = ruleCandidate{rule: rule, source: source}
			if disabledBy, ok := disabled[name]; ok && disabledBy.source.Order < source.Order {
				delete(disabled, name)
			}
		}
		for _, name := range layerLocal.DisabledRules {
			disabled[strings.TrimSpace(name)] = disabledCandidate{source: source}
		}
	}

	candidates := make([]ruleCandidate, 0, len(order))
	for _, name := range order {
		candidates = append(candidates, byName[name])
	}
	return candidates, disabled
}

func matchedRule(candidate ruleCandidate) MatchedRule {
	rule := candidate.rule
	normalized := NormalizeLocalUpdate(config.LocalUpdateHooksConfig{
		After:        rule.After,
		AfterSuccess: rule.AfterSuccess,
		AfterFailure: rule.AfterFailure,
		Always:       rule.Always,
	})
	return MatchedRule{
		Name:         strings.TrimSpace(rule.Name),
		Source:       candidate.source,
		AfterSuccess: normalized.AfterSuccess,
		AfterFailure: normalized.AfterFailure,
		Always:       normalized.Always,
	}
}

func ruleMatches(match config.LocalUpdateHookMatch, hookCtx Context, baseDir string) (bool, string) {
	module := strings.TrimSpace(hookCtx.Module)
	workspace := normalizePathForMatch(hookCtx.Workspace, "")
	moduleDir := normalizePathForMatch(hookCtx.ModuleDir, "")

	if matchesAnyString(module, match.ExcludeModules) {
		return false, "module excluded"
	}
	if matchesAnyPrefix(module, match.ExcludeModulePrefixes) {
		return false, "module prefix excluded"
	}

	if len(match.Modules) > 0 && !matchesAnyString(module, match.Modules) {
		return false, "module did not match"
	}
	if len(match.ModulePrefixes) > 0 && !matchesAnyPrefix(module, match.ModulePrefixes) {
		return false, "module prefix did not match"
	}
	if len(match.Workspaces) > 0 && !matchesAnyPath(workspace, match.Workspaces, baseDir) {
		return false, "workspace did not match"
	}
	if len(match.WorkspacePrefixes) > 0 && !matchesAnyPathPrefix(workspace, match.WorkspacePrefixes, baseDir) {
		return false, "workspace prefix did not match"
	}
	if len(match.ModuleDirs) > 0 && !matchesAnyPath(moduleDir, match.ModuleDirs, baseDir) {
		return false, "module dir did not match"
	}
	if len(match.ModuleDirPrefixes) > 0 && !matchesAnyPathPrefix(moduleDir, match.ModuleDirPrefixes, baseDir) {
		return false, "module dir prefix did not match"
	}
	return true, ""
}

func matchesAnyString(value string, candidates []string) bool {
	for _, candidate := range candidates {
		if value == strings.TrimSpace(candidate) {
			return true
		}
	}
	return false
}

func matchesAnyPrefix(value string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(value, strings.TrimSpace(prefix)) {
			return true
		}
	}
	return false
}

func matchesAnyPath(value string, candidates []string, baseDir string) bool {
	for _, candidate := range candidates {
		if value == normalizePathForMatch(candidate, baseDir) {
			return true
		}
	}
	return false
}

func matchesAnyPathPrefix(value string, prefixes []string, baseDir string) bool {
	for _, prefix := range prefixes {
		normalized := normalizePathForMatch(prefix, baseDir)
		if value == normalized || strings.HasPrefix(value, withPathSeparator(normalized)) {
			return true
		}
	}
	return false
}

func normalizePathForMatch(path string, baseDir string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "~") {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			if path == "~" {
				path = home
			} else if strings.HasPrefix(path, "~/") {
				path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
			}
		}
	}
	if !filepath.IsAbs(path) && strings.TrimSpace(baseDir) != "" {
		path = filepath.Join(baseDir, path)
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return filepath.Clean(path)
}

func withPathSeparator(path string) string {
	if path == string(filepath.Separator) {
		return path
	}
	if strings.HasSuffix(path, string(filepath.Separator)) {
		return path
	}
	return path + string(filepath.Separator)
}
