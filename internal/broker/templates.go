package broker

import (
	"bytes"
	"fmt"
	"regexp"
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
	Status            string
	Reason            string
	CommitHash        string
	TestOutputs       []string
	ExtraOutputs      []string
	FailureSummary    string
	FailureMessage    string
	FailureCommand    string
	DependencyModule  string
	DependencyTarget  string
	DependencyOld     string
	DependencyNew     string
	DependencyApplied bool
	DependencySummary string
	DependencyNote    string

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

		if failure := extractFirstTestFailure(result.TestResults); failure != nil {
			data.FailureSummary = buildFailureSummary(failure)
			if failure.Message != "" {
				data.FailureMessage = truncateString(failure.Message, 280)
			}
			data.FailureCommand = failure.Command
		}

		if impact := result.DependencyImpact; impact != nil {
			data.DependencyModule = impact.Module
			data.DependencyTarget = impact.TargetVersion
			data.DependencyOld = impact.OldVersion
			data.DependencyNew = impact.NewVersion
			data.DependencyApplied = impact.Applied
			data.DependencySummary = formatDependencySummary(impact)
			data.DependencyNote = formatDependencyNote(impact)
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

type testFailureInsight struct {
	Package string
	Test    string
	Message string
	Command string
}

var (
	goTestFailLine    = regexp.MustCompile(`^--- FAIL: ([^ ]+)`)
	goTestPackageLine = regexp.MustCompile(`^FAIL(?:\t|\s+)([^\s]+)`)
)

func extractFirstTestFailure(results []executor.CommandResult) *testFailureInsight {
	for _, res := range results {
		insight := parseGoTestFailure(res.Output)
		if insight == nil && res.Err != nil {
			if execErr, ok := res.Err.(*executor.CommandExecutionError); ok {
				insight = parseGoTestFailure(execErr.Output)
			}
		}

		if insight != nil {
			if len(res.Command.Cmd) > 0 {
				insight.Command = strings.Join(res.Command.Cmd, " ")
			}
			if insight.Message == "" {
				trimmed := strings.TrimSpace(res.Output)
				if trimmed != "" {
					insight.Message = truncateString(trimmed, 280)
				} else if res.Err != nil {
					insight.Message = res.Err.Error()
				}
			}
			return insight
		}

		if res.Err != nil {
			fallback := &testFailureInsight{}
			if len(res.Command.Cmd) > 0 {
				fallback.Command = strings.Join(res.Command.Cmd, " ")
			}
			if execErr, ok := res.Err.(*executor.CommandExecutionError); ok {
				output := strings.TrimSpace(execErr.Output)
				if output != "" {
					fallback.Message = truncateString(output, 280)
				} else {
					fallback.Message = execErr.Error()
				}
			} else if res.Output != "" {
				fallback.Message = truncateString(strings.TrimSpace(res.Output), 280)
			} else {
				fallback.Message = res.Err.Error()
			}
			return fallback
		}
	}

	return nil
}

func parseGoTestFailure(output string) *testFailureInsight {
	if strings.TrimSpace(output) == "" {
		return nil
	}

	lines := strings.Split(output, "\n")
	var (
		failureName  string
		packageName  string
		messageParts []string
		capturing    bool
	)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if failureName == "" {
			if match := goTestFailLine.FindStringSubmatch(trimmed); match != nil {
				failureName = strings.TrimSpace(match[1])
				capturing = true
				continue
			}
		} else if capturing {
			switch {
			case goTestFailLine.MatchString(trimmed):
				capturing = false
			case strings.HasPrefix(trimmed, "FAIL"):
				capturing = false
			case strings.HasPrefix(trimmed, "=== RUN"):
				capturing = false
			case strings.HasPrefix(trimmed, "PASS"):
				capturing = false
			case strings.HasPrefix(trimmed, "ok"):
				capturing = false
			default:
				if trimmed != "" {
					messageParts = append(messageParts, trimmed)
				}
				continue
			}
		}

		if packageName == "" && trimmed != "" {
			if match := goTestPackageLine.FindStringSubmatch(trimmed); match != nil {
				packageName = strings.TrimSpace(match[1])
			}
		}
	}

	if failureName == "" && packageName == "" {
		return nil
	}

	insight := &testFailureInsight{
		Package: packageName,
		Test:    failureName,
	}

	if len(messageParts) > 0 {
		insight.Message = strings.Join(messageParts, " | ")
	}

	return insight
}

func buildFailureSummary(insight *testFailureInsight) string {
	if insight == nil {
		return ""
	}

	parts := []string{}
	if insight.Test != "" {
		parts = append(parts, insight.Test)
	}
	if insight.Package != "" {
		if len(parts) > 0 {
			parts = append(parts, fmt.Sprintf("(%s)", insight.Package))
		} else {
			parts = append(parts, insight.Package)
		}
	}
	if len(parts) == 0 && insight.Command != "" {
		return insight.Command
	}
	return strings.Join(parts, " ")
}

func formatDependencySummary(impact *executor.DependencyImpact) string {
	if impact == nil || impact.Module == "" {
		return ""
	}

	summary := impact.Module

	switch {
	case impact.NewVersionDetected && impact.NewVersion != "":
		summary += fmt.Sprintf(" -> %s", impact.NewVersion)
		if impact.OldVersionDetected && impact.OldVersion != "" && impact.NewVersion != impact.OldVersion {
			summary += fmt.Sprintf(" (was %s)", impact.OldVersion)
		}
	case impact.OldVersionDetected && impact.OldVersion != "":
		summary += fmt.Sprintf(" -> %s (no change)", impact.OldVersion)
	default:
		summary += " -> (not found in go.mod)"
	}

	if impact.TargetVersion != "" && impact.NewVersion != impact.TargetVersion {
		summary += fmt.Sprintf(" | target %s", impact.TargetVersion)
	}

	return summary
}

func formatDependencyNote(impact *executor.DependencyImpact) string {
	if impact == nil {
		return ""
	}

	var notes []string

	if !impact.Applied && impact.NewVersionDetected && impact.OldVersionDetected && impact.NewVersion == impact.OldVersion && impact.TargetVersion != "" && impact.NewVersion != impact.TargetVersion {
		notes = append(notes, "go.mod still on previous version; update may not have applied")
	}

	if impact.Applied && impact.TargetVersion != "" && impact.NewVersion != impact.TargetVersion {
		notes = append(notes, fmt.Sprintf("resolved to %s (target %s)", impact.NewVersion, impact.TargetVersion))
	}

	if len(impact.Notes) > 0 {
		notes = append(notes, strings.Join(impact.Notes, "; "))
	}

	return strings.Join(notes, " | ")
}

// truncateString truncates a string to maxLen characters, adding ellipsis if needed.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

var slackEscapeReplacer = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	"`", "\\`",
	"*", "\\*",
	"[", "\\[",
	"]", "\\]",
	"|", "\\|",
)

// escapeMarkdown escapes characters that would break Slack mrkdwn formatting while
// keeping common symbols (like parentheses and underscores) readable.
func escapeMarkdown(s string) string {
	return slackEscapeReplacer.Replace(s)
}

// joinStrings is a template function that joins string slices.
func joinStrings(elems []string, sep string) string {
	return strings.Join(elems, sep)
}
