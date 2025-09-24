package broker

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/google/go-github/v66/github"
	"golang.org/x/oauth2"
)

// AuthConfig holds authentication configuration options.
type AuthConfig struct {
	// Token is the GitHub personal access token or OAuth token
	Token string
	// BaseURL is the GitHub API base URL (for GitHub Enterprise)
	BaseURL string
	// UploadURL is the GitHub upload URL (for GitHub Enterprise)
	UploadURL string
	// InsecureSkipVerify skips TLS verification (for self-signed certificates)
	InsecureSkipVerify bool
}

// LoadGitHubToken loads a GitHub token from environment variables or configuration.
// It checks multiple environment variables in order of precedence:
// 1. GITHUB_TOKEN
// 2. GITHUB_ACCESS_TOKEN
// 3. GH_TOKEN
func LoadGitHubToken() (string, error) {
	// Check environment variables in order of precedence
	envVars := []string{"GITHUB_TOKEN", "GITHUB_ACCESS_TOKEN", "GH_TOKEN"}

	for _, envVar := range envVars {
		if token := os.Getenv(envVar); token != "" {
			return strings.TrimSpace(token), nil
		}
	}

	return "", fmt.Errorf("GitHub token not found: set one of %v environment variables", envVars)
}

// CreateAuthenticatedClient creates a GitHub client with the given token and configuration.
func CreateAuthenticatedClient(config AuthConfig) (*github.Client, error) {
	if config.Token == "" {
		return nil, fmt.Errorf("GitHub token is required")
	}

	// Create OAuth2 token source
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: config.Token},
	)

	// Create HTTP client with OAuth2 transport
	httpClient := oauth2.NewClient(context.Background(), ts)

	// Configure TLS settings if needed
	if config.InsecureSkipVerify {
		transport := httpClient.Transport.(*oauth2.Transport)
		if transport.Base == nil {
			transport.Base = http.DefaultTransport
		}

		if baseTransport, ok := transport.Base.(*http.Transport); ok {
			baseTransport.TLSClientConfig = &tls.Config{
				InsecureSkipVerify: true,
			}
		}
	}

	var client *github.Client

	// Create GitHub client with custom base URL if specified (GitHub Enterprise)
	if config.BaseURL != "" {
		var err error
		client, err = github.NewClient(httpClient).WithEnterpriseURLs(config.BaseURL, config.UploadURL)
		if err != nil {
			return nil, fmt.Errorf("failed to create GitHub Enterprise client: %w", err)
		}
	} else {
		client = github.NewClient(httpClient)
	}

	return client, nil
}

// ValidateAuthentication verifies that the GitHub client can authenticate successfully.
func ValidateAuthentication(ctx context.Context, client *github.Client) error {
	if client == nil {
		return fmt.Errorf("GitHub client is nil")
	}

	// Test authentication by getting the authenticated user
	user, resp, err := client.Users.Get(ctx, "")
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusUnauthorized {
			return fmt.Errorf("GitHub authentication failed: invalid or expired token")
		}
		return fmt.Errorf("GitHub authentication validation failed: %w", err)
	}

	if user == nil || user.Login == nil {
		return fmt.Errorf("GitHub authentication succeeded but user information is unavailable")
	}

	return nil
}

// GetRateLimit retrieves current GitHub API rate limit information.
func GetRateLimit(ctx context.Context, client *github.Client) (*github.RateLimits, error) {
	if client == nil {
		return nil, fmt.Errorf("GitHub client is nil")
	}

	rateLimits, _, err := client.RateLimits(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get GitHub API rate limits: %w", err)
	}

	return rateLimits, nil
}

// IsRateLimitCritical checks if the rate limit is critically low (< 10% remaining).
func IsRateLimitCritical(rate *github.Rate) bool {
	if rate == nil {
		return false
	}

	threshold := float64(rate.Limit) * 0.10 // 10% threshold
	return float64(rate.Remaining) < threshold
}

// CheckRateLimit checks the current rate limit and returns a warning if it's low.
func CheckRateLimit(ctx context.Context, client *github.Client) error {
	rateLimits, err := GetRateLimit(ctx, client)
	if err != nil {
		return err
	}

	if rateLimits.Core != nil && IsRateLimitCritical(rateLimits.Core) {
		return fmt.Errorf("GitHub API rate limit critically low: %d/%d remaining (resets at %v)",
			rateLimits.Core.Remaining,
			rateLimits.Core.Limit,
			rateLimits.Core.Reset.Time)
	}

	return nil
}

// AuthenticationError represents authentication-related errors.
type AuthenticationError struct {
	Operation string
	Err       error
}

func (e *AuthenticationError) Error() string {
	return fmt.Sprintf("broker: authentication error during %s: %v", e.Operation, e.Err)
}

func (e *AuthenticationError) Unwrap() error {
	return e.Err
}

// IsAuthenticationError checks if an error is an AuthenticationError.
func IsAuthenticationError(err error) bool {
	var authErr *AuthenticationError
	return errors.As(err, &authErr)
}

// AsAuthenticationError extracts an AuthenticationError if the error chain contains one.
func AsAuthenticationError(err error) (*AuthenticationError, bool) {
	var authErr *AuthenticationError
	if errors.As(err, &authErr) {
		return authErr, true
	}
	return nil, false
}
