package broker

import "fmt"

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
	Operation string
	Repo      string
	Err       error
}

func (e *GitHubAPIError) Error() string {
	return fmt.Sprintf("broker: GitHub API operation %s failed for repo %s: %v", e.Operation, e.Repo, e.Err)
}

func (e *GitHubAPIError) Unwrap() error {
	return e.Err
}

// TemplateRenderError wraps template rendering failures.
type TemplateRenderError struct {
	TemplateName string
	Operation    string
	Err          error
}

func (e *TemplateRenderError) Error() string {
	return fmt.Sprintf("broker: template render %s failed for template %s: %v", e.Operation, e.TemplateName, e.Err)
}

func (e *TemplateRenderError) Unwrap() error {
	return e.Err
}
