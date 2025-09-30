package gitutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Environment variable names for authentication configuration.
const (
	// EnvGitHubToken is the primary GitHub token environment variable
	EnvGitHubToken = "GITHUB_TOKEN"

	// EnvGitHubToken2 is an alternative GitHub token environment variable
	EnvGitHubToken2 = "GH_TOKEN"

	// EnvGitHubAccessToken is another common GitHub token environment variable
	EnvGitHubAccessToken = "GITHUB_ACCESS_TOKEN"

	// EnvCascadeToken is the Cascade-specific GitHub token environment variable
	EnvCascadeToken = "CASCADE_GITHUB_TOKEN"

	// EnvSSHKeyPath is the environment variable for custom SSH key path
	EnvSSHKeyPath = "SSH_KEY_PATH"
)

// GetGitHubToken retrieves a GitHub token from environment variables.
// Checks multiple common environment variable names in order of precedence:
// 1. GITHUB_TOKEN
// 2. GH_TOKEN
// 3. CASCADE_GITHUB_TOKEN
// 4. GITHUB_ACCESS_TOKEN
// Returns empty string if no token is found.
func GetGitHubToken() string {
	// Check environment variables in order of precedence
	envVars := []string{
		EnvGitHubToken,
		EnvGitHubToken2,
		EnvCascadeToken,
		EnvGitHubAccessToken,
	}

	for _, envVar := range envVars {
		if token := os.Getenv(envVar); token != "" {
			return strings.TrimSpace(token)
		}
	}

	return ""
}

// GetGitHubTokenOrError retrieves a GitHub token or returns an error if not found.
// This is useful when a token is required for an operation.
func GetGitHubTokenOrError() (string, error) {
	token := GetGitHubToken()
	if token == "" {
		return "", fmt.Errorf("GitHub token not found: set one of %s, %s, %s, or %s environment variables",
			EnvGitHubToken, EnvGitHubToken2, EnvCascadeToken, EnvGitHubAccessToken)
	}
	return token, nil
}

// GetSSHKeyPath returns the SSH key path from environment or the default location.
// Checks SSH_KEY_PATH environment variable first, then falls back to ~/.ssh/id_rsa.
func GetSSHKeyPath() string {
	// Check environment variable first
	if sshKeyPath := os.Getenv(EnvSSHKeyPath); sshKeyPath != "" {
		return sshKeyPath
	}

	// Fall back to default SSH key location
	home, err := os.UserHomeDir()
	if err != nil {
		// If we can't get home dir, return a reasonable default
		return filepath.Join(".ssh", "id_rsa")
	}

	return filepath.Join(home, ".ssh", "id_rsa")
}

// SSHKeyExists checks if an SSH key file exists at the given path.
func SSHKeyExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// GetSSHKeyPathOrError returns the SSH key path or an error if it doesn't exist.
func GetSSHKeyPathOrError() (string, error) {
	keyPath := GetSSHKeyPath()
	if !SSHKeyExists(keyPath) {
		return "", fmt.Errorf("SSH key not found at %s", keyPath)
	}
	return keyPath, nil
}
