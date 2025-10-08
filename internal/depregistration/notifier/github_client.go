package notifier

import (
	"context"
	"errors"
	"strings"

	github "github.com/google/go-github/v66/github"
)

// ErrPullRequestNotSupported indicates automated PR creation is not yet implemented.
var ErrPullRequestNotSupported = errors.New("pull request automation not supported")

// GoGitHubClient implements GitHubClient using the go-github library.
type GoGitHubClient struct {
	client *github.Client
}

// NewGoGitHubClient builds a client wrapper around go-github.
func NewGoGitHubClient(client *github.Client) *GoGitHubClient {
	return &GoGitHubClient{client: client}
}

// HasFile checks for the presence of a path at a ref.
func (c *GoGitHubClient) HasFile(ctx context.Context, owner, repo, path, ref string) (bool, error) {
	if c.client == nil {
		return false, errors.New("github client not configured")
	}
	opts := &github.RepositoryContentGetOptions{}
	if ref != "" {
		opts.Ref = ref
	}
	_, dir, resp, err := c.client.Repositories.GetContents(ctx, owner, repo, path, opts)
	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			return false, nil
		}
		return false, err
	}
	return dir != nil || err == nil, nil
}

// DispatchWorkflow triggers a workflow dispatch event.
func (c *GoGitHubClient) DispatchWorkflow(ctx context.Context, owner, repo, workflow, ref string, inputs map[string]string) error {
	if c.client == nil {
		return errors.New("github client not configured")
	}
	request := github.CreateWorkflowDispatchEventRequest{
		Ref:    ref,
		Inputs: map[string]interface{}{},
	}
	if request.Ref == "" {
		request.Ref = "main"
	}
	for k, v := range inputs {
		request.Inputs[k] = v
	}
	_, err := c.client.Actions.CreateWorkflowDispatchEventByFileName(ctx, owner, repo, workflow, request)
	return err
}

// EnsurePullRequest currently returns a not-supported error. The orchestration
// layer will fall back to issues when this happens. Future work can populate
// this by generating manifest updates via the Git data API.
func (c *GoGitHubClient) EnsurePullRequest(ctx context.Context, owner, repo string, input PullRequestInput) (*PullRequestResult, error) {
	return nil, ErrPullRequestNotSupported
}

// EnsureIssue opens or updates a dependency issue.
func (c *GoGitHubClient) EnsureIssue(ctx context.Context, owner, repo string, input IssueInput) (*IssueResult, error) {
	if c.client == nil {
		return nil, errors.New("github client not configured")
	}

	issues, _, err := c.client.Issues.ListByRepo(ctx, owner, repo, &github.IssueListByRepoOptions{
		State:       "open",
		ListOptions: github.ListOptions{PerPage: 100},
	})
	if err != nil {
		return nil, err
	}

	tag := strings.TrimSpace(input.Tag)
	for _, issue := range issues {
		if issue == nil || issue.Title == nil {
			continue
		}
		if strings.EqualFold(*issue.Title, input.Title) || (tag != "" && strings.Contains(issue.GetBody(), tag)) {
			update := &github.IssueRequest{
				Body: &input.Body,
			}
			if len(input.Labels) > 0 {
				update.Labels = &input.Labels
			}
			_, _, err := c.client.Issues.Edit(ctx, owner, repo, issue.GetNumber(), update)
			if err != nil {
				return nil, err
			}
			return &IssueResult{URL: issue.GetHTMLURL(), Created: false}, nil
		}
	}

	issueRequest := &github.IssueRequest{
		Title:  github.String(input.Title),
		Body:   github.String(input.Body),
		Labels: &input.Labels,
	}

	issue, _, err := c.client.Issues.Create(ctx, owner, repo, issueRequest)
	if err != nil {
		return nil, err
	}

	return &IssueResult{URL: issue.GetHTMLURL(), Created: true}, nil
}
