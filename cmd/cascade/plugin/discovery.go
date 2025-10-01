package plugin

import (
	"context"

	"github.com/goliatone/cascade/internal/manifest"
	"github.com/goliatone/cascade/pkg/config"
	workspacepkg "github.com/goliatone/cascade/pkg/workspace"
)

type DiscoveryInputs struct {
	Workspace       string
	ModulePath      string
	ModuleDir       string
	Version         string
	GitHubOrg       string
	MaxDepth        int
	IncludePatterns []string
	ExcludePatterns []string
	GitHubInclude   []string
	GitHubExclude   []string
	Config          *config.Config
}

type DiscoveryOutputs struct {
	WorkspaceDir string
	Dependents   []manifest.DependentOptions
}

type DiscoveryPlugin interface {
	Discover(ctx context.Context, inputs DiscoveryInputs) (DiscoveryOutputs, error)
}
