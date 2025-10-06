package main

import (
	"embed"
	"fmt"
	"os"
	"strings"
)

var (
	//go:embed templates/workflow/github_actions.yaml.tmpl
	workflowTemplatesFS embed.FS
)

const defaultWorkflowTemplatePath = "templates/workflow/github_actions.yaml.tmpl"

// loadWorkflowTemplate returns the workflow template to render. If templatePath is provided,
// the file is read from disk. Otherwise, the embedded default template is used.
func loadWorkflowTemplate(templatePath string) ([]byte, error) {
	path := strings.TrimSpace(templatePath)
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, newFileError(fmt.Sprintf("failed to read template %s", path), err)
		}
		return data, nil
	}

	data, err := workflowTemplatesFS.ReadFile(defaultWorkflowTemplatePath)
	if err != nil {
		return nil, newFileError("failed to load embedded workflow template", err)
	}
	return data, nil
}
