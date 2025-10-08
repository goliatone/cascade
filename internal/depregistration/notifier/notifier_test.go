package notifier

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/goliatone/cascade/internal/depregistration"
)

func TestGitHubNotifier_DispatchWorkflow(t *testing.T) {
	client := &fakeGitHubClient{
		hasFile: true,
	}
	ctx := context.Background()

	request := Request{
		Dependency:      depregistration.DependencyDelta{Module: "github.com/goliatone/go-logger"},
		ConsumingRepo:   "goliatone/go-errors",
		ConsumingModule: "github.com/goliatone/go-errors",
		DependencyRepo:  "goliatone/go-logger",
		WorkflowFile:    "cascade-release.yml",
	}

	n := NewGitHubNotifier(client)
	result, err := n.Notify(ctx, request)
	if err != nil {
		t.Fatalf("notify: %v", err)
	}

	if diff := cmp.Diff(ActionWorkflowDispatched, result.Action); diff != "" {
		t.Fatalf("unexpected action (-want +got):\n%s", diff)
	}

	if client.dispatchedWorkflow != 1 {
		t.Fatalf("expected workflow dispatch, got %d", client.dispatchedWorkflow)
	}
}

func TestGitHubNotifier_FallbackToPullRequest(t *testing.T) {
	client := &fakeGitHubClient{
		hasFile:           true,
		dispatchErr:       errors.New("workflow fail"),
		pullRequestResult: &PullRequestResult{URL: "https://github.com/goliatone/go-logger/pull/42", Created: true},
	}

	n := NewGitHubNotifier(client)
	result, err := n.Notify(context.Background(), Request{
		Dependency:     depregistration.DependencyDelta{Module: "github.com/goliatone/go-logger"},
		ConsumingRepo:  "goliatone/go-errors",
		DependencyRepo: "goliatone/go-logger",
	})
	if err != nil {
		t.Fatalf("notify: %v", err)
	}

	if result.Action != ActionPullRequestCreated {
		t.Fatalf("expected pull request action, got %s", result.Action)
	}

	if client.pullRequests != 1 {
		t.Fatalf("expected single pull request invocation, got %d", client.pullRequests)
	}
}

func TestGitHubNotifier_FallbackToIssue(t *testing.T) {
	client := &fakeGitHubClient{
		hasFile:     false,
		issueResult: &IssueResult{URL: "https://github.com/goliatone/go-logger/issues/99", Created: true},
	}

	result, err := NewGitHubNotifier(client).Notify(context.Background(), Request{
		Dependency:     depregistration.DependencyDelta{Module: "github.com/goliatone/go-logger"},
		ConsumingRepo:  "goliatone/go-errors",
		DependencyRepo: "goliatone/go-logger",
	})
	if err != nil {
		t.Fatalf("notify: %v", err)
	}

	if result.Action != ActionIssueCreated {
		t.Fatalf("expected issue action, got %s", result.Action)
	}

	if client.issues != 1 {
		t.Fatalf("expected issue creation, got %d", client.issues)
	}
}

func TestGitHubNotifier_PullRequestFailureFollowsIssue(t *testing.T) {
	client := &fakeGitHubClient{
		hasFile:          true,
		dispatchErr:      errors.New("no workflow"),
		pullRequestError: errors.New("no permission"),
		issueResult:      &IssueResult{URL: "https://github.com/goliatone/go-logger/issues/10", Created: false},
	}

	result, err := NewGitHubNotifier(client).Notify(context.Background(), Request{
		Dependency:     depregistration.DependencyDelta{Module: "github.com/goliatone/go-logger"},
		ConsumingRepo:  "goliatone/go-errors",
		DependencyRepo: "goliatone/go-logger",
	})
	if err != nil {
		t.Fatalf("notify: %v", err)
	}

	if result.Action != ActionIssueUpdated {
		t.Fatalf("expected issue updated action, got %s", result.Action)
	}

	if client.issues != 1 {
		t.Fatalf("expected issue invocation, got %d", client.issues)
	}
}

func TestGitHubNotifier_DryRun(t *testing.T) {
	client := &fakeGitHubClient{}

	result, err := NewGitHubNotifier(client).Notify(context.Background(), Request{
		Dependency:     depregistration.DependencyDelta{Module: "github.com/goliatone/go-logger"},
		ConsumingRepo:  "goliatone/go-errors",
		DependencyRepo: "goliatone/go-logger",
		DryRun:         true,
	})
	if err != nil {
		t.Fatalf("notify: %v", err)
	}

	if result.Action != ActionDryRun {
		t.Fatalf("expected dry run action, got %s", result.Action)
	}

	if client.dispatchedWorkflow != 0 || client.pullRequests != 0 || client.issues != 0 {
		t.Fatalf("expected no client calls during dry run")
	}
}

type fakeGitHubClient struct {
	// configuration
	hasFile           bool
	hasFileErr        error
	dispatchErr       error
	pullRequestResult *PullRequestResult
	pullRequestError  error
	issueResult       *IssueResult
	issueError        error

	// metrics
	dispatchedWorkflow int
	pullRequests       int
	issues             int
}

func (f *fakeGitHubClient) HasFile(ctx context.Context, owner, repo, path, ref string) (bool, error) {
	return f.hasFile, f.hasFileErr
}

func (f *fakeGitHubClient) DispatchWorkflow(ctx context.Context, owner, repo, workflow, ref string, inputs map[string]string) error {
	f.dispatchedWorkflow++
	return f.dispatchErr
}

func (f *fakeGitHubClient) EnsurePullRequest(ctx context.Context, owner, repo string, input PullRequestInput) (*PullRequestResult, error) {
	f.pullRequests++
	if f.pullRequestError != nil {
		return nil, f.pullRequestError
	}
	if f.pullRequestResult == nil {
		return &PullRequestResult{URL: "", Created: true}, nil
	}
	return f.pullRequestResult, nil
}

func (f *fakeGitHubClient) EnsureIssue(ctx context.Context, owner, repo string, input IssueInput) (*IssueResult, error) {
	f.issues++
	if f.issueError != nil {
		return nil, f.issueError
	}
	if f.issueResult == nil {
		return &IssueResult{URL: "", Created: true}, nil
	}
	return f.issueResult, nil
}
