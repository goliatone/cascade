package broker

import "strings"

// RenderTitle renders the PR title using simple placeholder substitution.
func RenderTitle(template, module, version string) string {
	if template == "" {
		return "Update dependencies"
	}
	result := strings.ReplaceAll(template, "{{ module }}", module)
	result = strings.ReplaceAll(result, "{{ version }}", version)
	return result
}

// RenderBody renders the PR body from a template; placeholder semantics may expand later.
func RenderBody(template string, context map[string]string) string {
	result := template
	for k, v := range context {
		placeholder := "{{ " + k + " }}"
		result = strings.ReplaceAll(result, placeholder, v)
	}
	return result
}
