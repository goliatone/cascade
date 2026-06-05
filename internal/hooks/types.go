package hooks

import (
	"fmt"
	"strings"
	"time"

	"github.com/goliatone/cascade/pkg/config"
)

type Phase string

const (
	PhaseAfterSuccess Phase = "after_success"
	PhaseAfterFailure Phase = "after_failure"
	PhaseAlways       Phase = "always"
)

type PhaseHooks struct {
	Phase Phase
	Hooks []config.HookConfig
}

type LocalUpdatePlan struct {
	AfterSuccess  []config.HookConfig
	AfterFailure  []config.HookConfig
	Always        []config.HookConfig
	MatchedRules  []MatchedRule
	SkippedRules  []RuleMatch
	DisabledRules []DisabledRule
}

func (p LocalUpdatePlan) Empty() bool {
	return len(p.AfterSuccess) == 0 && len(p.AfterFailure) == 0 && len(p.Always) == 0
}

func (p LocalUpdatePlan) SelectedPhases(updateSucceeded bool) []PhaseHooks {
	phases := make([]PhaseHooks, 0, 2)
	if updateSucceeded {
		if len(p.AfterSuccess) > 0 {
			phases = append(phases, PhaseHooks{Phase: PhaseAfterSuccess, Hooks: p.AfterSuccess})
		}
	} else if len(p.AfterFailure) > 0 {
		phases = append(phases, PhaseHooks{Phase: PhaseAfterFailure, Hooks: p.AfterFailure})
	}
	if len(p.Always) > 0 {
		phases = append(phases, PhaseHooks{Phase: PhaseAlways, Hooks: p.Always})
	}
	return phases
}

type Context struct {
	Command      string
	Module       string
	ModuleDir    string
	Workspace    string
	UpdateStatus string
	UpdatedCount int
	TidyRan      bool
	TidyFailed   bool
	Env          map[string]string
}

type RuleSource struct {
	Scope   config.ConfigLayerScope
	Path    string
	BaseDir string
	Order   int
}

type MatchedRule struct {
	Name         string
	Source       RuleSource
	AfterSuccess []config.HookConfig
	AfterFailure []config.HookConfig
	Always       []config.HookConfig
}

type RuleMatch struct {
	Name   string
	Source RuleSource
	Reason string
}

type DisabledRule struct {
	Name       string
	Source     RuleSource
	DisabledBy RuleSource
}

type Result struct {
	Phase    Phase
	Hook     config.HookConfig
	Name     string
	Command  string
	Dir      string
	Output   string
	ExitCode int
	Duration time.Duration
	Err      error
	Timeout  bool
}

func (r Result) Failed() bool {
	return r.Err != nil
}

func (r Result) DisplayName() string {
	if strings.TrimSpace(r.Name) != "" {
		return strings.TrimSpace(r.Name)
	}
	if strings.TrimSpace(r.Command) != "" {
		return strings.TrimSpace(r.Command)
	}
	return "hook"
}

type Results []Result

func (r Results) Failed() bool {
	for _, result := range r {
		if result.Failed() {
			return true
		}
	}
	return false
}

func (r Results) Err() error {
	var failed []string
	for _, result := range r {
		if result.Failed() {
			failed = append(failed, fmt.Sprintf("%s:%s", result.Phase, result.DisplayName()))
		}
	}
	if len(failed) == 0 {
		return nil
	}
	return fmt.Errorf("local update hook failure: %s", strings.Join(failed, ", "))
}
