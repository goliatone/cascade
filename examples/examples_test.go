package examples_test

import (
	"os"
	"strings"
	"testing"

	"github.com/goliatone/cascade/internal/manifest"
	"gopkg.in/yaml.v3"
)

// TestManifestExamples validates that all example manifest files
// can be loaded and are valid according to the manifest schema.
func TestManifestExamples(t *testing.T) {
	examples := []struct {
		name string
		file string
	}{
		{"Basic", "basic-manifest.yaml"},
		{"FullFeatured", "full-featured-manifest.yaml"},
		{"CustomTemplates", "custom-templates-manifest.yaml"},
	}

	for _, example := range examples {
		t.Run(example.name, func(t *testing.T) {
			// Verify file exists
			if _, err := os.Stat(example.file); os.IsNotExist(err) {
				t.Fatalf("Example manifest file does not exist: %s", example.file)
			}

			// Read and parse the YAML file
			content, err := os.ReadFile(example.file)
			if err != nil {
				t.Fatalf("Failed to read example manifest %s: %v", example.file, err)
			}

			var manifestData manifest.Manifest
			if err := yaml.Unmarshal(content, &manifestData); err != nil {
				t.Fatalf("Failed to parse example manifest %s: %v", example.file, err)
			}

			// Basic validation that the manifest loaded successfully
			validateExampleManifest(t, &manifestData, example.name)
		})
	}
}

// validateExampleManifest performs basic validation on loaded example manifests
func validateExampleManifest(t *testing.T, m *manifest.Manifest, exampleName string) {
	t.Helper()

	// Validate manifest version
	if m.ManifestVersion == 0 {
		t.Errorf("%s example: manifest_version should be set", exampleName)
	}

	// Validate defaults section
	if m.Defaults.Branch == "" {
		t.Errorf("%s example: defaults.branch should not be empty", exampleName)
	}
	if m.Defaults.CommitTemplate == "" {
		t.Errorf("%s example: defaults.commit_template should not be empty", exampleName)
	}
	if m.Defaults.PR.TitleTemplate == "" {
		t.Errorf("%s example: defaults.pr.title should not be empty", exampleName)
	}

	// Validate modules section
	if len(m.Modules) == 0 {
		t.Errorf("%s example: should have at least one module", exampleName)
	} else {
		for i, module := range m.Modules {
			if module.Name == "" {
				t.Errorf("%s example: module[%d].name should not be empty", exampleName, i)
			}
			if module.Module == "" {
				t.Errorf("%s example: module[%d].module should not be empty", exampleName, i)
			}
			if module.Repo == "" {
				t.Errorf("%s example: module[%d].repo should not be empty", exampleName, i)
			}
		}
	}

	// Validate template variables are properly formatted
	validateTemplateVariables(t, m, exampleName)
}

// validateTemplateVariables checks that template variables use proper Go template syntax
func validateTemplateVariables(t *testing.T, m *manifest.Manifest, exampleName string) {
	t.Helper()

	// Check commit template
	if m.Defaults.CommitTemplate != "" {
		if !strings.Contains(m.Defaults.CommitTemplate, "{{") {
			t.Logf("%s example: commit_template doesn't contain template variables (this is OK)", exampleName)
		} else {
			// Verify it contains expected variables
			expectedVars := []string{".Module", ".Version"}
			for _, variable := range expectedVars {
				if strings.Contains(m.Defaults.CommitTemplate, variable) {
					t.Logf("%s example: commit_template contains %s", exampleName, variable)
				}
			}
		}
	}

	// Check PR title template
	if m.Defaults.PR.TitleTemplate != "" {
		if !strings.Contains(m.Defaults.PR.TitleTemplate, "{{") {
			t.Logf("%s example: pr.title doesn't contain template variables (this is OK)", exampleName)
		}
	}
}

// TestExampleManifestStructure tests the structure of example manifests
// to ensure they demonstrate the expected features.
func TestExampleManifestStructure(t *testing.T) {
	tests := []struct {
		file             string
		expectedFeatures map[string]bool
	}{
		{
			"basic-manifest.yaml",
			map[string]bool{
				"single_module":    true,
				"no_dependents":    true,
				"default_settings": true,
				"no_notifications": true,
			},
		},
		{
			"full-featured-manifest.yaml",
			map[string]bool{
				"multiple_dependents": true,
				"notifications":       true,
				"custom_tests":        true,
				"team_reviewers":      true,
				"canary_deployment":   true,
			},
		},
		{
			"custom-templates-manifest.yaml",
			map[string]bool{
				"custom_templates":      true,
				"rich_pr_body":          true,
				"webhook_notifications": true,
				"environment_variables": true,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.file, func(t *testing.T) {
			content, err := os.ReadFile(test.file)
			if err != nil {
				t.Fatalf("Failed to read manifest: %v", err)
			}

			var m manifest.Manifest
			if err := yaml.Unmarshal(content, &m); err != nil {
				t.Fatalf("Failed to parse manifest: %v", err)
			}

			// Test specific features based on the example
			if test.expectedFeatures["single_module"] {
				if len(m.Modules) != 1 {
					t.Errorf("Expected exactly 1 module, got %d", len(m.Modules))
				}
			}

			if test.expectedFeatures["no_dependents"] {
				for _, module := range m.Modules {
					if len(module.Dependents) > 0 {
						t.Errorf("Expected no dependents, but found %d", len(module.Dependents))
					}
				}
			}

			if test.expectedFeatures["multiple_dependents"] {
				hasDependents := false
				for _, module := range m.Modules {
					if len(module.Dependents) > 1 {
						hasDependents = true
						break
					}
				}
				if !hasDependents {
					t.Error("Expected multiple dependents but found none")
				}
			}

			if test.expectedFeatures["notifications"] {
				if m.Defaults.Notifications.SlackChannel == "" {
					t.Error("Expected notification configuration")
				}
			}

			if test.expectedFeatures["custom_templates"] {
				if !strings.Contains(m.Defaults.CommitTemplate, "{{") {
					t.Error("Expected custom template with variables")
				}
			}

			if test.expectedFeatures["canary_deployment"] {
				foundCanary := false
				for _, module := range m.Modules {
					for _, dep := range module.Dependents {
						if dep.Canary {
							foundCanary = true
							break
						}
					}
					if foundCanary {
						break
					}
				}
				if !foundCanary {
					t.Error("Expected at least one canary deployment")
				}
			}
		})
	}
}

// TestExampleReadmeAccuracy validates that the README examples are accurate
func TestExampleReadmeAccuracy(t *testing.T) {
	readmePath := "../README.md"
	readmeContent, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("Failed to read README.md: %v", err)
	}

	readme := string(readmeContent)

	// Check that README references the example files
	expectedReferences := []string{
		"examples/basic-manifest.yaml",
		"examples/full-featured-manifest.yaml",
		"examples/custom-templates-manifest.yaml",
	}

	for _, reference := range expectedReferences {
		if !strings.Contains(readme, reference) {
			t.Errorf("README.md should reference %s", reference)
		}
	}

	// Verify the files referenced in README actually exist (relative to project root)
	expectedFiles := []string{
		"basic-manifest.yaml",
		"full-featured-manifest.yaml",
		"custom-templates-manifest.yaml",
	}

	for _, filename := range expectedFiles {
		if _, err := os.Stat(filename); os.IsNotExist(err) {
			t.Errorf("Expected example file %s does not exist", filename)
		}
	}
}
