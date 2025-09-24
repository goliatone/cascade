package broker

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/go-github/v66/github"
)

// Provider defines the interface for GitHub operations.
type Provider interface {
	CreateOrUpdatePullRequest(ctx context.Context, input PRInput) (*PullRequest, error)
	AddLabels(ctx context.Context, repo string, number int, labels []string) error
	RequestReviewers(ctx context.Context, repo string, number int, reviewers []string, teamReviewers []string) error
	ListPullRequests(ctx context.Context, repo string, headBranch string) ([]*PullRequest, error)
	AddComment(ctx context.Context, repo string, number int, body string) error
}

// GitHubProvider implements the Provider interface using the GitHub API.
type GitHubProvider struct {
	client *github.Client
}

// NewGitHubProvider creates a new GitHub provider with the given client.
func NewGitHubProvider(client *github.Client) Provider {
	return &GitHubProvider{
		client: client,
	}
}

// CreateOrUpdatePullRequest creates a new pull request or updates an existing one.
func (p *GitHubProvider) CreateOrUpdatePullRequest(ctx context.Context, input PRInput) (*PullRequest, error) {
	owner, repo, err := ParseRepoString(input.Repo)
	if err != nil {
		return nil, fmt.Errorf("invalid repository format %q: %w", input.Repo, err)
	}

	// Check for existing PR with the same head branch
	existingPR, err := p.findExistingPR(ctx, owner, repo, input.HeadBranch)
	if err != nil {
		return nil, fmt.Errorf("failed to find existing PR: %w", err)
	}

	if existingPR != nil {
		// Update existing PR
		updatePR := &github.PullRequest{
			Title: &input.Title,
			Body:  &input.Body,
		}

		updatedPR, _, err := p.client.PullRequests.Edit(ctx, owner, repo, existingPR.GetNumber(), updatePR)
		if err != nil {
			return nil, &GitHubAPIError{
				Operation: "update pull request",
				Repo:      input.Repo,
				Err:       err,
			}
		}

		if err := p.ensureLabels(ctx, input.Repo, updatedPR.GetNumber(), updatedPR, input.Labels); err != nil {
			return nil, err
		}

		return &PullRequest{
			URL:    updatedPR.GetHTMLURL(),
			Number: updatedPR.GetNumber(),
			Repo:   input.Repo,
			Labels: input.Labels,
		}, nil
	}

	// Create new PR
	newPR := &github.NewPullRequest{
		Title: &input.Title,
		Head:  &input.HeadBranch,
		Base:  &input.BaseBranch,
		Body:  &input.Body,
	}

	createdPR, _, err := p.client.PullRequests.Create(ctx, owner, repo, newPR)
	if err != nil {
		return nil, &GitHubAPIError{
			Operation: "create pull request",
			Repo:      input.Repo,
			Err:       err,
		}
	}

	if err := p.ensureLabels(ctx, input.Repo, createdPR.GetNumber(), createdPR, input.Labels); err != nil {
		return nil, err
	}

	return &PullRequest{
		URL:    createdPR.GetHTMLURL(),
		Number: createdPR.GetNumber(),
		Repo:   input.Repo,
		Labels: input.Labels,
	}, nil
}

// AddLabels adds labels to a pull request.
func (p *GitHubProvider) AddLabels(ctx context.Context, repo string, number int, labels []string) error {
	if len(labels) == 0 {
		return nil
	}

	owner, repoName, err := ParseRepoString(repo)
	if err != nil {
		return fmt.Errorf("invalid repository format %q: %w", repo, err)
	}

	_, _, err = p.client.Issues.AddLabelsToIssue(ctx, owner, repoName, number, labels)
	if err != nil {
		return &GitHubAPIError{
			Operation: "add labels",
			Repo:      repo,
			Err:       err,
		}
	}

	return nil
}

// RequestReviewers requests reviewers for a pull request.
func (p *GitHubProvider) RequestReviewers(ctx context.Context, repo string, number int, reviewers []string, teamReviewers []string) error {
	if len(reviewers) == 0 && len(teamReviewers) == 0 {
		return nil
	}

	owner, repoName, err := ParseRepoString(repo)
	if err != nil {
		return fmt.Errorf("invalid repository format %q: %w", repo, err)
	}

	reviewersRequest := github.ReviewersRequest{
		Reviewers:     reviewers,
		TeamReviewers: teamReviewers,
	}

	_, _, err = p.client.PullRequests.RequestReviewers(ctx, owner, repoName, number, reviewersRequest)
	if err != nil {
		return &GitHubAPIError{
			Operation: "request reviewers",
			Repo:      repo,
			Err:       err,
		}
	}

	return nil
}

// ListPullRequests lists pull requests for the given repository and head branch.
func (p *GitHubProvider) ListPullRequests(ctx context.Context, repo string, headBranch string) ([]*PullRequest, error) {
	owner, repoName, err := ParseRepoString(repo)
	if err != nil {
		return nil, fmt.Errorf("invalid repository format %q: %w", repo, err)
	}

	opts := &github.PullRequestListOptions{
		Head:      owner + ":" + headBranch,
		State:     "open",
		Sort:      "created",
		Direction: "desc",
		ListOptions: github.ListOptions{
			PerPage: 10,
		},
	}

	githubPRs, _, err := p.client.PullRequests.List(ctx, owner, repoName, opts)
	if err != nil {
		return nil, &GitHubAPIError{
			Operation: "list pull requests",
			Repo:      repo,
			Err:       err,
		}
	}

	// Convert GitHub PR structs to our PullRequest structs
	var prs []*PullRequest
	for _, githubPR := range githubPRs {
		prs = append(prs, &PullRequest{
			URL:    githubPR.GetHTMLURL(),
			Number: githubPR.GetNumber(),
			Repo:   repo,
			Labels: []string{}, // Note: Labels would need to be fetched separately if needed
		})
	}

	return prs, nil
}

// AddComment adds a comment to a pull request.
func (p *GitHubProvider) AddComment(ctx context.Context, repo string, number int, body string) error {
	owner, repoName, err := ParseRepoString(repo)
	if err != nil {
		return fmt.Errorf("invalid repository format %q: %w", repo, err)
	}

	comment := &github.IssueComment{
		Body: &body,
	}

	_, _, err = p.client.Issues.CreateComment(ctx, owner, repoName, number, comment)
	if err != nil {
		return &GitHubAPIError{
			Operation: "add comment",
			Repo:      repo,
			Err:       err,
		}
	}

	return nil
}

func (p *GitHubProvider) ensureLabels(ctx context.Context, repo string, number int, pr *github.PullRequest, desired []string) error {
	labelsToApply := diffLabels(pr, desired)
	if len(labelsToApply) == 0 {
		return nil
	}
	if err := p.AddLabels(ctx, repo, number, labelsToApply); err != nil {
		return fmt.Errorf("apply labels: %w", err)
	}
	return nil
}

func diffLabels(pr *github.PullRequest, desired []string) []string {
	if len(desired) == 0 {
		return nil
	}
	if pr == nil {
		return desired
	}

	seen := make(map[string]struct{})
	for _, label := range pr.Labels {
		if label == nil {
			continue
		}
		name := label.GetName()
		if name == "" {
			continue
		}
		seen[strings.ToLower(name)] = struct{}{}
	}

	var filtered []string
	for _, label := range desired {
		if label == "" {
			continue
		}
		if _, exists := seen[strings.ToLower(label)]; exists {
			continue
		}
		filtered = append(filtered, label)
	}

	return filtered
}

// findExistingPR searches for an existing PR with the given head branch.
func (p *GitHubProvider) findExistingPR(ctx context.Context, owner, repo, headBranch string) (*github.PullRequest, error) {
	opts := &github.PullRequestListOptions{
		Head:      owner + ":" + headBranch,
		State:     "open",
		Sort:      "created",
		Direction: "desc",
		ListOptions: github.ListOptions{
			PerPage: 10,
		},
	}

	prs, _, err := p.client.PullRequests.List(ctx, owner, repo, opts)
	if err != nil {
		return nil, &GitHubAPIError{
			Operation: "list pull requests",
			Repo:      owner + "/" + repo,
			Err:       err,
		}
	}

	if len(prs) > 0 {
		return prs[0], nil
	}

	return nil, nil
}

// IsRateLimitError checks if an error is a GitHub rate limit error.
func IsRateLimitError(err error) bool {
	var rateLimitError *github.RateLimitError
	return errors.As(err, &rateLimitError)
}

// IsAbuseRateLimitError checks if an error is a GitHub abuse rate limit error.
func IsAbuseRateLimitError(err error) bool {
	var abuseRateLimitError *github.AbuseRateLimitError
	return errors.As(err, &abuseRateLimitError)
}

// ExtractRateLimitInfo extracts rate limit information from a GitHub API response.
func ExtractRateLimitInfo(resp *github.Response) *github.Rate {
	if resp == nil {
		return nil
	}
	return &resp.Rate
}
