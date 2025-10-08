package commenter

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/goliatone/cascade/internal/depregistration/notifier"
)

const (
	commentTag      = "<!-- dependency-registration -->"
	commentHeadline = "### Dependency Registration Summary"
)

// Request captures the inputs required to upsert a dependency registration comment.
type Request struct {
	Repository string
	PRNumber   int
	Results    []notifier.ActionResult
	RunURL     string
	Now        func() time.Time
}

// Response captures metadata returned from the commenter.
type Response struct {
	CommentID int64
	Created   bool
}

// GitHubClient defines the GitHub comment operations used by the commenter.
type GitHubClient interface {
	ListComments(ctx context.Context, owner, repo string, number int) ([]Comment, error)
	CreateComment(ctx context.Context, owner, repo string, number int, body string) (Comment, error)
	UpdateComment(ctx context.Context, owner, repo string, id int64, body string) (Comment, error)
}

// Comment represents a GitHub issue comment.
type Comment struct {
	ID   int64
	Body string
	URL  string
}

// Commenter upserts dependency registration comments on pull requests.
type Commenter struct {
	client GitHubClient
}

// New creates a Commenter with the provided GitHub client.
func New(client GitHubClient) *Commenter {
	return &Commenter{client: client}
}

// Upsert ensures a dependency registration comment is present and up to date.
func (c *Commenter) Upsert(ctx context.Context, req Request) (*Response, error) {
	if c.client == nil {
		return nil, fmt.Errorf("commenter client is nil")
	}

	owner, repo, err := splitRepo(req.Repository)
	if err != nil {
		return nil, err
	}

	body := renderCommentBody(req)

	existing, err := c.client.ListComments(ctx, owner, repo, req.PRNumber)
	if err != nil {
		return nil, err
	}

	for _, comment := range existing {
		if strings.Contains(comment.Body, commentTag) {
			updated, err := c.client.UpdateComment(ctx, owner, repo, comment.ID, body)
			if err != nil {
				return nil, err
			}
			return &Response{CommentID: updated.ID, Created: false}, nil
		}
	}

	created, err := c.client.CreateComment(ctx, owner, repo, req.PRNumber, body)
	if err != nil {
		return nil, err
	}

	return &Response{CommentID: created.ID, Created: true}, nil
}

func renderCommentBody(req Request) string {
	results := append([]notifier.ActionResult{}, req.Results...)
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].Dependency.Module < results[j].Dependency.Module
	})

	now := time.Now().UTC()
	if req.Now != nil {
		now = req.Now()
	}

	var builder strings.Builder
	builder.WriteString(commentTag)
	builder.WriteString("\n\n")
	builder.WriteString(commentHeadline)
	builder.WriteString("\n\n")
	builder.WriteString(fmt.Sprintf("_Last updated: %s_", now.Format(time.RFC3339)))

	if req.RunURL != "" {
		builder.WriteString(" • ")
		builder.WriteString(fmt.Sprintf("[Workflow Run](%s)", req.RunURL))
	}

	builder.WriteString("\n\n")
	builder.WriteString("| Module | Action | Details |\n")
	builder.WriteString("| --- | --- | --- |\n")

	if len(results) == 0 {
		builder.WriteString("| _(none)_ | - | No new dependencies detected |\n")
	} else {
		for _, res := range results {
			module := res.Dependency.Module
			if module == "" {
				module = "(unknown)"
			}
			action := string(res.Action)
			details := formatDetails(res)
			builder.WriteString(fmt.Sprintf("| `%s` | %s | %s |\n", module, action, details))
		}
	}

	return builder.String()
}

func formatDetails(res notifier.ActionResult) string {
	detail := strings.TrimSpace(res.Notes)
	if res.URL != "" {
		target := res.URL
		if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
			target = "https://" + strings.TrimPrefix(target, "//")
		}
		link := fmt.Sprintf("[View](%s)", target)
		if detail == "" {
			return link
		}
		return detail + " " + link
	}
	if detail == "" {
		return "-"
	}
	return detail
}

func splitRepo(repo string) (string, string, error) {
	parts := strings.Split(repo, "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid repository identifier: %s", repo)
	}
	return parts[0], parts[1], nil
}
