package notifier

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/goliatone/cascade/internal/depregistration"
)

// ActionType represents the type of notification action taken for a dependency.
type ActionType string

const (
	// ActionWorkflowDispatched indicates the dependency workflow was dispatched.
	ActionWorkflowDispatched ActionType = "workflow_dispatched"
	// ActionPullRequestCreated indicates a new PR was opened.
	ActionPullRequestCreated ActionType = "pull_request_created"
	// ActionPullRequestUpdated indicates an existing PR was reused.
	ActionPullRequestUpdated ActionType = "pull_request_updated"
	// ActionIssueCreated indicates a new issue was opened.
	ActionIssueCreated ActionType = "issue_created"
	// ActionIssueUpdated indicates an existing issue was reused.
	ActionIssueUpdated ActionType = "issue_updated"
	// ActionDryRun indicates no action was taken due to dry-run mode.
	ActionDryRun ActionType = "dry_run"
)

// Request captures the information required to notify a dependency repository.
type Request struct {
	Dependency      depregistration.DependencyDelta
	ConsumingRepo   string
	ConsumingModule string
	DependencyRepo  string
	WorkflowFile    string
	WorkflowInputs  map[string]string
	IssueLabels     []string
	BranchName      string
	BaseBranch      string
	DryRun          bool
	CommentTag      string
	Now             func() time.Time
}

// ActionResult captures the outcome of the notifier.
type ActionResult struct {
	Dependency depregistration.DependencyDelta `json:"dependency"`
	Action     ActionType                      `json:"action"`
	URL        string                          `json:"url,omitempty"`
	Notes      string                          `json:"notes,omitempty"`
	Reused     bool                            `json:"reused"`
}

// PullRequestInput describes the PR automation flow.
type PullRequestInput struct {
	Title       string
	Body        string
	HeadBranch  string
	BaseBranch  string
	CommitTitle string
	CommitBody  string
	Files       []PullRequestFile
}

// PullRequestFile represents a file mutation in the generated PR.
type PullRequestFile struct {
	Path     string
	Contents []byte
}

// PullRequestResult captures the PR outcome.
type PullRequestResult struct {
	URL     string
	Created bool
}

// IssueInput describes the requested issue payload.
type IssueInput struct {
	Title  string
	Body   string
	Labels []string
	Tag    string
	Reopen bool
}

// IssueResult captures the issue outcome.
type IssueResult struct {
	URL     string
	Created bool
}

// GitHubClient abstracts calls to the GitHub API used by the notifier.
type GitHubClient interface {
	HasFile(ctx context.Context, owner, repo, path, ref string) (bool, error)
	DispatchWorkflow(ctx context.Context, owner, repo, workflow, ref string, inputs map[string]string) error
	EnsurePullRequest(ctx context.Context, owner, repo string, input PullRequestInput) (*PullRequestResult, error)
	EnsureIssue(ctx context.Context, owner, repo string, input IssueInput) (*IssueResult, error)
}

// Notifier invokes GitHub automation for new dependencies.
type Notifier interface {
	Notify(ctx context.Context, req Request) (*ActionResult, error)
}

// GitHubNotifier is the default Notification orchestrator backed by GitHub.
type GitHubNotifier struct {
	client GitHubClient
}

// NewGitHubNotifier constructs a new notifier instance.
func NewGitHubNotifier(client GitHubClient) *GitHubNotifier {
	return &GitHubNotifier{client: client}
}

// Notify implements the Notifier interface.
func (n *GitHubNotifier) Notify(ctx context.Context, req Request) (*ActionResult, error) {
	if n.client == nil {
		return nil, errors.New("notifier client is nil")
	}

	if err := validateRequest(req); err != nil {
		return nil, err
	}

	action := &ActionResult{Dependency: req.Dependency}

	if req.DryRun {
		action.Action = ActionDryRun
		action.Notes = "dry-run enabled; no GitHub actions executed"
		return action, nil
	}

	owner, repo := splitRepo(req.DependencyRepo)
	workflow := req.WorkflowFile
	if workflow == "" {
		workflow = "cascade-release.yml"
	}

	inputs := map[string]string{}
	for k, v := range req.WorkflowInputs {
		inputs[k] = v
	}
	inputs["consuming_repo"] = req.ConsumingRepo
	if req.ConsumingModule != "" {
		inputs["consuming_module"] = req.ConsumingModule
	}
	inputs["dependency_module"] = req.Dependency.Module

	ref := req.BaseBranch
	if ref == "" {
		ref = "main"
	}

	hasCascade, err := n.client.HasFile(ctx, owner, repo, ".cascade.yaml", ref)
	if err != nil {
		return nil, fmt.Errorf("probe cascade file: %w", err)
	}

	if hasCascade {
		if err := n.client.DispatchWorkflow(ctx, owner, repo, workflow, ref, inputs); err == nil {
			action.Action = ActionWorkflowDispatched
			action.Notes = fmt.Sprintf("workflow %s dispatched", workflow)
			return action, nil
		}

		prInput := defaultPullRequestInput(req)
		prResult, prErr := n.client.EnsurePullRequest(ctx, owner, repo, prInput)
		if prErr == nil {
			action.URL = prResult.URL
			action.Reused = !prResult.Created
			if prResult.Created {
				action.Action = ActionPullRequestCreated
				action.Notes = "opened dependency registration PR"
			} else {
				action.Action = ActionPullRequestUpdated
				action.Notes = "reused dependency registration PR"
			}
			return action, nil
		}
		action.Notes = fmt.Sprintf("pull request fallback failed: %v", prErr)
	}

	issueInput := defaultIssueInput(req, action.Notes)
	issueResult, issueErr := n.client.EnsureIssue(ctx, owner, repo, issueInput)
	if issueErr != nil {
		return nil, fmt.Errorf("ensure dependency issue: %w", issueErr)
	}

	action.URL = issueResult.URL
	action.Reused = !issueResult.Created
	if issueResult.Created {
		action.Action = ActionIssueCreated
		action.Notes = "opened dependency onboarding issue"
	} else {
		action.Action = ActionIssueUpdated
		action.Notes = "updated dependency onboarding issue"
	}

	return action, nil
}

func validateRequest(req Request) error {
	if req.ConsumingRepo == "" {
		return errors.New("consuming repo is required")
	}
	if req.DependencyRepo == "" {
		return errors.New("dependency repo is required")
	}
	if req.Dependency.Module == "" {
		return errors.New("dependency module is required")
	}
	return nil
}

func splitRepo(repo string) (string, string) {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return "", repo
	}
	return parts[0], parts[1]
}

func defaultPullRequestInput(req Request) PullRequestInput {
	branch := req.BranchName
	if branch == "" {
		branch = fmt.Sprintf("automation/dependency-registration/%s", sanitizeBranch(req.ConsumingRepo))
	}

	return PullRequestInput{
		Title:       fmt.Sprintf("chore: register %s as dependent", req.ConsumingRepo),
		Body:        buildPullRequestBody(req),
		HeadBranch:  branch,
		BaseBranch:  req.BaseBranch,
		CommitTitle: fmt.Sprintf("chore: register %s as dependent", req.ConsumingRepo),
		CommitBody:  "", // manifest mutation handled by client implementation.
	}
}

func defaultIssueInput(req Request, previousNote string) IssueInput {
	bodyLines := []string{
		fmt.Sprintf("We detected a new internal dependency: **%s** is now consumed by **%s**.", req.Dependency.Module, req.ConsumingRepo),
		"",
		"Please ensure `.cascade.yaml` includes this repository or trigger your dependency registration workflow.",
	}
	if previousNote != "" {
		bodyLines = append(bodyLines, "", fmt.Sprintf("Previous attempt details: %s", previousNote))
	}
	return IssueInput{
		Title:  fmt.Sprintf("Adopt Cascade for new dependent %s", req.ConsumingRepo),
		Body:   strings.Join(bodyLines, "\n"),
		Labels: req.IssueLabels,
		Tag:    req.CommentTag,
	}
}

func sanitizeBranch(repo string) string {
	replacer := strings.NewReplacer(" ", "-", ".", "-", ":", "-", "@", "-", "#", "-", "?", "-", "=", "-", "+", "-", ",", "-", "*", "-", "[", "-", "]", "-")
	return replacer.Replace(strings.ToLower(repo))
}

func buildPullRequestBody(req Request) string {
	var builder strings.Builder
	builder.WriteString("## Dependency Registration\n\n")
	builder.WriteString("We detected a new internal consumer via Cascade.\n\n")
	builder.WriteString("- Consuming repository: ")
	builder.WriteString(req.ConsumingRepo)
	builder.WriteString("\n")
	if req.ConsumingModule != "" {
		builder.WriteString("- Consuming module: ")
		builder.WriteString(req.ConsumingModule)
		builder.WriteString("\n")
	}
	builder.WriteString("- Dependency module: ")
	builder.WriteString(req.Dependency.Module)
	if req.Dependency.Version != "" {
		builder.WriteString(" (version ")
		builder.WriteString(req.Dependency.Version)
		builder.WriteString(")")
	}
	builder.WriteString("\n\n")
	builder.WriteString("Please ensure `.cascade.yaml` lists this repository so cascade can orchestrate future releases.")
	return builder.String()
}
