package broker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/goliatone/cascade/internal/executor"
	"github.com/goliatone/cascade/internal/planner"
	"github.com/google/go-github/v66/github"
)

// Notifier defines behaviour for sending notifications.
type Notifier interface {
	Send(ctx context.Context, item planner.WorkItem, result *executor.Result) (*NotificationResult, error)
}

// NotificationConfig holds configuration for notifications.
type NotificationConfig struct {
	// Template for notification messages
	Template string

	// Retry configuration
	MaxRetries int
	RetryDelay time.Duration

	// HTTP client timeout
	Timeout time.Duration
}

// DefaultNotificationConfig returns sensible defaults.
func DefaultNotificationConfig() NotificationConfig {
	return NotificationConfig{
		Template:   defaultNotificationTemplate,
		MaxRetries: 3,
		RetryDelay: time.Second * 2,
		Timeout:    time.Second * 30,
	}
}

// HTTPClient interface for HTTP requests (for testing and dependency injection).
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// SlackNotifier sends notifications to Slack channels.
type SlackNotifier struct {
	botToken string
	channel  string
	client   HTTPClient
	config   NotificationConfig
}

// NewSlackNotifier creates a new Slack notifier.
func NewSlackNotifier(botToken, channel string, client HTTPClient, config NotificationConfig) *SlackNotifier {
	if client == nil {
		client = &http.Client{Timeout: config.Timeout}
	}
	return &SlackNotifier{
		botToken: botToken,
		channel:  channel,
		client:   client,
		config:   config,
	}
}

// Send sends a notification to Slack.
func (s *SlackNotifier) Send(ctx context.Context, item planner.WorkItem, result *executor.Result) (*NotificationResult, error) {
	message, err := RenderNotification(s.config.Template, item, result)
	if err != nil {
		return nil, &NotificationError{
			Channel: s.channel,
			Err:     fmt.Errorf("render notification template: %w", err),
		}
	}

	payload := map[string]any{
		"channel": s.channel,
		"text":    message,
		"as_user": true,
		"mrkdwn":  true,
	}

	return s.sendWithRetry(ctx, payload)
}

// sendWithRetry sends the message with retry logic.
func (s *SlackNotifier) sendWithRetry(ctx context.Context, payload map[string]any) (*NotificationResult, error) {
	var lastErr error

	for attempt := 0; attempt <= s.config.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, &NotificationError{
					Channel: s.channel,
					Err:     fmt.Errorf("context cancelled after %d attempts: %w", attempt, ctx.Err()),
				}
			case <-time.After(s.config.RetryDelay * time.Duration(attempt)):
				// Exponential backoff
			}
		}

		result, err := s.sendSlackMessage(ctx, payload)
		if err == nil {
			return result, nil
		}

		lastErr = err

		// Don't retry on non-transient errors
		if !isTransientError(err) {
			break
		}
	}

	return nil, &NotificationError{
		Channel: s.channel,
		Err:     fmt.Errorf("failed after %d attempts: %w", s.config.MaxRetries+1, lastErr),
	}
}

// sendSlackMessage sends a single message to Slack API.
func (s *SlackNotifier) sendSlackMessage(ctx context.Context, payload map[string]any) (*NotificationResult, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://slack.com/api/chat.postMessage", bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.botToken)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("slack API error: status %d", resp.StatusCode)
	}

	return &NotificationResult{
		Channel: s.channel,
		Message: payload["text"].(string),
	}, nil
}

// WebhookNotifier sends notifications to generic webhook endpoints.
type WebhookNotifier struct {
	url    string
	client HTTPClient
	config NotificationConfig
}

// NewWebhookNotifier creates a new webhook notifier.
func NewWebhookNotifier(url string, client HTTPClient, config NotificationConfig) *WebhookNotifier {
	if client == nil {
		client = &http.Client{Timeout: config.Timeout}
	}
	return &WebhookNotifier{
		url:    url,
		client: client,
		config: config,
	}
}

// Send sends a notification to the webhook endpoint.
func (w *WebhookNotifier) Send(ctx context.Context, item planner.WorkItem, result *executor.Result) (*NotificationResult, error) {
	message, err := RenderNotification(w.config.Template, item, result)
	if err != nil {
		return nil, &NotificationError{
			Channel: w.url,
			Err:     fmt.Errorf("render notification template: %w", err),
		}
	}

	status := ""
	if result != nil {
		status = string(result.Status)
	}

	payload := map[string]any{
		"text":   message,
		"module": item.Module,
		"repo":   item.Repo,
		"status": status,
	}

	return w.sendWithRetry(ctx, payload)
}

// sendWithRetry sends the webhook with retry logic.
func (w *WebhookNotifier) sendWithRetry(ctx context.Context, payload map[string]any) (*NotificationResult, error) {
	var lastErr error

	for attempt := 0; attempt <= w.config.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, &NotificationError{
					Channel: w.url,
					Err:     fmt.Errorf("context cancelled after %d attempts: %w", attempt, ctx.Err()),
				}
			case <-time.After(w.config.RetryDelay * time.Duration(attempt)):
				// Exponential backoff
			}
		}

		result, err := w.sendWebhook(ctx, payload)
		if err == nil {
			return result, nil
		}

		lastErr = err

		// Don't retry on non-transient errors
		if !isTransientError(err) {
			break
		}
	}

	return nil, &NotificationError{
		Channel: w.url,
		Err:     fmt.Errorf("failed after %d attempts: %w", w.config.MaxRetries+1, lastErr),
	}
}

// sendWebhook sends a single webhook request.
func (w *WebhookNotifier) sendWebhook(ctx context.Context, payload map[string]any) (*NotificationResult, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", w.url, bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := w.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("webhook error: status %d", resp.StatusCode)
	}

	return &NotificationResult{
		Channel: w.url,
		Message: payload["text"].(string),
	}, nil
}

// GitHubIssuesService defines the subset of the GitHub Issues API that the notifier requires.
type GitHubIssuesService interface {
	Create(ctx context.Context, owner, repo string, issue *github.IssueRequest) (*github.Issue, *github.Response, error)
	ListByRepo(ctx context.Context, owner, repo string, opts *github.IssueListByRepoOptions) ([]*github.Issue, *github.Response, error)
}

// GitHubIssueConfig captures default configuration for GitHub issue notifications.
type GitHubIssueConfig struct {
	Enabled bool
	Labels  []string
}

// GitHubIssueNotifier creates or reuses GitHub issues for failure notifications.
type GitHubIssueNotifier struct {
	issues        GitHubIssuesService
	defaults      *GitHubIssueConfig
	titleTemplate string
	bodyTemplate  string
}

// NewGitHubIssueNotifier constructs a notifier backed by the GitHub Issues API.
func NewGitHubIssueNotifier(issues GitHubIssuesService, defaults *GitHubIssueConfig) *GitHubIssueNotifier {
	return &GitHubIssueNotifier{
		issues:        issues,
		defaults:      cloneGitHubIssueConfig(defaults),
		titleTemplate: defaultGitHubIssueTitleTemplate,
		bodyTemplate:  defaultGitHubIssueBodyTemplate,
	}
}

func cloneGitHubIssueConfig(cfg *GitHubIssueConfig) *GitHubIssueConfig {
	if cfg == nil {
		return nil
	}
	clone := &GitHubIssueConfig{Enabled: cfg.Enabled}
	if len(cfg.Labels) > 0 {
		clone.Labels = append([]string(nil), cfg.Labels...)
	}
	return clone
}

// Send creates a GitHub issue for failed work items when enabled by configuration.
func (g *GitHubIssueNotifier) Send(ctx context.Context, item planner.WorkItem, result *executor.Result) (*NotificationResult, error) {
	if g.issues == nil {
		return nil, &NotificationError{
			Channel: "github-issues",
			Err:     fmt.Errorf("github issues client not configured"),
		}
	}

	channel := fmt.Sprintf("github:%s", item.Repo)

	if result == nil {
		return &NotificationResult{
			Channel: channel,
			Message: "no execution result provided",
		}, nil
	}

	config := g.effectiveConfig(item)
	if !config.Enabled {
		return &NotificationResult{
			Channel: channel,
			Message: "github issue notifications disabled",
		}, nil
	}

	if result.Status != executor.StatusFailed {
		return &NotificationResult{
			Channel: channel,
			Message: "no failure detected",
		}, nil
	}

	owner, repo, err := ParseRepoString(item.Repo)
	if err != nil {
		return nil, &NotificationError{
			Channel: channel,
			Err:     fmt.Errorf("invalid repository: %w", err),
		}
	}

	title, err := RenderGitHubIssueTitle(g.titleTemplate, item, result)
	if err != nil {
		return nil, &NotificationError{
			Channel: channel,
			Err:     fmt.Errorf("render issue title: %w", err),
		}
	}

	body, err := RenderGitHubIssueBody(g.bodyTemplate, item, result)
	if err != nil {
		return nil, &NotificationError{
			Channel: channel,
			Err:     fmt.Errorf("render issue body: %w", err),
		}
	}

	labels := config.Labels
	if len(labels) == 0 {
		labels = []string{"cascade-failure"}
	}

	existing, err := g.findExistingIssue(ctx, owner, repo, title, labels)
	if err != nil {
		return nil, &NotificationError{
			Channel: channel,
			Err:     fmt.Errorf("list existing issues: %w", err),
		}
	}

	if existing != nil {
		url := existing.GetHTMLURL()
		if url == "" {
			url = fmt.Sprintf("https://github.com/%s/issues/%d", item.Repo, existing.GetNumber())
		}
		return &NotificationResult{
			Channel: channel,
			Message: url,
		}, nil
	}

	labelsCopy := append([]string(nil), labels...)
	request := &github.IssueRequest{
		Title: &title,
		Body:  &body,
	}
	if len(labelsCopy) > 0 {
		request.Labels = &labelsCopy
	}

	issue, resp, err := g.issues.Create(ctx, owner, repo, request)
	if err != nil {
		status := 0
		if resp != nil && resp.Response != nil {
			status = resp.Response.StatusCode
		}
		return nil, &NotificationError{
			Channel: channel,
			Err: &GitHubAPIError{
				Operation:    "create issue",
				Repo:         item.Repo,
				StatusCode:   status,
				ResponseBody: extractGitHubResponseBody(resp),
				Err:          err,
			},
		}
	}

	issueURL := ""
	if issue != nil {
		issueURL = issue.GetHTMLURL()
		if issueURL == "" {
			issueURL = fmt.Sprintf("https://github.com/%s/issues/%d", item.Repo, issue.GetNumber())
		}
	}

	return &NotificationResult{
		Channel: channel,
		Message: issueURL,
	}, nil
}

func (g *GitHubIssueNotifier) effectiveConfig(item planner.WorkItem) GitHubIssueConfig {
	config := GitHubIssueConfig{}
	if g.defaults != nil {
		config.Enabled = g.defaults.Enabled
		if len(g.defaults.Labels) > 0 {
			config.Labels = append([]string(nil), g.defaults.Labels...)
		}
	}

	if item.Notifications.GitHubIssues != nil {
		config.Enabled = item.Notifications.GitHubIssues.Enabled
		if len(item.Notifications.GitHubIssues.Labels) > 0 {
			config.Labels = append([]string(nil), item.Notifications.GitHubIssues.Labels...)
		}
	}

	return config
}

func (g *GitHubIssueNotifier) findExistingIssue(ctx context.Context, owner, repo, title string, labels []string) (*github.Issue, error) {
	opts := &github.IssueListByRepoOptions{
		State:       "open",
		ListOptions: github.ListOptions{PerPage: 50},
	}
	if len(labels) > 0 {
		opts.Labels = labels
	}

	checkedPages := 0
	for {
		issues, resp, err := g.issues.ListByRepo(ctx, owner, repo, opts)
		if err != nil {
			return nil, err
		}
		for _, issue := range issues {
			if strings.EqualFold(issue.GetTitle(), title) {
				return issue, nil
			}
		}

		checkedPages++
		if resp == nil || resp.NextPage == 0 || checkedPages >= 3 {
			break
		}
		opts.Page = resp.NextPage
	}

	return nil, nil
}

func extractGitHubResponseBody(resp *github.Response) string {
	if resp == nil || resp.Response == nil || resp.Response.Body == nil {
		return ""
	}
	defer resp.Response.Body.Close()
	body, err := io.ReadAll(resp.Response.Body)
	if err != nil {
		return ""
	}
	return string(body)
}

// MultiNotifier sends notifications to multiple notifiers.
type MultiNotifier struct {
	notifiers []Notifier
}

// NewMultiNotifier creates a notifier that sends to multiple destinations.
func NewMultiNotifier(notifiers ...Notifier) *MultiNotifier {
	return &MultiNotifier{notifiers: notifiers}
}

// Send sends notifications to all configured notifiers.
// Failures from individual notifiers don't prevent others from sending.
func (m *MultiNotifier) Send(ctx context.Context, item planner.WorkItem, result *executor.Result) (*NotificationResult, error) {
	var errors []string
	var firstResult *NotificationResult

	for _, notifier := range m.notifiers {
		notifyResult, err := notifier.Send(ctx, item, result)
		if err != nil {
			errors = append(errors, err.Error())
			continue
		}

		if firstResult == nil {
			firstResult = notifyResult
		}
	}

	if len(errors) == len(m.notifiers) {
		// All notifiers failed
		return nil, &NotificationError{
			Channel: "multi",
			Err:     fmt.Errorf("all notifiers failed: %s", strings.Join(errors, "; ")),
		}
	}

	// Return partial success (some notifiers succeeded)
	return firstResult, nil
}

// NoOpNotifier is a notifier that records notification intent but doesn't
// send actual notifications. Used when notification integrations are not configured.
type NoOpNotifier struct{}

// NewNoOpNotifier creates a new no-op notifier.
func NewNoOpNotifier() *NoOpNotifier {
	return &NoOpNotifier{}
}

// Send records the notification intent but doesn't send actual notifications.
// This avoids NotImplementedError when notifications are intentionally disabled.
func (n *NoOpNotifier) Send(ctx context.Context, item planner.WorkItem, result *executor.Result) (*NotificationResult, error) {
	// Return a successful result indicating the notification was "sent" (but actually skipped)
	return &NotificationResult{
		Channel: "noop",
		Message: "Notification skipped (no integrations configured)",
	}, nil
}

// isTransientError determines if an error is worth retrying.
func isTransientError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	return strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "connection") ||
		strings.Contains(errStr, "temporary") ||
		strings.Contains(errStr, "status 5") // 5xx HTTP errors
}

// Default notification template
const defaultNotificationTemplate = `{{if eq .Status "completed"}}✅{{else if eq .Status "failed"}}❌{{else}}⚠️{{end}} *{{.Module}}* update *{{.Status}}*

*Repository:* {{.Repo}}
{{if .BranchName}}*Branch:* {{.BranchName}}{{end}}
{{if .CommitHash}}*Commit:* {{.CommitHash | truncate8}}{{end}}

{{if .Reason}}*Details:* {{.Reason | truncate200 | escape}}{{end}}
{{if .FailureSummary}}*Failing Test:* {{.FailureSummary | escape}}{{end}}
{{if .FailureMessage}}*Failure:* {{.FailureMessage | truncate200 | escape}}{{end}}
{{if .FailureCommand}}*Command:* {{.FailureCommand | escape}}{{end}}
{{if .DependencySummary}}*Dependency:* {{.DependencySummary | escape}}{{if .DependencyNote}} — {{.DependencyNote | truncate200 | escape}}{{end}}{{end}}

Generated at {{.Timestamp.Format "15:04:05 MST"}}`

const defaultGitHubIssueTitleTemplate = `Cascade failure: update {{.SourceModule}}{{if .SourceVersion}} to {{.SourceVersion}}{{end}} in {{.Repo}}`

const defaultGitHubIssueBodyTemplate = `Cascade failed to update *{{.SourceModule}}*{{if .SourceVersion}} to *{{.SourceVersion}}*{{end}} for repository *{{.Repo}}*.

- **Status:** {{.Status}}
{{if .BranchName}}- **Branch:** {{.BranchName}}{{end}}
{{if .CommitHash}}- **Commit:** {{.CommitHash | truncate8}}{{end}}
{{if .ModulePath}}- **Module Path:** {{.ModulePath}}{{end}}

{{if .Reason}}## Failure Reason
{{.Reason | escape}}
{{end}}
{{if .FailureSummary}}## Test Failure
{{.FailureSummary | escape}}
{{end}}
{{if .FailureMessage}}## Failure Details
{{.FailureMessage | escape}}
{{end}}
{{if .FailureCommand}}## Command
` + "```" + `
{{.FailureCommand}}
` + "```" + `
{{end}}
{{if .DependencySummary}}## Dependency Impact
{{.DependencySummary}}
{{end}}

_Reported by Cascade at {{.Timestamp.Format "2006-01-02 15:04:05 MST"}}._`

// RenderNotification renders a notification message from a template.
func RenderNotification(tmpl string, item planner.WorkItem, result *executor.Result) (string, error) {
	if tmpl == "" {
		tmpl = defaultNotificationTemplate
	}

	data := buildTemplateData(item, result)
	return renderTemplate("notification", tmpl, data)
}

// RenderGitHubIssueTitle renders a GitHub issue title using the provided or default template.
func RenderGitHubIssueTitle(tmpl string, item planner.WorkItem, result *executor.Result) (string, error) {
	if tmpl == "" {
		tmpl = defaultGitHubIssueTitleTemplate
	}

	data := buildTemplateData(item, result)
	return renderTemplate("github_issue_title", tmpl, data)
}

// RenderGitHubIssueBody renders a GitHub issue body using the provided or default template.
func RenderGitHubIssueBody(tmpl string, item planner.WorkItem, result *executor.Result) (string, error) {
	if tmpl == "" {
		tmpl = defaultGitHubIssueBodyTemplate
	}

	data := buildTemplateData(item, result)
	return renderTemplate("github_issue_body", tmpl, data)
}
