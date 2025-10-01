package service

import (
	"context"

	"github.com/goliatone/cascade/internal/manifest"
	"github.com/goliatone/cascade/pkg/config"
	"github.com/goliatone/cascade/pkg/workspace"
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

type DiscoveryService interface {
	Discover(ctx context.Context, inputs DiscoveryInputs) (DiscoveryOutputs, error)
}

func NewDiscoveryService() DiscoveryService {
	return &discoveryService{}
}

type discoveryService struct{}

func (d *discoveryService) Discover(ctx context.Context, inputs DiscoveryInputs) (DiscoveryOutputs, error) {
	workspaceDir := workspace.Resolve(inputs.Workspace, inputs.Config, inputs.ModulePath, inputs.ModuleDir)

	merged, err := performMultiSourceDiscovery(ctx, inputs.ModulePath, inputs.Version, inputs.GitHubOrg,
		workspaceDir, inputs.MaxDepth, inputs.IncludePatterns, inputs.ExcludePatterns,
		inputs.GitHubInclude, inputs.GitHubExclude, inputs.Config, container.Logger())
	if err != nil {
		return DiscoveryOutputs{WorkspaceDir: workspaceDir}, err
	}

	filtered, _ := filterDiscoveredDependents(merged, inputs.ModulePath, inputs.Version, workspaceDir, container.Logger())

	return DiscoveryOutputs{
		WorkspaceDir: workspaceDir,
		Dependents:   filtered,
	}, nil
}
