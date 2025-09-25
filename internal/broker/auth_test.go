package broker

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v66/github"
)

func TestLoadGitHubToken(t *testing.T) {
	tests := []struct {
		name       string
		envVars    map[string]string
		wantToken  string
		wantErr    bool
		errMessage string
	}{
		{
			name:      "GITHUB_TOKEN set",
			envVars:   map[string]string{"GITHUB_TOKEN": "token123"},
			wantToken: "token123",
			wantErr:   false,
		},
		{
			name:      "GITHUB_ACCESS_TOKEN set",
			envVars:   map[string]string{"GITHUB_ACCESS_TOKEN": "access_token456"},
			wantToken: "access_token456",
			wantErr:   false,
		},
		{
			name:      "GH_TOKEN set",
			envVars:   map[string]string{"GH_TOKEN": "gh_token789"},
			wantToken: "gh_token789",
			wantErr:   false,
		},
		{
			name:      "token with whitespace",
			envVars:   map[string]string{"GITHUB_TOKEN": "  token123  "},
			wantToken: "token123",
			wantErr:   false,
		},
		{
			name:      "precedence: GITHUB_TOKEN over others",
			envVars:   map[string]string{"GITHUB_TOKEN": "primary", "GH_TOKEN": "secondary"},
			wantToken: "primary",
			wantErr:   false,
		},
		{
			name:       "no token set",
			envVars:    map[string]string{},
			wantToken:  "",
			wantErr:    true,
			errMessage: "GitHub token not found",
		},
		{
			name:       "empty token",
			envVars:    map[string]string{"GITHUB_TOKEN": ""},
			wantToken:  "",
			wantErr:    true,
			errMessage: "GitHub token not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vars := []string{"GITHUB_TOKEN", "GITHUB_ACCESS_TOKEN", "GH_TOKEN"}
			for _, env := range vars {
				os.Unsetenv(env)
			}

			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			defer func() {
				for key := range tt.envVars {
					os.Unsetenv(key)
				}
			}()

			got, err := LoadGitHubToken()

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tt.errMessage != "" && !strings.Contains(err.Error(), tt.errMessage) {
					t.Fatalf("error = %v, want substring %q", err, tt.errMessage)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tt.wantToken {
				t.Fatalf("LoadGitHubToken() = %q, want %q", got, tt.wantToken)
			}
		})
	}
}

func TestCreateAuthenticatedClient(t *testing.T) {
	tests := []struct {
		name    string
		config  AuthConfig
		wantErr bool
	}{
		{
			name: "valid token",
			config: AuthConfig{
				Token: "valid_token",
			},
			wantErr: false,
		},
		{
			name: "with base URL",
			config: AuthConfig{
				Token:   "valid_token",
				BaseURL: "https://github.enterprise.com/api/v3/",
			},
			wantErr: false,
		},
		{
			name: "with insecure skip verify",
			config: AuthConfig{
				Token:              "valid_token",
				InsecureSkipVerify: true,
			},
			wantErr: false,
		},
		{
			name: "empty token",
			config: AuthConfig{
				Token: "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := CreateAuthenticatedClient(tt.config)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if client != nil {
					t.Fatalf("expected nil client on error, got %v", client)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if client == nil {
				t.Fatalf("expected non-nil client")
			}
		})
	}
}

func TestValidateAuthentication(t *testing.T) {
	tests := []struct {
		name        string
		clientFunc  func(t *testing.T) *github.Client
		wantErr     bool
		errContains string
	}{
		{
			name: "successful authentication",
			clientFunc: func(t *testing.T) *github.Client {
				return newStubGitHubClient(t, func(req *http.Request) (*http.Response, error) {
					if req.URL.Path != "/user" {
						t.Fatalf("expected /user path, got %s", req.URL.Path)
					}
					return jsonResponse(req, http.StatusOK, `{"login":"testuser","id":12345}`), nil
				})
			},
			wantErr: false,
		},
		{
			name: "unauthorized",
			clientFunc: func(t *testing.T) *github.Client {
				return newStubGitHubClient(t, func(req *http.Request) (*http.Response, error) {
					return jsonResponse(req, http.StatusUnauthorized, `{"message":"Bad credentials"}`), nil
				})
			},
			wantErr:     true,
			errContains: "invalid or expired token",
		},
		{
			name: "server error",
			clientFunc: func(t *testing.T) *github.Client {
				return newStubGitHubClient(t, func(req *http.Request) (*http.Response, error) {
					return jsonResponse(req, http.StatusInternalServerError, "{}"), nil
				})
			},
			wantErr:     true,
			errContains: "authentication validation failed",
		},
		{
			name:        "nil client",
			clientFunc:  func(t *testing.T) *github.Client { return nil },
			wantErr:     true,
			errContains: "GitHub client is nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := tt.clientFunc(t)
			err := ValidateAuthentication(context.Background(), client)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("error = %v, want substring %q", err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestGetRateLimit(t *testing.T) {
	tests := []struct {
		name        string
		clientFunc  func(t *testing.T) *github.Client
		wantErr     bool
		errContains string
	}{
		{
			name: "successful rate limit fetch",
			clientFunc: func(t *testing.T) *github.Client {
				payload := `{"rate":{"limit":5000,"remaining":4999,"reset":1625097600,"used":1},"resources":{"core":{"limit":5000,"remaining":4999,"reset":1625097600,"used":1}}}`
				return newStubGitHubClient(t, func(req *http.Request) (*http.Response, error) {
					if req.URL.Path != "/rate_limit" {
						t.Fatalf("expected /rate_limit path, got %s", req.URL.Path)
					}
					return jsonResponse(req, http.StatusOK, payload), nil
				})
			},
			wantErr: false,
		},
		{
			name: "server error",
			clientFunc: func(t *testing.T) *github.Client {
				return newStubGitHubClient(t, func(req *http.Request) (*http.Response, error) {
					return jsonResponse(req, http.StatusInternalServerError, "{}"), nil
				})
			},
			wantErr:     true,
			errContains: "failed to get GitHub API rate limits",
		},
		{
			name:        "nil client",
			clientFunc:  func(t *testing.T) *github.Client { return nil },
			wantErr:     true,
			errContains: "GitHub client is nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := tt.clientFunc(t)
			limits, err := GetRateLimit(context.Background(), client)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("error = %v, want substring %q", err, tt.errContains)
				}
				if limits != nil {
					t.Fatalf("expected nil limits when err != nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if limits == nil {
				t.Fatalf("expected non-nil rate limits")
			}
		})
	}
}

func TestCheckRateLimit(t *testing.T) {
	resetTime := time.Now().Add(time.Hour).Unix()

	tests := []struct {
		name        string
		clientFunc  func(t *testing.T) *github.Client
		wantErr     bool
		errContains string
	}{
		{
			name: "rate limit healthy",
			clientFunc: func(t *testing.T) *github.Client {
				payload := `{"resources":{"core":{"limit":5000,"remaining":4000,"reset":` + fmt.Sprintf("%d", resetTime) + `}}}`
				return newStubGitHubClient(t, func(req *http.Request) (*http.Response, error) {
					if req.URL.Path != "/rate_limit" {
						t.Fatalf("expected /rate_limit path, got %s", req.URL.Path)
					}
					return jsonResponse(req, http.StatusOK, payload), nil
				})
			},
			wantErr: false,
		},
		{
			name: "rate limit low",
			clientFunc: func(t *testing.T) *github.Client {
				payload := `{"resources":{"core":{"limit":1000,"remaining":50,"reset":` + fmt.Sprintf("%d", resetTime) + `}}}`
				return newStubGitHubClient(t, func(req *http.Request) (*http.Response, error) {
					return jsonResponse(req, http.StatusOK, payload), nil
				})
			},
			wantErr:     true,
			errContains: "GitHub API rate limit critically low",
		},
		{
			name: "rate limit fetch error",
			clientFunc: func(t *testing.T) *github.Client {
				return newStubGitHubClient(t, func(req *http.Request) (*http.Response, error) {
					return jsonResponse(req, http.StatusInternalServerError, "{}"), nil
				})
			},
			wantErr:     true,
			errContains: "failed to get GitHub API rate limits",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := tt.clientFunc(t)
			err := CheckRateLimit(context.Background(), client)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("error = %v, want substring %q", err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestIsRateLimitCritical(t *testing.T) {
	tests := []struct {
		name string
		rate *github.Rate
		want bool
	}{
		{name: "nil", rate: nil, want: false},
		{name: "above threshold", rate: &github.Rate{Limit: 1000, Remaining: 200}, want: false},
		{name: "below threshold", rate: &github.Rate{Limit: 1000, Remaining: 50}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRateLimitCritical(tt.rate); got != tt.want {
				t.Fatalf("IsRateLimitCritical() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAuthenticationErrorHelpers(t *testing.T) {
	authErr := &AuthenticationError{Operation: "test", Err: fmt.Errorf("test error")}
	wrappedAuthErr := fmt.Errorf("wrapped: %w", authErr)
	otherErr := fmt.Errorf("other error")

	tests := []struct {
		name     string
		err      error
		checker  func(error) bool
		expected bool
	}{
		{"AuthenticationError direct", authErr, IsAuthenticationError, true},
		{"AuthenticationError wrapped", wrappedAuthErr, IsAuthenticationError, true},
		{"Other error", otherErr, IsAuthenticationError, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.checker(tt.err); got != tt.expected {
				t.Fatalf("expected %v, got %v", tt.expected, got)
			}
		})
	}

	t.Run("AsAuthenticationError", func(t *testing.T) {
		extracted, ok := AsAuthenticationError(authErr)
		if !ok {
			t.Fatalf("expected true for AuthenticationError")
		}
		if extracted != authErr {
			t.Fatalf("expected original error back")
		}

		if _, ok := AsAuthenticationError(otherErr); ok {
			t.Fatalf("expected false for non AuthenticationError")
		}
	})
}

func TestAuthenticationError(t *testing.T) {
	err := &AuthenticationError{Operation: "validate token", Err: fmt.Errorf("invalid token")}

	expected := "broker: authentication error during validate token: invalid token"
	if err.Error() != expected {
		t.Fatalf("AuthenticationError.Error() = %q, want %q", err.Error(), expected)
	}

	unwrapped := err.Unwrap()
	if unwrapped.Error() != "invalid token" {
		t.Fatalf("AuthenticationError.Unwrap() = %q, want %q", unwrapped.Error(), "invalid token")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newStubGitHubClient(t *testing.T, fn roundTripFunc) *github.Client {
	t.Helper()
	httpClient := &http.Client{Transport: fn}
	client := github.NewClient(httpClient)
	base, err := url.Parse("https://api.github.example/")
	if err != nil {
		t.Fatalf("failed to parse base URL: %v", err)
	}
	client.BaseURL = base
	return client
}

func jsonResponse(req *http.Request, status int, body string) *http.Response {
	resp := &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
	resp.Header.Set("Content-Type", "application/json")
	return resp
}
