package commenter

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/goliatone/cascade/internal/depregistration"
	"github.com/goliatone/cascade/internal/depregistration/notifier"
)

func TestCommenter_CreatesCommentWhenMissing(t *testing.T) {
	client := &fakeGitHubClient{}
	commenter := New(client)

	results := []notifier.ActionResult{
		{
			Dependency: depregistration.DependencyDelta{Module: "github.com/goliatone/go-logger"},
			Action:     notifier.ActionWorkflowDispatched,
			Notes:      "workflow dispatched",
			URL:        "https://github.com/goliatone/go-logger/actions/runs/1",
		},
	}

	resp, err := commenter.Upsert(context.Background(), Request{
		Repository: "goliatone/go-errors",
		PRNumber:   12,
		Results:    results,
		RunURL:     "https://github.com/goliatone/go-errors/actions/runs/99",
		Now: func() time.Time {
			return time.Date(2024, time.January, 1, 10, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	if !resp.Created {
		t.Fatalf("expected comment creation")
	}

	if len(client.createdBodies) != 1 {
		t.Fatalf("expected a single created comment, got %d", len(client.createdBodies))
	}

	body := client.createdBodies[0]
	if !containsTag(body) {
		t.Fatalf("comment body missing tag: %s", body)
	}

	if diff := cmp.Diff(1, len(client.createdBodies)); diff != "" {
		t.Fatalf("unexpected created count diff: %s", diff)
	}
}

func TestCommenter_UpdatesExistingComment(t *testing.T) {
	existing := Comment{ID: 42, Body: commentTag + "\nold"}
	client := &fakeGitHubClient{existing: []Comment{existing}}
	commenter := New(client)

	resp, err := commenter.Upsert(context.Background(), Request{
		Repository: "goliatone/go-errors",
		PRNumber:   12,
		Results: []notifier.ActionResult{
			{
				Dependency: depregistration.DependencyDelta{Module: "github.com/goliatone/go-logger"},
				Action:     notifier.ActionIssueCreated,
				Notes:      "created issue",
			},
		},
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	if resp.Created {
		t.Fatalf("expected update, not creation")
	}

	if len(client.updatedBodies) != 1 {
		t.Fatalf("expected a single update, got %d", len(client.updatedBodies))
	}

	if client.updatedIDs[0] != existing.ID {
		t.Fatalf("expected update of comment %d, got %d", existing.ID, client.updatedIDs[0])
	}
}

func TestRenderCommentBody_EmptyResults(t *testing.T) {
	body := renderCommentBody(Request{
		Results: nil,
		Now: func() time.Time {
			return time.Date(2024, 2, 2, 0, 0, 0, 0, time.UTC)
		},
	})

	if !containsTag(body) {
		t.Fatalf("expected tag in body: %s", body)
	}

	if !strings.Contains(body, "_(none)_") {
		t.Fatalf("expected placeholder row for empty results: %s", body)
	}
}

func containsTag(body string) bool {
	return strings.Contains(body, commentTag)
}

type fakeGitHubClient struct {
	existing      []Comment
	createdBodies []string
	updatedBodies []string
	updatedIDs    []int64
}

func (f *fakeGitHubClient) ListComments(ctx context.Context, owner, repo string, number int) ([]Comment, error) {
	return append([]Comment(nil), f.existing...), nil
}

func (f *fakeGitHubClient) CreateComment(ctx context.Context, owner, repo string, number int, body string) (Comment, error) {
	f.createdBodies = append(f.createdBodies, body)
	comment := Comment{ID: int64(len(f.createdBodies))}
	return comment, nil
}

func (f *fakeGitHubClient) UpdateComment(ctx context.Context, owner, repo string, id int64, body string) (Comment, error) {
	f.updatedBodies = append(f.updatedBodies, body)
	f.updatedIDs = append(f.updatedIDs, id)
	return Comment{ID: id}, nil
}
