package commenter

import (
	"context"
	"errors"

	github "github.com/google/go-github/v66/github"
)

// GoGitHubClient wraps go-github to satisfy the commenter GitHubClient interface.
type GoGitHubClient struct {
	client *github.Client
}

// NewGoGitHubClient instantiates the wrapper.
func NewGoGitHubClient(client *github.Client) *GoGitHubClient {
	return &GoGitHubClient{client: client}
}

func (g *GoGitHubClient) ListComments(ctx context.Context, owner, repo string, number int) ([]Comment, error) {
	if g.client == nil {
		return nil, errors.New("github client not configured")
	}
	comments, _, err := g.client.Issues.ListComments(ctx, owner, repo, number, nil)
	if err != nil {
		return nil, err
	}
	var result []Comment
	for _, comment := range comments {
		if comment == nil {
			continue
		}
		result = append(result, Comment{
			ID:   comment.GetID(),
			Body: comment.GetBody(),
			URL:  comment.GetHTMLURL(),
		})
	}
	return result, nil
}

func (g *GoGitHubClient) CreateComment(ctx context.Context, owner, repo string, number int, body string) (Comment, error) {
	if g.client == nil {
		return Comment{}, errors.New("github client not configured")
	}
	created, _, err := g.client.Issues.CreateComment(ctx, owner, repo, number, &github.IssueComment{Body: &body})
	if err != nil {
		return Comment{}, err
	}
	return Comment{ID: created.GetID(), Body: created.GetBody(), URL: created.GetHTMLURL()}, nil
}

func (g *GoGitHubClient) UpdateComment(ctx context.Context, owner, repo string, id int64, body string) (Comment, error) {
	if g.client == nil {
		return Comment{}, errors.New("github client not configured")
	}
	updated, _, err := g.client.Issues.EditComment(ctx, owner, repo, id, &github.IssueComment{Body: &body})
	if err != nil {
		return Comment{}, err
	}
	return Comment{ID: updated.GetID(), Body: updated.GetBody(), URL: updated.GetHTMLURL()}, nil
}
