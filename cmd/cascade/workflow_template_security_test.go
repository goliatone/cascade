package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDependencyWorkflowTemplateSecurityGuards(t *testing.T) {
	path := filepath.Join("..", "data", "workflows", "dependency-registration.yml.tpl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	template := string(data)

	mustContain := []string{
		"permissions:\n  contents: read\n  issues: write\n  pull-requests: write\n  actions: write",
		"Validate secret",
		"::add-mask::${TOKEN}",
	}
	for _, needle := range mustContain {
		if !strings.Contains(template, needle) {
			t.Fatalf("template missing expected content: %q", needle)
		}
	}
}
