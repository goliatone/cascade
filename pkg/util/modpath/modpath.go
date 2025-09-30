package modpath

import (
    "strings"
)

// DeriveRepository converts module paths into owner/repo identifiers for common hosts.
func DeriveRepository(modulePath string) string {
    if modulePath == "" {
        return ""
    }
    parts := strings.Split(modulePath, "/")
    if len(parts) >= 3 {
        switch parts[0] {
        case "github.com", "gitlab.com", "bitbucket.org":
            return strings.Join(parts[1:3], "/")
        }
    }
    return modulePath
}

// DeriveLocalModulePath returns the path under the repo root for the module.
func DeriveLocalModulePath(modulePath string) string {
    parts := strings.Split(modulePath, "/")
    if len(parts) >= 4 {
        switch parts[0] {
        case "github.com", "gitlab.com", "bitbucket.org":
            return strings.Join(parts[3:], "/")
        }
    }
    return "."
}

// BuildCloneURL normalises repository identifiers into HTTPS clone URLs.
func BuildCloneURL(repo string) string {
    if strings.HasPrefix(repo, "https://") || strings.HasPrefix(repo, "http://") || strings.HasPrefix(repo, "git@") {
        return repo
    }
    if strings.Count(repo, "/") == 1 {
        return "https://github.com/" + repo
    }
    return repo
}
