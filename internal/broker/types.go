package broker

import (
	"context"

	"github.com/goliatone/cascade/internal/executor"
	"github.com/goliatone/cascade/internal/planner"
)

// Broker orchestrates pull-request creation and notifications.
type Broker interface {
	EnsurePR(ctx context.Context, item planner.WorkItem, result *executor.Result) (*PullRequest, error)
	Comment(ctx context.Context, pr *PullRequest, body string) error
	Notify(ctx context.Context, item planner.WorkItem, result *executor.Result) (*NotificationResult, error)
}

// PullRequest represents metadata returned from the provider.
type PullRequest struct {
	URL    string
	Number int
	Repo   string
	Labels []string
}

// PRInput stores payload data sent to the provider when creating/updating a PR.
type PRInput struct {
	Repo       string
	BaseBranch string
	HeadBranch string
	Title      string
	Body       string
	Labels     []string
}

// NotificationResult holds notification metadata (e.g. Slack message IDs).
type NotificationResult struct {
	Channel string
	Message string
}
