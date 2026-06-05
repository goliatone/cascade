package hooks

import (
	"maps"
	"slices"

	"github.com/goliatone/cascade/pkg/config"
)

// NormalizeLocalUpdate treats hooks.update.local.after as an alias for
// after_success. Alias entries run before explicit after_success entries.
func NormalizeLocalUpdate(in config.LocalUpdateHooksConfig) LocalUpdatePlan {
	return LocalUpdatePlan{
		AfterSuccess: appendHookConfigs(in.After, in.AfterSuccess...),
		AfterFailure: cloneHookConfigs(in.AfterFailure),
		Always:       cloneHookConfigs(in.Always),
	}
}

func appendHookConfigs(first []config.HookConfig, rest ...config.HookConfig) []config.HookConfig {
	out := cloneHookConfigs(first)
	for _, hook := range rest {
		out = append(out, cloneHookConfig(hook))
	}
	return out
}

func cloneHookConfigs(in []config.HookConfig) []config.HookConfig {
	if len(in) == 0 {
		return nil
	}
	out := make([]config.HookConfig, len(in))
	for i, hook := range in {
		out[i] = cloneHookConfig(hook)
	}
	return out
}

func cloneHookConfig(in config.HookConfig) config.HookConfig {
	out := in
	out.Cmd = slices.Clone(in.Cmd)
	if in.Env != nil {
		out.Env = maps.Clone(in.Env)
	}
	return out
}
