package localupdate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var errLocalVersionNotFound = errors.New("local version file not found")

func readLocalVersion(module localModule) (string, error) {
	for _, dir := range versionLookupDirs(module) {
		version, err := readVersionFromDir(dir)
		if err != nil {
			if errors.Is(err, errLocalVersionNotFound) {
				continue
			}
			return "", err
		}
		return version, nil
	}
	return "", errLocalVersionNotFound
}

func versionLookupDirs(module localModule) []string {
	dirs := []string{}
	seen := map[string]bool{}
	for _, dir := range []string{module.ModuleDir, module.RepoRoot} {
		if strings.TrimSpace(dir) == "" {
			continue
		}
		clean := filepath.Clean(dir)
		if seen[clean] {
			continue
		}
		seen[clean] = true
		dirs = append(dirs, clean)
	}
	return dirs
}

func readVersionFromDir(repoPath string) (string, error) {
	for _, name := range []string{".version", "VERSION"} {
		candidate := filepath.Join(repoPath, name)
		data, err := os.ReadFile(candidate)
		if err != nil {
			if !os.IsNotExist(err) {
				return "", fmt.Errorf("read %s: %w", name, err)
			}
			continue
		}
		if version := normalizeGoVersion(string(data)); version != "" {
			return version, nil
		}
	}
	return "", errLocalVersionNotFound
}

func normalizeGoVersion(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "v") {
		return trimmed
	}
	if trimmed[0] >= '0' && trimmed[0] <= '9' {
		return "v" + trimmed
	}
	return trimmed
}
