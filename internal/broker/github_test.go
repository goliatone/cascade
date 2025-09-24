package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/go-github/v66/github"
)

// fakeRoundTripper implements http.RoundTripper for testing.
type fakeRoundTripper struct {
	responses map[string]*http.Response
}

func (f *fakeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	key := fmt.Sprintf("%s %s", req.Method, req.URL.Path)
	if resp, ok := f.responses[key]; ok {
		return resp, nil
	}
	return &http.Response{
		StatusCode: 404,
		Body:       http.NoBody,
		Header:     make(http.Header),
	}, nil
}

func newTestGitHubProvider(responses map[string]*http.Response) Provider {
	httpClient := &http.Client{
		Transport: &fakeRoundTripper{responses: responses},
	}
	client := github.NewClient(httpClient)
	return NewGitHubProvider(client)
}

func createJSONResponse(statusCode int, body any) *http.Response {
	data, _ := json.Marshal(body)
	return &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(strings.NewReader(string(data))),
		Header:     map[string][]string{"Content-Type": {"application/json"}},
	}
}

func TestGitHubProvider_CreateOrUpdatePullRequest_CreateNew(t *testing.T) {
	responses := map[string]*http.Response{
		"GET /repos/owner/repo/pulls": createJSONResponse(200, []*github.PullRequest{}),
		"POST /repos/owner/repo/pulls": createJSONResponse(201, &github.PullRequest{
			Number:  github.Int(1),
			HTMLURL: github.String("https://github.com/owner/repo/pull/1"),
		}),
		"POST /repos/owner/repo/issues/1/labels": createJSONResponse(200, []*github.Label{}),
	}

	provider := newTestGitHubProvider(responses)
	ctx := context.Background()

	input := PRInput{
		Repo:       "owner/repo",
		BaseBranch: "main",
		HeadBranch: "feature-branch",
		Title:      "Test PR",
		Body:       "Test PR body",
		Labels:     []string{"enhancement"},
	}

	result, err := provider.CreateOrUpdatePullRequest(ctx, input)
	if err != nil {
		t.Fatalf("CreateOrUpdatePullRequest failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected result, got nil")
	}

	if result.URL != "https://github.com/owner/repo/pull/1" {
		t.Errorf("expected URL %q, got %q", "https://github.com/owner/repo/pull/1", result.URL)
	}

	if result.Number != 1 {
		t.Errorf("expected number 1, got %d", result.Number)
	}

	if result.Repo != "owner/repo" {
		t.Errorf("expected repo %q, got %q", "owner/repo", result.Repo)
	}
}

func TestGitHubProvider_CreateOrUpdatePullRequest_UpdateExisting(t *testing.T) {
	existingPR := &github.PullRequest{
		Number:  github.Int(1),
		HTMLURL: github.String("https://github.com/owner/repo/pull/1"),
	}

	responses := map[string]*http.Response{
		"GET /repos/owner/repo/pulls": createJSONResponse(200, []*github.PullRequest{existingPR}),
		"PATCH /repos/owner/repo/pulls/1": createJSONResponse(200, &github.PullRequest{
			Number:  github.Int(1),
			HTMLURL: github.String("https://github.com/owner/repo/pull/1"),
		}),
		"POST /repos/owner/repo/issues/1/labels": createJSONResponse(200, []*github.Label{}),
	}

	provider := newTestGitHubProvider(responses)
	ctx := context.Background()

	input := PRInput{
		Repo:       "owner/repo",
		BaseBranch: "main",
		HeadBranch: "feature-branch",
		Title:      "Updated PR",
		Body:       "Updated PR body",
		Labels:     []string{"enhancement"},
	}

	result, err := provider.CreateOrUpdatePullRequest(ctx, input)
	if err != nil {
		t.Fatalf("CreateOrUpdatePullRequest failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected result, got nil")
	}

	if result.Number != 1 {
		t.Errorf("expected number 1, got %d", result.Number)
	}
}

func TestGitHubProvider_CreateOrUpdatePullRequest_InvalidRepo(t *testing.T) {
	provider := newTestGitHubProvider(map[string]*http.Response{})
	ctx := context.Background()

	input := PRInput{
		Repo:       "invalid-repo-format",
		BaseBranch: "main",
		HeadBranch: "feature-branch",
		Title:      "Test PR",
		Body:       "Test PR body",
	}

	_, err := provider.CreateOrUpdatePullRequest(ctx, input)
	if err == nil {
		t.Fatal("expected error for invalid repo format, got nil")
	}

	if !strings.Contains(err.Error(), "invalid repository format") {
		t.Errorf("expected error about invalid repository format, got: %v", err)
	}
}

func TestGitHubProvider_AddLabels(t *testing.T) {
	responses := map[string]*http.Response{
		"POST /repos/owner/repo/issues/1/labels": createJSONResponse(200, []*github.Label{}),
	}

	provider := newTestGitHubProvider(responses)
	ctx := context.Background()

	err := provider.AddLabels(ctx, "owner/repo", 1, []string{"bug", "enhancement"})
	if err != nil {
		t.Fatalf("AddLabels failed: %v", err)
	}
}

func TestGitHubProvider_AddLabels_NoLabels(t *testing.T) {
	provider := newTestGitHubProvider(map[string]*http.Response{})
	ctx := context.Background()

	err := provider.AddLabels(ctx, "owner/repo", 1, []string{})
	if err != nil {
		t.Fatalf("AddLabels with no labels should not fail, got: %v", err)
	}
}

func TestGitHubProvider_AddLabels_InvalidRepo(t *testing.T) {
	provider := newTestGitHubProvider(map[string]*http.Response{})
	ctx := context.Background()

	err := provider.AddLabels(ctx, "invalid-repo", 1, []string{"bug"})
	if err == nil {
		t.Fatal("expected error for invalid repo format, got nil")
	}

	if !strings.Contains(err.Error(), "invalid repository format") {
		t.Errorf("expected error about invalid repository format, got: %v", err)
	}
}

func TestGitHubProvider_RequestReviewers(t *testing.T) {
	responses := map[string]*http.Response{
		"POST /repos/owner/repo/pulls/1/requested_reviewers": createJSONResponse(201, &github.PullRequest{}),
	}

	provider := newTestGitHubProvider(responses)
	ctx := context.Background()

	err := provider.RequestReviewers(ctx, "owner/repo", 1, []string{"reviewer1"}, []string{"team1"})
	if err != nil {
		t.Fatalf("RequestReviewers failed: %v", err)
	}
}

func TestGitHubProvider_RequestReviewers_NoReviewers(t *testing.T) {
	provider := newTestGitHubProvider(map[string]*http.Response{})
	ctx := context.Background()

	err := provider.RequestReviewers(ctx, "owner/repo", 1, []string{}, []string{})
	if err != nil {
		t.Fatalf("RequestReviewers with no reviewers should not fail, got: %v", err)
	}
}

func TestGitHubProvider_RequestReviewers_InvalidRepo(t *testing.T) {
	provider := newTestGitHubProvider(map[string]*http.Response{})
	ctx := context.Background()

	err := provider.RequestReviewers(ctx, "invalid-repo", 1, []string{"reviewer1"}, []string{})
	if err == nil {
		t.Fatal("expected error for invalid repo format, got nil")
	}

	if !strings.Contains(err.Error(), "invalid repository format") {
		t.Errorf("expected error about invalid repository format, got: %v", err)
	}
}

func TestParseRepoString(t *testing.T) {
	tests := []struct {
		input       string
		wantOwner   string
		wantName    string
		wantErr     bool
		description string
	}{
		{
			input:       "owner/repo",
			wantOwner:   "owner",
			wantName:    "repo",
			wantErr:     false,
			description: "valid repo format",
		},
		{
			input:       "invalid-format",
			wantErr:     true,
			description: "missing slash",
		},
		{
			input:       "/repo",
			wantErr:     true,
			description: "empty owner",
		},
		{
			input:       "owner/",
			wantErr:     true,
			description: "empty repo name",
		},
		{
			input:       "owner/repo/extra",
			wantErr:     true,
			description: "too many parts",
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			owner, name, err := ParseRepoString(tt.input)

			if tt.wantErr && err == nil {
				t.Errorf("expected error for input %q, got nil", tt.input)
			}

			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error for input %q: %v", tt.input, err)
			}

			if !tt.wantErr {
				if owner != tt.wantOwner {
					t.Errorf("expected owner %q, got %q", tt.wantOwner, owner)
				}
				if name != tt.wantName {
					t.Errorf("expected name %q, got %q", tt.wantName, name)
				}
			}
		})
	}
}

func TestIsRateLimitError(t *testing.T) {
	rateLimitErr := &github.RateLimitError{}
	if !IsRateLimitError(rateLimitErr) {
		t.Error("expected true for RateLimitError, got false")
	}

	otherErr := fmt.Errorf("some other error")
	if IsRateLimitError(otherErr) {
		t.Error("expected false for other error, got true")
	}
}

func TestIsAbuseRateLimitError(t *testing.T) {
	abuseErr := &github.AbuseRateLimitError{}
	if !IsAbuseRateLimitError(abuseErr) {
		t.Error("expected true for AbuseRateLimitError, got false")
	}

	otherErr := fmt.Errorf("some other error")
	if IsAbuseRateLimitError(otherErr) {
		t.Error("expected false for other error, got true")
	}
}

func TestExtractRateLimitInfo(t *testing.T) {
	resp := &github.Response{
		Rate: github.Rate{
			Limit:     5000,
			Remaining: 4999,
		},
	}

	rate := ExtractRateLimitInfo(resp)
	if rate == nil {
		t.Fatal("expected rate limit info, got nil")
	}

	if rate.Limit != 5000 {
		t.Errorf("expected limit 5000, got %d", rate.Limit)
	}

	if rate.Remaining != 4999 {
		t.Errorf("expected remaining 4999, got %d", rate.Remaining)
	}

	nilResp := ExtractRateLimitInfo(nil)
	if nilResp != nil {
		t.Error("expected nil for nil response, got non-nil")
	}
}
