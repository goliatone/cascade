package broker

import (
	"errors"
	"fmt"
	"net/http"
)

// NotImplementedError signals stubbed behaviour.
type NotImplementedError struct {
	Operation string
}

func (e *NotImplementedError) Error() string {
	return "not implemented: " + e.Operation
}

// ProviderError wraps errors from the external provider.
type ProviderError struct {
	Operation string
	Err       error
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("broker: provider operation %s failed: %v", e.Operation, e.Err)
}

func (e *ProviderError) Unwrap() error {
	return e.Err
}

// NotificationError wraps notification delivery failures.
type NotificationError struct {
	Channel string
	Err     error
}

func (e *NotificationError) Error() string {
	return fmt.Sprintf("broker: notification to %s failed: %v", e.Channel, e.Err)
}

func (e *NotificationError) Unwrap() error {
	return e.Err
}

// GitHubAPIError wraps GitHub API operation failures.
type GitHubAPIError struct {
	Operation    string
	Repo         string
	StatusCode   int
	ResponseBody string
	Err          error
}

func (e *GitHubAPIError) Error() string {
	if e.StatusCode != 0 {
		return fmt.Sprintf("broker: GitHub API operation %s failed for repo %s (status %d): %v", e.Operation, e.Repo, e.StatusCode, e.Err)
	}
	return fmt.Sprintf("broker: GitHub API operation %s failed for repo %s: %v", e.Operation, e.Repo, e.Err)
}

func (e *GitHubAPIError) Unwrap() error {
	return e.Err
}

// IsRateLimited returns true if the error is due to GitHub API rate limiting.
func (e *GitHubAPIError) IsRateLimited() bool {
	return e.StatusCode == http.StatusForbidden && e.ResponseBody != ""
}

// TemplateRenderError wraps template rendering failures.
type TemplateRenderError struct {
	TemplateName string
	Operation    string
	Placeholders []string
	Err          error
}

func (e *TemplateRenderError) Error() string {
	if len(e.Placeholders) > 0 {
		return fmt.Sprintf("broker: template render %s failed for template %s with placeholders %v: %v", e.Operation, e.TemplateName, e.Placeholders, e.Err)
	}
	return fmt.Sprintf("broker: template render %s failed for template %s: %v", e.Operation, e.TemplateName, e.Err)
}

func (e *TemplateRenderError) Unwrap() error {
	return e.Err
}

// Error detection helpers using errors.As for type checking

// IsGitHubAPIError returns true if the error is a GitHubAPIError.
func IsGitHubAPIError(err error) bool {
	var gitHubErr *GitHubAPIError
	return errors.As(err, &gitHubErr)
}

// IsTemplateRenderError returns true if the error is a TemplateRenderError.
func IsTemplateRenderError(err error) bool {
	var templateErr *TemplateRenderError
	return errors.As(err, &templateErr)
}

// IsNotificationError returns true if the error is a NotificationError.
func IsNotificationError(err error) bool {
	var notificationErr *NotificationError
	return errors.As(err, &notificationErr)
}

// IsPRValidationError returns true if the error is a PRValidationError.
func IsPRValidationError(err error) bool {
	var validationErr *PRValidationError
	return errors.As(err, &validationErr)
}

// AsGitHubAPIError extracts a GitHubAPIError if the error chain contains one.
func AsGitHubAPIError(err error) (*GitHubAPIError, bool) {
	var gitHubErr *GitHubAPIError
	if errors.As(err, &gitHubErr) {
		return gitHubErr, true
	}
	return nil, false
}

// AsTemplateRenderError extracts a TemplateRenderError if the error chain contains one.
func AsTemplateRenderError(err error) (*TemplateRenderError, bool) {
	var templateErr *TemplateRenderError
	if errors.As(err, &templateErr) {
		return templateErr, true
	}
	return nil, false
}

// AsNotificationError extracts a NotificationError if the error chain contains one.
func AsNotificationError(err error) (*NotificationError, bool) {
	var notificationErr *NotificationError
	if errors.As(err, &notificationErr) {
		return notificationErr, true
	}
	return nil, false
}

// AsPRValidationError extracts a PRValidationError if the error chain contains one.
func AsPRValidationError(err error) (*PRValidationError, bool) {
	var validationErr *PRValidationError
	if errors.As(err, &validationErr) {
		return validationErr, true
	}
	return nil, false
}
