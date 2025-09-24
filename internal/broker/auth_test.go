package broker

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
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
			// Clear all relevant environment variables
			envVars := []string{"GITHUB_TOKEN", "GITHUB_ACCESS_TOKEN", "GH_TOKEN"}
			for _, env := range envVars {
				os.Unsetenv(env)
			}

			// Set test environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			// Clean up after test
			defer func() {
				for key := range tt.envVars {
					os.Unsetenv(key)
				}
			}()

			got, err := LoadGitHubToken()

			if tt.wantErr {
				if err == nil {
					t.Errorf("LoadGitHubToken() expected error, got nil")
					return
				}
				if tt.errMessage != "" && err.Error() != fmt.Sprintf("GitHub token not found: set one of [GITHUB_TOKEN GITHUB_ACCESS_TOKEN GH_TOKEN] environment variables") {
					t.Errorf("LoadGitHubToken() error = %v, want error containing %q", err, tt.errMessage)
				}
				return
			}

			if err != nil {
				t.Errorf("LoadGitHubToken() unexpected error = %v", err)
				return
			}

			if got != tt.wantToken {
				t.Errorf("LoadGitHubToken() = %v, want %v", got, tt.wantToken)
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
					t.Errorf("CreateAuthenticatedClient() expected error, got nil")
				}
				if client != nil {
					t.Errorf("CreateAuthenticatedClient() expected nil client on error, got %v", client)
				}
				return
			}

			if err != nil {
				t.Errorf("CreateAuthenticatedClient() unexpected error = %v", err)
				return
			}

			if client == nil {
				t.Errorf("CreateAuthenticatedClient() returned nil client")
			}
		})
	}
}

func TestValidateAuthentication(t *testing.T) {
	tests := []struct {
		name        string
		setupServer func() *httptest.Server
		client      *github.Client
		wantErr     bool
		errContains string
	}{
		{
			name: "successful authentication",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path != "/user" {
						t.Errorf("Expected /user path, got %s", r.URL.Path)
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"login": "testuser", "id": 12345}`))
				}))
			},
			wantErr: false,
		},
		{
			name: "unauthorized",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusUnauthorized)
					w.Write([]byte(`{"message": "Bad credentials"}`))
				}))
			},
			wantErr:     true,
			errContains: "invalid or expired token",
		},
		{
			name: "server error",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				}))
			},
			wantErr:     true,
			errContains: "authentication validation failed",
		},
		{
			name:        "nil client",
			client:      nil,
			wantErr:     true,
			errContains: "GitHub client is nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var client *github.Client

			if tt.setupServer != nil {
				server := tt.setupServer()
				defer server.Close()

				// Parse server URL with trailing slash
				serverURL, err := url.Parse(server.URL + "/")
				if err != nil {
					t.Fatalf("Failed to parse server URL: %v", err)
				}

				// Create client with test server
				client = github.NewClient(server.Client())
				client.BaseURL = serverURL
			} else if tt.client != nil {
				client = tt.client
			}

			ctx := context.Background()
			err := ValidateAuthentication(ctx, client)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateAuthentication() expected error, got nil")
					return
				}
				if tt.errContains != "" && !containsString(err.Error(), tt.errContains) {
					t.Errorf("ValidateAuthentication() error = %v, want error containing %q", err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("ValidateAuthentication() unexpected error = %v", err)
			}
		})
	}
}

func TestGetRateLimit(t *testing.T) {
	tests := []struct {
		name        string
		setupServer func() *httptest.Server
		client      *github.Client
		wantErr     bool
		errContains string
	}{
		{
			name: "successful rate limit fetch",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path != "/rate_limit" {
						t.Errorf("Expected /rate_limit path, got %s", r.URL.Path)
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{
						"rate": {
							"limit": 5000,
							"remaining": 4999,
							"reset": 1625097600,
							"used": 1
						},
						"resources": {
							"core": {
								"limit": 5000,
								"remaining": 4999,
								"reset": 1625097600,
								"used": 1
							}
						}
					}`))
				}))
			},
			wantErr: false,
		},
		{
			name: "server error",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				}))
			},
			wantErr:     true,
			errContains: "failed to get GitHub API rate limits",
		},
		{
			name:        "nil client",
			client:      nil,
			wantErr:     true,
			errContains: "GitHub client is nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var client *github.Client

			if tt.setupServer != nil {
				server := tt.setupServer()
				defer server.Close()

				// Parse server URL with trailing slash
				serverURL, err := url.Parse(server.URL + "/")
				if err != nil {
					t.Fatalf("Failed to parse server URL: %v", err)
				}

				// Create client with test server
				client = github.NewClient(server.Client())
				client.BaseURL = serverURL
			} else if tt.client != nil {
				client = tt.client
			}

			ctx := context.Background()
			rateLimits, err := GetRateLimit(ctx, client)

			if tt.wantErr {
				if err == nil {
					t.Errorf("GetRateLimit() expected error, got nil")
					return
				}
				if tt.errContains != "" && !containsString(err.Error(), tt.errContains) {
					t.Errorf("GetRateLimit() error = %v, want error containing %q", err, tt.errContains)
				}
				if rateLimits != nil {
					t.Errorf("GetRateLimit() expected nil rateLimits on error, got %v", rateLimits)
				}
				return
			}

			if err != nil {
				t.Errorf("GetRateLimit() unexpected error = %v", err)
				return
			}

			if rateLimits == nil {
				t.Errorf("GetRateLimit() returned nil rateLimits")
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
		{
			name: "nil rate",
			rate: nil,
			want: false,
		},
		{
			name: "high remaining",
			rate: &github.Rate{
				Limit:     5000,
				Remaining: 4000,
			},
			want: false,
		},
		{
			name: "critical remaining (exactly 10%)",
			rate: &github.Rate{
				Limit:     1000,
				Remaining: 100,
			},
			want: false, // 100 is exactly 10%, so not less than 10%
		},
		{
			name: "critical remaining (less than 10%)",
			rate: &github.Rate{
				Limit:     1000,
				Remaining: 99,
			},
			want: true, // 99 is less than 10% of 1000
		},
		{
			name: "zero remaining",
			rate: &github.Rate{
				Limit:     5000,
				Remaining: 0,
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsRateLimitCritical(tt.rate)
			if got != tt.want {
				t.Errorf("IsRateLimitCritical() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckRateLimit(t *testing.T) {
	tests := []struct {
		name        string
		setupServer func() *httptest.Server
		wantErr     bool
		errContains string
	}{
		{
			name: "healthy rate limit",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{
						"resources": {
							"core": {
								"limit": 5000,
								"remaining": 4000,
								"reset": 1625097600
							}
						}
					}`))
				}))
			},
			wantErr: false,
		},
		{
			name: "critical rate limit",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					resetTime := time.Now().Add(time.Hour).Unix()
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(fmt.Sprintf(`{
						"resources": {
							"core": {
								"limit": 1000,
								"remaining": 50,
								"reset": %d
							}
						}
					}`, resetTime)))
				}))
			},
			wantErr:     true,
			errContains: "GitHub API rate limit critically low",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			defer server.Close()

			// Parse server URL with trailing slash
			serverURL, err := url.Parse(server.URL + "/")
			if err != nil {
				t.Fatalf("Failed to parse server URL: %v", err)
			}

			// Create client with test server
			client := github.NewClient(server.Client())
			client.BaseURL = serverURL

			ctx := context.Background()
			err = CheckRateLimit(ctx, client)

			if tt.wantErr {
				if err == nil {
					t.Errorf("CheckRateLimit() expected error, got nil")
					return
				}
				if tt.errContains != "" && !containsString(err.Error(), tt.errContains) {
					t.Errorf("CheckRateLimit() error = %v, want error containing %q", err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("CheckRateLimit() unexpected error = %v", err)
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
			result := tt.checker(tt.err)
			if result != tt.expected {
				t.Errorf("%s: expected %v, got %v", tt.name, tt.expected, result)
			}
		})
	}

	// Test AsAuthenticationError
	t.Run("AsAuthenticationError", func(t *testing.T) {
		extracted, ok := AsAuthenticationError(authErr)
		if !ok {
			t.Error("AsAuthenticationError should return true for AuthenticationError")
		}
		if extracted != authErr {
			t.Error("AsAuthenticationError should return the original error")
		}

		_, ok = AsAuthenticationError(otherErr)
		if ok {
			t.Error("AsAuthenticationError should return false for non-AuthenticationError")
		}
	})
}

func TestAuthenticationError(t *testing.T) {
	err := &AuthenticationError{
		Operation: "validate token",
		Err:       fmt.Errorf("invalid token"),
	}

	expected := "broker: authentication error during validate token: invalid token"
	if err.Error() != expected {
		t.Errorf("AuthenticationError.Error() = %q, want %q", err.Error(), expected)
	}

	unwrapped := err.Unwrap()
	if unwrapped.Error() != "invalid token" {
		t.Errorf("AuthenticationError.Unwrap() = %q, want %q", unwrapped.Error(), "invalid token")
	}
}

// containsString checks if a string contains a substring.
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
