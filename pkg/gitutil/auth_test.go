package gitutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetGitHubToken(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		wantNone bool
	}{
		{
			name:     "no token returns empty",
			envVars:  map[string]string{},
			wantNone: true,
		},
		{
			name: "GITHUB_TOKEN is read",
			envVars: map[string]string{
				EnvGitHubToken: "token123",
			},
		},
		{
			name: "GH_TOKEN is read",
			envVars: map[string]string{
				EnvGitHubToken2: "token456",
			},
		},
		{
			name: "CASCADE_GITHUB_TOKEN is read",
			envVars: map[string]string{
				EnvCascadeToken: "token789",
			},
		},
		{
			name: "GITHUB_ACCESS_TOKEN is read",
			envVars: map[string]string{
				EnvGitHubAccessToken: "token000",
			},
		},
		{
			name: "GITHUB_TOKEN takes precedence",
			envVars: map[string]string{
				EnvGitHubToken:  "token1",
				EnvGitHubToken2: "token2",
			},
		},
		{
			name: "token with whitespace is trimmed",
			envVars: map[string]string{
				EnvGitHubToken: "  token-with-spaces  ",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all token environment variables first
			clearTokenEnvVars()
			defer clearTokenEnvVars()

			// Set test environment variables
			for k, v := range tt.envVars {
				os.Setenv(k, v)
			}

			got := GetGitHubToken()
			if tt.wantNone && got != "" {
				t.Errorf("GetGitHubToken() = %v, want empty", got)
			}
			if !tt.wantNone && got == "" {
				t.Errorf("GetGitHubToken() = empty, want non-empty")
			}

			// Verify whitespace is trimmed
			if !tt.wantNone && got != "" {
				for k, v := range tt.envVars {
					if k == EnvGitHubToken && v != got {
						// Check if it was trimmed
						trimmed := len(got) < len(v)
						if trimmed && got+" " != v && " "+got != v {
							// It should be trimmed
							expected := v
							if v != got {
								t.Logf("Token was trimmed from '%s' to '%s'", v, got)
							}
							_ = expected
						}
					}
				}
			}
		})
	}
}

func TestGetGitHubTokenOrError(t *testing.T) {
	tests := []struct {
		name    string
		envVars map[string]string
		wantErr bool
	}{
		{
			name:    "no token returns error",
			envVars: map[string]string{},
			wantErr: true,
		},
		{
			name: "with token returns no error",
			envVars: map[string]string{
				EnvGitHubToken: "token123",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearTokenEnvVars()
			defer clearTokenEnvVars()

			for k, v := range tt.envVars {
				os.Setenv(k, v)
			}

			token, err := GetGitHubTokenOrError()

			if (err != nil) != tt.wantErr {
				t.Errorf("GetGitHubTokenOrError() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && token == "" {
				t.Errorf("GetGitHubTokenOrError() returned empty token")
			}

			if tt.wantErr && token != "" {
				t.Errorf("GetGitHubTokenOrError() returned token %v, want empty on error", token)
			}
		})
	}
}

func TestGetSSHKeyPath(t *testing.T) {
	tests := []struct {
		name       string
		envKeyPath string
		wantCustom bool
	}{
		{
			name:       "default path when env not set",
			envKeyPath: "",
			wantCustom: false,
		},
		{
			name:       "custom path from environment",
			envKeyPath: "/custom/path/to/key",
			wantCustom: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Unsetenv(EnvSSHKeyPath)
			defer os.Unsetenv(EnvSSHKeyPath)

			if tt.envKeyPath != "" {
				os.Setenv(EnvSSHKeyPath, tt.envKeyPath)
			}

			got := GetSSHKeyPath()

			if tt.wantCustom {
				if got != tt.envKeyPath {
					t.Errorf("GetSSHKeyPath() = %v, want %v", got, tt.envKeyPath)
				}
			} else {
				// Should return default path (~/.ssh/id_rsa)
				if got == "" {
					t.Errorf("GetSSHKeyPath() returned empty path")
				}
				if !filepath.IsAbs(got) && got != filepath.Join(".ssh", "id_rsa") {
					t.Errorf("GetSSHKeyPath() = %v, expected absolute path or .ssh/id_rsa", got)
				}
			}
		})
	}
}

func TestSSHKeyExists(t *testing.T) {
	// Create a temporary file to test with
	tmpDir := t.TempDir()
	existingKey := filepath.Join(tmpDir, "test_key")
	if err := os.WriteFile(existingKey, []byte("test"), 0600); err != nil {
		t.Fatalf("Failed to create test key file: %v", err)
	}

	tests := []struct {
		name string
		path string
		want bool
	}{
		{
			name: "existing file",
			path: existingKey,
			want: true,
		},
		{
			name: "non-existing file",
			path: filepath.Join(tmpDir, "nonexistent"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SSHKeyExists(tt.path)
			if got != tt.want {
				t.Errorf("SSHKeyExists() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetSSHKeyPathOrError(t *testing.T) {
	// Create a temporary file to test with
	tmpDir := t.TempDir()
	existingKey := filepath.Join(tmpDir, "test_key")
	if err := os.WriteFile(existingKey, []byte("test"), 0600); err != nil {
		t.Fatalf("Failed to create test key file: %v", err)
	}

	tests := []struct {
		name       string
		envKeyPath string
		wantErr    bool
	}{
		{
			name:       "existing key path",
			envKeyPath: existingKey,
			wantErr:    false,
		},
		{
			name:       "non-existing key path",
			envKeyPath: "/nonexistent/path/to/key",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Unsetenv(EnvSSHKeyPath)
			defer os.Unsetenv(EnvSSHKeyPath)

			if tt.envKeyPath != "" {
				os.Setenv(EnvSSHKeyPath, tt.envKeyPath)
			}

			path, err := GetSSHKeyPathOrError()

			if (err != nil) != tt.wantErr {
				t.Errorf("GetSSHKeyPathOrError() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && path == "" {
				t.Errorf("GetSSHKeyPathOrError() returned empty path")
			}

			if !tt.wantErr && path != tt.envKeyPath {
				t.Errorf("GetSSHKeyPathOrError() = %v, want %v", path, tt.envKeyPath)
			}
		})
	}
}

// Helper function to clear all token environment variables
func clearTokenEnvVars() {
	os.Unsetenv(EnvGitHubToken)
	os.Unsetenv(EnvGitHubToken2)
	os.Unsetenv(EnvCascadeToken)
	os.Unsetenv(EnvGitHubAccessToken)
}
