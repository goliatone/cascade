package broker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/goliatone/cascade/internal/executor"
	"github.com/goliatone/cascade/internal/planner"
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
const defaultNotificationTemplate = `{{if eq .Status "completed"}}✅{{else if eq .Status "failed"}}❌{{else}}⚠️{{end}} **{{.Module}}** update {{.Status}}

**Repository**: {{.Repo}}
{{if .BranchName}}**Branch**: {{.BranchName}}{{end}}
{{if .Status}}**Status**: {{.Status}}{{end}}
{{if .CommitHash}}**Commit**: {{.CommitHash | truncate8}}{{end}}

{{if .Reason}}**Details**: {{.Reason | truncate200}}{{end}}

Generated at {{.Timestamp.Format "15:04:05 MST"}}`

// RenderNotification renders a notification message from a template.
func RenderNotification(tmpl string, item planner.WorkItem, result *executor.Result) (string, error) {
	if tmpl == "" {
		tmpl = defaultNotificationTemplate
	}

	data := buildTemplateData(item, result)
	return renderTemplate("notification", tmpl, data)
}
