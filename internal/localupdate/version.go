package localupdate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func readLocalVersion(repoPath string) (string, error) {
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
	return "", fmt.Errorf("local version file not found")
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
