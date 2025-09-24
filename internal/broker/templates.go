package broker

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/goliatone/cascade/internal/executor"
	"github.com/goliatone/cascade/internal/planner"
)

// TemplateData contains all available data for template rendering.
type TemplateData struct {
	// Work item data
	Module        string
	ModulePath    string
	SourceModule  string
	SourceVersion string
	Repo          string
	Branch        string
	BranchName    string
	CommitMessage string
	Labels        []string

	// Execution result data
	Status       string
	Reason       string
	CommitHash   string
	TestOutputs  []string
	ExtraOutputs []string

	// Metadata
	Timestamp time.Time
}

// Default templates
const (
	defaultTitleTemplate = "Update {{.Module}} to {{.SourceVersion}}"
	defaultBodyTemplate  = `## Summary
Updates {{.Module}} from current version to {{.SourceVersion}}.

**Repository**: {{.Repo}}
{{if .BranchName}}**Branch**: {{.BranchName}}{{end}}
{{if .Status}}**Status**: {{.Status}}{{end}}
{{if .CommitHash}}**Commit**: {{.CommitHash}}{{end}}

{{if .Reason}}## Details
{{.Reason}}
{{end}}

{{if .TestOutputs}}## Test Results
{{range .TestOutputs}}
<details>
<summary>Test Output</summary>

` + "```" + `
{{.}}
` + "```" + `

</details>
{{end}}
{{end}}

{{if .ExtraOutputs}}## Additional Command Results
{{range .ExtraOutputs}}
<details>
<summary>Command Output</summary>

` + "```" + `
{{.}}
` + "```" + `

</details>
{{end}}
{{end}}

Generated at {{.Timestamp.Format "2006-01-02 15:04:05 MST"}}`
)

// templateFuncMap provides safe template functions
var templateFuncMap = template.FuncMap{
	"upper":       strings.ToUpper,
	"lower":       strings.ToLower,
	"title":       strings.Title,
	"truncate":    truncateString,
	"truncate8":   func(s string) string { return truncateString(s, 8) },
	"truncate200": func(s string) string { return truncateString(s, 200) },
	"escape":      escapeMarkdown,
	"join":        joinStrings,
}

// RenderTitle renders a PR title from a template with work item and result data.
func RenderTitle(tmpl string, item planner.WorkItem, result *executor.Result) (string, error) {
	if tmpl == "" {
		tmpl = defaultTitleTemplate
	}

	data := buildTemplateData(item, result)
	return renderTemplate("title", tmpl, data)
}

// RenderBody renders a PR body from a template with work item and result data.
func RenderBody(tmpl string, item planner.WorkItem, result *executor.Result) (string, error) {
	if tmpl == "" {
		tmpl = defaultBodyTemplate
	}

	data := buildTemplateData(item, result)
	return renderTemplate("body", tmpl, data)
}

// buildTemplateData creates template data from work item and result.
func buildTemplateData(item planner.WorkItem, result *executor.Result) TemplateData {
	data := TemplateData{
		Module:        item.Module,
		ModulePath:    item.ModulePath,
		SourceModule:  item.SourceModule,
		SourceVersion: item.SourceVersion,
		Repo:          item.Repo,
		Branch:        item.Branch,
		BranchName:    item.BranchName,
		CommitMessage: item.CommitMessage,
		Labels:        item.Labels,
		Timestamp:     time.Now(),
	}

	if result != nil {
		data.Status = string(result.Status)
		data.Reason = result.Reason
		data.CommitHash = result.CommitHash

		// Collect test outputs (truncated for safety)
		for _, testResult := range result.TestResults {
			if testResult.Output != "" {
				data.TestOutputs = append(data.TestOutputs, truncateString(testResult.Output, 1000))
			}
		}

		// Collect extra command outputs (truncated for safety)
		for _, extraResult := range result.ExtraResults {
			if extraResult.Output != "" {
				data.ExtraOutputs = append(data.ExtraOutputs, truncateString(extraResult.Output, 1000))
			}
		}
	}

	return data
}

// renderTemplate executes a template with the given data.
func renderTemplate(name, tmpl string, data TemplateData) (string, error) {
	t, err := template.New(name).Funcs(templateFuncMap).Parse(tmpl)
	if err != nil {
		// Return default on parse error
		if name == "title" {
			fallback, fallbackErr := renderTemplate("title-fallback", defaultTitleTemplate, data)
			if fallbackErr != nil {
				return fmt.Sprintf("Update %s to %s", data.Module, data.SourceVersion), nil
			}
			return fallback, &TemplateRenderError{
				TemplateName: name,
				Operation:    "parse",
				Err:          err,
			}
		}
		fallback, fallbackErr := renderTemplate("body-fallback", defaultBodyTemplate, data)
		if fallbackErr != nil {
			return fmt.Sprintf("Update %s to %s\n\nStatus: %s", data.Module, data.SourceVersion, data.Status), nil
		}
		return fallback, &TemplateRenderError{
			TemplateName: name,
			Operation:    "parse",
			Err:          err,
		}
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		// Return default on execution error
		if name == "title" {
			return fmt.Sprintf("Update %s to %s", data.Module, data.SourceVersion), &TemplateRenderError{
				TemplateName: name,
				Operation:    "execute",
				Err:          err,
			}
		}
		return fmt.Sprintf("Update %s to %s\n\nStatus: %s", data.Module, data.SourceVersion, data.Status), &TemplateRenderError{
			TemplateName: name,
			Operation:    "execute",
			Err:          err,
		}
	}

	return buf.String(), nil
}

// truncateString truncates a string to maxLen characters, adding ellipsis if needed.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// escapeMarkdown escapes special markdown characters to prevent injection.
func escapeMarkdown(s string) string {
	// Order matters: escape & first to avoid double-escaping
	result := strings.ReplaceAll(s, "&", "&amp;")
	result = strings.ReplaceAll(result, "<", "&lt;")
	result = strings.ReplaceAll(result, ">", "&gt;")
	result = strings.ReplaceAll(result, "`", "\\`")
	result = strings.ReplaceAll(result, "*", "\\*")
	result = strings.ReplaceAll(result, "_", "\\_")
	result = strings.ReplaceAll(result, "#", "\\#")
	result = strings.ReplaceAll(result, "[", "\\[")
	result = strings.ReplaceAll(result, "]", "\\]")
	result = strings.ReplaceAll(result, "(", "\\(")
	result = strings.ReplaceAll(result, ")", "\\)")
	result = strings.ReplaceAll(result, "|", "\\|")
	return result
}

// joinStrings is a template function that joins string slices.
func joinStrings(elems []string, sep string) string {
	return strings.Join(elems, sep)
}
