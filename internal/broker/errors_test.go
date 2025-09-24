package broker

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
)

func TestGitHubAPIError(t *testing.T) {
	tests := []struct {
		name        string
		err         *GitHubAPIError
		wantMessage string
		rateLimited bool
	}{
		{
			name: "basic error without status code",
			err: &GitHubAPIError{
				Operation: "create_pr",
				Repo:      "owner/repo",
				Err:       errors.New("network error"),
			},
			wantMessage: "broker: GitHub API operation create_pr failed for repo owner/repo: network error",
			rateLimited: false,
		},
		{
			name: "error with status code",
			err: &GitHubAPIError{
				Operation:  "update_pr",
				Repo:       "owner/repo",
				StatusCode: http.StatusNotFound,
				Err:        errors.New("not found"),
			},
			wantMessage: "broker: GitHub API operation update_pr failed for repo owner/repo (status 404): not found",
			rateLimited: false,
		},
		{
			name: "rate limited error",
			err: &GitHubAPIError{
				Operation:    "list_prs",
				Repo:         "owner/repo",
				StatusCode:   http.StatusForbidden,
				ResponseBody: `{"message": "API rate limit exceeded"}`,
				Err:          errors.New("rate limit exceeded"),
			},
			wantMessage: "broker: GitHub API operation list_prs failed for repo owner/repo (status 403): rate limit exceeded",
			rateLimited: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.wantMessage {
				t.Errorf("GitHubAPIError.Error() = %v, want %v", got, tt.wantMessage)
			}

			if got := tt.err.IsRateLimited(); got != tt.rateLimited {
				t.Errorf("GitHubAPIError.IsRateLimited() = %v, want %v", got, tt.rateLimited)
			}

			// Test unwrapping
			if underlying := tt.err.Unwrap(); underlying != tt.err.Err {
				t.Errorf("GitHubAPIError.Unwrap() = %v, want %v", underlying, tt.err.Err)
			}
		})
	}
}

func TestTemplateRenderError(t *testing.T) {
	tests := []struct {
		name        string
		err         *TemplateRenderError
		wantMessage string
	}{
		{
			name: "basic template error",
			err: &TemplateRenderError{
				TemplateName: "pr_title",
				Operation:    "render",
				Err:          errors.New("template parse error"),
			},
			wantMessage: "broker: template render render failed for template pr_title: template parse error",
		},
		{
			name: "template error with placeholders",
			err: &TemplateRenderError{
				TemplateName: "pr_body",
				Operation:    "execute",
				Placeholders: []string{"{{.Module}}", "{{.Version}}"},
				Err:          errors.New("missing variable"),
			},
			wantMessage: "broker: template render execute failed for template pr_body with placeholders [{{.Module}} {{.Version}}]: missing variable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.wantMessage {
				t.Errorf("TemplateRenderError.Error() = %v, want %v", got, tt.wantMessage)
			}

			// Test unwrapping
			if underlying := tt.err.Unwrap(); underlying != tt.err.Err {
				t.Errorf("TemplateRenderError.Unwrap() = %v, want %v", underlying, tt.err.Err)
			}
		})
	}
}

func TestNotificationError(t *testing.T) {
	err := &NotificationError{
		Channel: "slack://general",
		Err:     errors.New("webhook failed"),
	}

	wantMessage := "broker: notification to slack://general failed: webhook failed"
	if got := err.Error(); got != wantMessage {
		t.Errorf("NotificationError.Error() = %v, want %v", got, wantMessage)
	}

	// Test unwrapping
	if underlying := err.Unwrap(); underlying != err.Err {
		t.Errorf("NotificationError.Unwrap() = %v, want %v", underlying, err.Err)
	}
}


func TestErrorDetectionHelpers(t *testing.T) {
	gitHubErr := &GitHubAPIError{Operation: "test", Repo: "test/test", Err: errors.New("test")}
	templateErr := &TemplateRenderError{TemplateName: "test", Operation: "test", Err: errors.New("test")}
	notificationErr := &NotificationError{Channel: "test", Err: errors.New("test")}
	validationErr := &PRValidationError{Field: "test", Message: "test"}
	genericErr := errors.New("generic error")

	// Wrap errors to test error chain detection
	wrappedGitHubErr := fmt.Errorf("wrapped: %w", gitHubErr)
	wrappedTemplateErr := fmt.Errorf("wrapped: %w", templateErr)
	wrappedNotificationErr := fmt.Errorf("wrapped: %w", notificationErr)
	wrappedValidationErr := fmt.Errorf("wrapped: %w", validationErr)

	tests := []struct {
		name     string
		err      error
		testFunc func(error) bool
		want     bool
	}{
		// Direct error type checks
		{"GitHubAPIError direct", gitHubErr, IsGitHubAPIError, true},
		{"TemplateRenderError direct", templateErr, IsTemplateRenderError, true},
		{"NotificationError direct", notificationErr, IsNotificationError, true},
		{"PRValidationError direct", validationErr, IsPRValidationError, true},

		// Wrapped error type checks
		{"GitHubAPIError wrapped", wrappedGitHubErr, IsGitHubAPIError, true},
		{"TemplateRenderError wrapped", wrappedTemplateErr, IsTemplateRenderError, true},
		{"NotificationError wrapped", wrappedNotificationErr, IsNotificationError, true},
		{"PRValidationError wrapped", wrappedValidationErr, IsPRValidationError, true},

		// Cross-type checks (should be false)
		{"GitHubAPIError vs Template check", gitHubErr, IsTemplateRenderError, false},
		{"TemplateRenderError vs Notification check", templateErr, IsNotificationError, false},
		{"NotificationError vs Validation check", notificationErr, IsPRValidationError, false},
		{"PRValidationError vs GitHub check", validationErr, IsGitHubAPIError, false},

		// Generic error checks (should be false)
		{"Generic error vs GitHub check", genericErr, IsGitHubAPIError, false},
		{"Generic error vs Template check", genericErr, IsTemplateRenderError, false},
		{"Generic error vs Notification check", genericErr, IsNotificationError, false},
		{"Generic error vs Validation check", genericErr, IsPRValidationError, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.testFunc(tt.err); got != tt.want {
				t.Errorf("Error detection helper = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestErrorExtractionHelpers(t *testing.T) {
	gitHubErr := &GitHubAPIError{Operation: "test", Repo: "test/test", Err: errors.New("test")}
	templateErr := &TemplateRenderError{TemplateName: "test", Operation: "test", Err: errors.New("test")}
	notificationErr := &NotificationError{Channel: "test", Err: errors.New("test")}
	validationErr := &PRValidationError{Field: "test", Message: "test"}

	// Test AsGitHubAPIError
	t.Run("AsGitHubAPIError", func(t *testing.T) {
		extracted, ok := AsGitHubAPIError(gitHubErr)
		if !ok {
			t.Error("AsGitHubAPIError should return true for GitHubAPIError")
		}
		if extracted != gitHubErr {
			t.Error("AsGitHubAPIError should return the original error")
		}

		// Test with wrapped error
		wrapped := fmt.Errorf("wrapped: %w", gitHubErr)
		extracted, ok = AsGitHubAPIError(wrapped)
		if !ok {
			t.Error("AsGitHubAPIError should return true for wrapped GitHubAPIError")
		}
		if extracted != gitHubErr {
			t.Error("AsGitHubAPIError should return the original error from wrapped error")
		}

		// Test with wrong error type
		extracted, ok = AsGitHubAPIError(templateErr)
		if ok {
			t.Error("AsGitHubAPIError should return false for TemplateRenderError")
		}
		if extracted != nil {
			t.Error("AsGitHubAPIError should return nil for wrong error type")
		}
	})

	t.Run("AsTemplateRenderError", func(t *testing.T) {
		extracted, ok := AsTemplateRenderError(templateErr)
		if !ok {
			t.Error("AsTemplateRenderError should return true for TemplateRenderError")
		}
		if extracted != templateErr {
			t.Error("AsTemplateRenderError should return the original error")
		}
	})

	t.Run("AsNotificationError", func(t *testing.T) {
		extracted, ok := AsNotificationError(notificationErr)
		if !ok {
			t.Error("AsNotificationError should return true for NotificationError")
		}
		if extracted != notificationErr {
			t.Error("AsNotificationError should return the original error")
		}
	})

	t.Run("AsPRValidationError", func(t *testing.T) {
		extracted, ok := AsPRValidationError(validationErr)
		if !ok {
			t.Error("AsPRValidationError should return true for PRValidationError")
		}
		if extracted != validationErr {
			t.Error("AsPRValidationError should return the original error")
		}
	})
}

func TestNotImplementedError(t *testing.T) {
	err := &NotImplementedError{Operation: "test_operation"}
	wantMessage := "not implemented: test_operation"
	if got := err.Error(); got != wantMessage {
		t.Errorf("NotImplementedError.Error() = %v, want %v", got, wantMessage)
	}
}

func TestProviderError(t *testing.T) {
	underlyingErr := errors.New("underlying error")
	err := &ProviderError{
		Operation: "test_operation",
		Err:       underlyingErr,
	}
	wantMessage := "broker: provider operation test_operation failed: underlying error"
	if got := err.Error(); got != wantMessage {
		t.Errorf("ProviderError.Error() = %v, want %v", got, wantMessage)
	}

	// Test unwrapping
	if underlying := err.Unwrap(); underlying != underlyingErr {
		t.Errorf("ProviderError.Unwrap() = %v, want %v", underlying, underlyingErr)
	}
}