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

// Logger defines the logging interface used by the broker.
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

// New returns a broker implementation with the given provider and notifier.
// Returns an error if provider or notifier is nil to fail fast on misconfiguration.
func New(provider Provider, notifier Notifier, config Config, logger Logger) Broker {
	if provider == nil {
		panic("broker.New: provider cannot be nil (use NewStub for testing)")
	}
	if notifier == nil {
		panic("broker.New: notifier cannot be nil (use NewStub for testing)")
	}
	if logger == nil {
		panic("broker.New: logger cannot be nil")
	}
	return &broker{
		provider: provider,
		notifier: notifier,
		config:   config,
		logger:   logger,
	}
}

// noOpLogger is a no-op implementation of Logger for stub brokers.
type noOpLogger struct{}

func (noOpLogger) Debug(msg string, args ...any) {}
func (noOpLogger) Info(msg string, args ...any)  {}
func (noOpLogger) Warn(msg string, args ...any)  {}
func (noOpLogger) Error(msg string, args ...any) {}

// NewStub returns a stub broker implementation for testing.
func NewStub() Broker {
	return &broker{
		logger: noOpLogger{},
	}
}

type broker struct {
	provider Provider
	notifier Notifier
	config   Config
	logger   Logger
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

	// Skip PR creation gracefully for failed execution results
	if result != nil && result.Status == executor.StatusFailed {
		// Log the failure but don't return an error to allow orchestration to continue
		b.logger.Info("Skipping PR creation for failed execution", "module", item.Module, "repo", item.Repo, "reason", result.Reason)
		return nil, nil
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
		Labels:     SanitizeLabels(b.mergeLabels(item.Labels)),
	}

	// Validate PR input before sending to provider
	if err := ValidatePRInput(&prInput); err != nil {
		return nil, fmt.Errorf("PR input validation failed: %w", err)
	}

	// Create or update the pull request
	pr, err := b.provider.CreateOrUpdatePullRequest(ctx, prInput)
	if err != nil {
		return nil, fmt.Errorf("create or update PR: %w", err)
	}

	// Note: Labels are applied during PR creation, no need for separate AddLabels call

	// Request reviewers if configured
	if len(item.PR.Reviewers) > 0 || len(item.PR.TeamReviewers) > 0 {
		// Sanitize reviewer lists
		sanitizedReviewers := SanitizeLabels(item.PR.Reviewers)
		sanitizedTeamReviewers := SanitizeLabels(item.PR.TeamReviewers)

		if err := b.provider.RequestReviewers(ctx, item.Repo, pr.Number, sanitizedReviewers, sanitizedTeamReviewers); err != nil {
			// Don't fail the whole operation for reviewer errors
			b.logger.Warn("Failed to request reviewers", "module", item.Module, "repo", item.Repo, "reviewers", sanitizedReviewers, "team_reviewers", sanitizedTeamReviewers, "error", err)
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

	if pr == nil {
		return fmt.Errorf("pull request cannot be nil")
	}

	if body == "" {
		return fmt.Errorf("comment body cannot be empty")
	}

	// Add comment via provider
	if err := b.provider.AddComment(ctx, pr.Repo, pr.Number, body); err != nil {
		return fmt.Errorf("failed to add comment to PR #%d in %s: %w", pr.Number, pr.Repo, err)
	}

	return nil
}

func (b *broker) Notify(ctx context.Context, item planner.WorkItem, result *executor.Result) (*NotificationResult, error) {
	// In dry-run mode, skip actual notifications
	if b.config.DryRun {
		return nil, nil
	}

	if b.notifier == nil {
		return nil, &NotImplementedError{Operation: "broker.Notify"}
	}

	// Send notification - failures shouldn't block main PR flow
	notificationResult, err := b.notifier.Send(ctx, item, result)
	if err != nil {
		// Log notification failures but don't fail the operation
		b.logger.Warn("Notification failed", "module", item.Module, "repo", item.Repo, "error", err)
		return nil, nil // Return nil so PR creation continues
	}

	// Log when notifications are intentionally disabled
	if notificationResult != nil && notificationResult.Channel == "noop" {
		b.logger.Info("Notifications disabled", "module", item.Module, "repo", item.Repo, "message", notificationResult.Message)
	}

	return notificationResult, nil
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
