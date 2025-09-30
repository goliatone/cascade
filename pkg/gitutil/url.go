package gitutil

import (
	"fmt"
	"strings"
)

// ParseRepoURL parses a repository string into a structured RepoURL.
// Handles various formats:
// - user/repo (assumes GitHub)
// - github.com/user/repo
// - https://github.com/user/repo.git
// - git@github.com:user/repo.git
func ParseRepoURL(repo string) (*RepoURL, error) {
	if repo == "" {
		return nil, fmt.Errorf("repository string cannot be empty")
	}

	var result RepoURL

	// Trim whitespace and .git suffix
	repo = strings.TrimSpace(repo)
	originalRepo := repo

	// Detect SSH format: git@host:owner/repo.git
	if strings.HasPrefix(repo, "git@") {
		result.Protocol = ProtocolSSH
		// Extract host and path from git@github.com:user/repo.git
		parts := strings.SplitN(repo, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid SSH URL format: %s", repo)
		}

		// Extract host from git@github.com
		hostPart := strings.TrimPrefix(parts[0], "git@")
		result.Host = hostPart

		// Extract owner/repo from user/repo.git
		path := strings.TrimSuffix(parts[1], ".git")
		ownerRepo := strings.Split(path, "/")
		if len(ownerRepo) < 2 {
			return nil, fmt.Errorf("invalid SSH URL path: %s", repo)
		}
		result.Owner = ownerRepo[0]
		result.Name = ownerRepo[len(ownerRepo)-1]

		result.CloneURL = repo
		result.SSHURL = fmt.Sprintf("git@%s:%s/%s.git", result.Host, result.Owner, result.Name)
		result.HTTPSURL = fmt.Sprintf("https://%s/%s/%s.git", result.Host, result.Owner, result.Name)
		return &result, nil
	}

	// Detect HTTPS/HTTP format: https://github.com/user/repo.git
	if strings.HasPrefix(repo, "https://") || strings.HasPrefix(repo, "http://") {
		if strings.HasPrefix(repo, "https://") {
			result.Protocol = ProtocolHTTPS
		} else {
			result.Protocol = ProtocolGit
		}

		// Remove protocol prefix
		repo = strings.TrimPrefix(repo, "https://")
		repo = strings.TrimPrefix(repo, "http://")
		repo = strings.TrimSuffix(repo, ".git")

		// Split into host/owner/repo
		parts := strings.Split(repo, "/")
		if len(parts) < 3 {
			return nil, fmt.Errorf("invalid HTTPS URL format: %s", originalRepo)
		}

		result.Host = parts[0]
		result.Owner = parts[1]
		result.Name = parts[len(parts)-1]

		result.CloneURL = originalRepo
		result.HTTPSURL = fmt.Sprintf("https://%s/%s/%s.git", result.Host, result.Owner, result.Name)
		result.SSHURL = fmt.Sprintf("git@%s:%s/%s.git", result.Host, result.Owner, result.Name)
		return &result, nil
	}

	// Handle shorthand formats
	parts := strings.Split(repo, "/")
	switch len(parts) {
	case 2:
		// Format: "user/repo" - assume GitHub
		result.Host = "github.com"
		result.Owner = parts[0]
		result.Name = strings.TrimSuffix(parts[1], ".git")
		result.Protocol = ProtocolHTTPS

	case 3:
		// Format: "github.com/user/repo" or "gitlab.com/user/repo"
		result.Host = parts[0]
		result.Owner = parts[1]
		result.Name = strings.TrimSuffix(parts[2], ".git")
		result.Protocol = ProtocolHTTPS

	default:
		// Try using it as-is with github.com prefix
		result.Host = "github.com"
		result.Owner = ""
		result.Name = strings.TrimSuffix(repo, ".git")
		result.Protocol = ProtocolHTTPS
	}

	// Validate we have required fields
	if result.Host == "" || result.Name == "" {
		return nil, fmt.Errorf("invalid repository format: %s", originalRepo)
	}

	// Build URLs
	if result.Owner != "" {
		result.HTTPSURL = fmt.Sprintf("https://%s/%s/%s.git", result.Host, result.Owner, result.Name)
		result.SSHURL = fmt.Sprintf("git@%s:%s/%s.git", result.Host, result.Owner, result.Name)
	} else {
		result.HTTPSURL = fmt.Sprintf("https://%s/%s.git", result.Host, result.Name)
		result.SSHURL = fmt.Sprintf("git@%s:%s.git", result.Host, result.Name)
	}
	result.CloneURL = result.HTTPSURL

	return &result, nil
}

// BuildCloneURL constructs a cloneable git URL from a repository string.
// If the string is already a valid URL, returns it as-is.
// For shorthand formats (user/repo), constructs an HTTPS URL.
func BuildCloneURL(repo string, protocol Protocol) (string, error) {
	parsed, err := ParseRepoURL(repo)
	if err != nil {
		return "", err
	}

	switch protocol {
	case ProtocolSSH:
		return parsed.SSHURL, nil
	case ProtocolHTTPS, ProtocolGit:
		return parsed.HTTPSURL, nil
	default:
		return parsed.CloneURL, nil
	}
}

// NormalizeURL normalizes git URLs for comparison.
// Removes .git suffix, converts SSH to HTTPS format, and lowercases.
func NormalizeURL(url string) string {
	url = strings.TrimSpace(url)

	// Remove .git suffix
	url = strings.TrimSuffix(url, ".git")

	// Convert SSH to HTTPS format for comparison
	if strings.HasPrefix(url, "git@") {
		// Convert git@github.com:user/repo to https://github.com/user/repo
		url = strings.TrimPrefix(url, "git@")
		url = strings.Replace(url, ":", "/", 1)
		url = "https://" + url
	}

	return strings.ToLower(url)
}

// ExtractRepoName extracts the repository name from a git URL or path.
func ExtractRepoName(repo string) string {
	// Parse the URL
	parsed, err := ParseRepoURL(repo)
	if err != nil {
		// Fallback: try to extract from the string directly
		repo = strings.TrimSuffix(repo, ".git")
		if strings.Contains(repo, "/") {
			parts := strings.Split(repo, "/")
			return parts[len(parts)-1]
		}
		return repo
	}

	return parsed.Name
}

// ExtractOwnerAndRepo extracts owner and repository name from a URL or path.
// Returns owner and repo name separately.
func ExtractOwnerAndRepo(repo string) (owner, name string, err error) {
	parsed, err := ParseRepoURL(repo)
	if err != nil {
		return "", "", err
	}

	if parsed.Owner == "" {
		return "", "", fmt.Errorf("repository format does not contain owner: %s", repo)
	}

	return parsed.Owner, parsed.Name, nil
}
