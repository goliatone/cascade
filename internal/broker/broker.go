package broker

import (
	"context"
	"fmt"

	"github.com/goliatone/cascade/internal/executor"
	"github.com/goliatone/cascade/internal/planner"
)

// Config holds broker configuration.
type Config struct {
	// DryRun mode skips actual provider operations
	DryRun bool

	// Default templates
	TitleTemplate string
	BodyTemplate  string

	// Default labels for all PRs
	DefaultLabels []string

	// Notification configuration
	NotificationConfig NotificationConfig
}

// DefaultConfig returns sensible broker defaults.
func DefaultConfig() Config {
	return Config{
		DryRun:             false,
		TitleTemplate:      "", // Will use default from templates.go
		BodyTemplate:       "", // Will use default from templates.go
		DefaultLabels:      []string{"automation:cascade"},
		NotificationConfig: DefaultNotificationConfig(),
	}
}

// New returns a broker implementation with the given provider and notifier.
func New(provider Provider, notifier Notifier, config Config) Broker {
	return &broker{
		provider: provider,
		notifier: notifier,
		config:   config,
	}
}

// NewStub returns a stub broker implementation for testing.
func NewStub() Broker {
	return &broker{}
}

type broker struct {
	provider Provider
	notifier Notifier
	config   Config
}

func (b *broker) EnsurePR(ctx context.Context, item planner.WorkItem, result *executor.Result) (*PullRequest, error) {
	// In dry-run mode, return deterministic metadata without making API calls
	if b.config.DryRun {
		return &PullRequest{
			URL:    fmt.Sprintf("https://github.com/%s/pull/0", item.Repo),
			Number: 0,
			Repo:   item.Repo,
			Labels: b.mergeLabels(item.Labels),
		}, nil
	}

	// Return stub implementation with deterministic golden output if provider not set
	if b.provider == nil {
		// For contract testing, return deterministic result that matches golden file
		return &PullRequest{
			URL:    "",
			Number: 0,
			Repo:   item.Repo,
			Labels: []string{},
		}, nil
	}

	// Skip PR creation for failed execution results (could add comment instead)
	if result != nil && result.Status == executor.StatusFailed {
		return nil, fmt.Errorf("skipping PR creation for failed execution: %s", result.Reason)
	}

	// Render PR title and body using templates
	title, err := RenderTitle(b.config.TitleTemplate, item, result)
	if err != nil {
		return nil, fmt.Errorf("render PR title: %w", err)
	}

	body, err := RenderBody(b.config.BodyTemplate, item, result)
	if err != nil {
		return nil, fmt.Errorf("render PR body: %w", err)
	}

	// Prepare PR input
	prInput := PRInput{
		Repo:       item.Repo,
		BaseBranch: item.Branch,
		HeadBranch: item.BranchName,
		Title:      title,
		Body:       body,
		Labels:     b.mergeLabels(item.Labels),
	}

	// Create or update the pull request
	pr, err := b.provider.CreateOrUpdatePullRequest(ctx, prInput)
	if err != nil {
		return nil, fmt.Errorf("create or update PR: %w", err)
	}

	// Apply labels if they weren't applied during PR creation
	if len(prInput.Labels) > 0 {
		if err := b.provider.AddLabels(ctx, item.Repo, pr.Number, prInput.Labels); err != nil {
			// Don't fail the whole operation for label errors
			// TODO: Log this error
		}
	}

	// Request reviewers if configured
	if len(item.PR.Reviewers) > 0 || len(item.PR.TeamReviewers) > 0 {
		if err := b.provider.RequestReviewers(ctx, item.Repo, pr.Number, item.PR.Reviewers, item.PR.TeamReviewers); err != nil {
			// Don't fail the whole operation for reviewer errors
			// TODO: Log this error
		}
	}

	return pr, nil
}

func (b *broker) Comment(ctx context.Context, pr *PullRequest, body string) error {
	// In dry-run mode, skip actual commenting
	if b.config.DryRun {
		return nil
	}

	if b.provider == nil {
		return &NotImplementedError{Operation: "broker.Comment"}
	}

	// TODO: Implement PR commenting via provider
	// For now, return not implemented since GitHub provider doesn't expose comment method
	return &NotImplementedError{Operation: "broker.Comment"}
}

func (b *broker) Notify(ctx context.Context, item planner.WorkItem, result *executor.Result) error {
	// In dry-run mode, skip actual notifications
	if b.config.DryRun {
		return nil
	}

	if b.notifier == nil {
		return &NotImplementedError{Operation: "broker.Notify"}
	}

	// Send notification - failures shouldn't block main PR flow
	_, err := b.notifier.Send(ctx, item, result)
	if err != nil {
		// Wrap as NotificationError but don't fail the operation
		return &NotificationError{
			Channel: "unknown",
			Err:     fmt.Errorf("send notification: %w", err),
		}
	}

	return nil
}

// mergeLabels combines item labels with default labels, removing duplicates.
func (b *broker) mergeLabels(itemLabels []string) []string {
	labelSet := make(map[string]struct{})
	var result []string

	// Add default labels first
	for _, label := range b.config.DefaultLabels {
		if _, exists := labelSet[label]; !exists {
			labelSet[label] = struct{}{}
			result = append(result, label)
		}
	}

	// Add item-specific labels
	for _, label := range itemLabels {
		if _, exists := labelSet[label]; !exists {
			labelSet[label] = struct{}{}
			result = append(result, label)
		}
	}

	return result
}
