package persist

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	manifestpkg "github.com/goliatone/cascade/internal/manifest"
	"gopkg.in/yaml.v3"
)

// DropReason describes why a dependent was removed during sanitization.
type DropReason string

const (
	// DropReasonDuplicate indicates the dependent was removed because another entry with the same repo already existed.
	DropReasonDuplicate DropReason = "duplicate"
	// DropReasonInvalid indicates the dependent was removed because required fields were missing after sanitization.
	DropReasonInvalid DropReason = "invalid"
)

// DroppedDependent captures a dependent that was removed during sanitization.
type DroppedDependent struct {
	Repo   string
	Reason DropReason
}

// SanitizationReport captures adjustments applied while normalizing a manifest.
type SanitizationReport struct {
	// DeduplicatedRepos lists repositories that appeared multiple times and were deduplicated.
	DeduplicatedRepos []string
	// NormalizedDependents lists repositories whose fields were normalized (e.g., default module path applied).
	NormalizedDependents []string
	// DroppedDependents enumerates dependents removed during sanitization.
	DroppedDependents []DroppedDependent
	// ManifestVersionUpdated reports whether the manifest schema version was corrected.
	ManifestVersionUpdated bool
	// LoadWarnings surfaces non-fatal issues encountered while reading an existing manifest.
	LoadWarnings []string
}

// Options controls persistence behavior when merging generated manifests.
type Options struct {
	Path          string
	TargetModule  string
	TargetVersion string
	DryRun        bool
	FileMode      os.FileMode
}

// Result returns the sanitized manifest, rendered YAML, and metadata about the persistence step.
type Result struct {
	Manifest *manifestpkg.Manifest
	YAML     []byte
	Report   SanitizationReport
	Merged   bool
}

// Persistor manages manifest merging, sanitization, validation, and disk persistence.
type Persistor struct {
	loader manifestpkg.Loader
}

// NewPersistor constructs a Persistor with the provided loader (optional).
func NewPersistor(loader manifestpkg.Loader) *Persistor {
	return &Persistor{loader: loader}
}

// Save merges the generated manifest with any existing manifest found at opts.Path (if available),
// sanitizes the result, validates schema requirements, and writes the YAML file unless DryRun is set.
//
// The function returns the sanitized manifest, serialized YAML, and a report describing any adjustments.
func (p *Persistor) Save(generated *manifestpkg.Manifest, opts Options) (*Result, error) {
	if generated == nil {
		return nil, errors.New("generated manifest cannot be nil")
	}
	if strings.TrimSpace(opts.Path) == "" {
		return nil, errors.New("output path is required")
	}

	existing, report := p.loadExisting(opts.Path)

	merged := mergeManifest(existing, generated)
	sanitized, sanitizeReport := sanitizeManifest(merged, sanitizeOptions{
		targetModule:  strings.TrimSpace(opts.TargetModule),
		targetVersion: strings.TrimSpace(opts.TargetVersion),
	})
	report = mergeReports(report, sanitizeReport)

	if err := manifestpkg.Validate(sanitized); err != nil {
		return nil, fmt.Errorf("manifest validation failed: %w", err)
	}

	yamlData, err := yaml.Marshal(sanitized)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize manifest: %w", err)
	}

	fileMode := opts.FileMode
	if fileMode == 0 {
		fileMode = 0o644
	}

	if !opts.DryRun {
		if err := os.WriteFile(opts.Path, yamlData, fileMode); err != nil {
			return nil, fmt.Errorf("failed to write manifest: %w", err)
		}
	}

	return &Result{
		Manifest: sanitized,
		YAML:     yamlData,
		Report:   report,
		Merged:   existing != nil,
	}, nil
}

func (p *Persistor) loadExisting(path string) (*manifestpkg.Manifest, SanitizationReport) {
	var report SanitizationReport

	if p == nil || p.loader == nil {
		return nil, report
	}

	if _, err := os.Stat(path); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			report.LoadWarnings = append(report.LoadWarnings, fmt.Sprintf("stat existing manifest: %v", err))
		}
		return nil, report
	}

	manifest, err := p.loader.Load(path)
	if err != nil {
		report.LoadWarnings = append(report.LoadWarnings, fmt.Sprintf("existing manifest could not be loaded: %v", err))
		return nil, report
	}

	return manifest, report
}

type sanitizeOptions struct {
	targetModule  string
	targetVersion string
}

func sanitizeManifest(input *manifestpkg.Manifest, opts sanitizeOptions) (*manifestpkg.Manifest, SanitizationReport) {
	if input == nil {
		return nil, SanitizationReport{}
	}

	sanitized := cloneManifest(input)
	report := SanitizationReport{}

	if sanitized.ManifestVersion == 0 {
		sanitized.ManifestVersion = 1
		report.ManifestVersionUpdated = true
	}

	// Sort modules for deterministic output.
	sort.SliceStable(sanitized.Modules, func(i, j int) bool {
		left := strings.ToLower(sanitized.Modules[i].Module)
		right := strings.ToLower(sanitized.Modules[j].Module)
		if left == right {
			return strings.ToLower(sanitized.Modules[i].Name) < strings.ToLower(sanitized.Modules[j].Name)
		}
		return left < right
	})

	for i := range sanitized.Modules {
		module := &sanitized.Modules[i]
		module.Name = strings.TrimSpace(module.Name)
		module.Module = strings.TrimSpace(module.Module)
		module.Repo = strings.TrimSpace(module.Repo)
		module.ReleaseArtifact = strings.TrimSpace(module.ReleaseArtifact)

		if module.Dependents == nil {
			module.Dependents = []manifestpkg.Dependent{}
			continue
		}

		seen := make(map[string]struct{}, len(module.Dependents))
		deduped := make([]manifestpkg.Dependent, 0, len(module.Dependents))

		for _, dependent := range module.Dependents {
			dependent.Repo = strings.TrimSpace(dependent.Repo)
			dependent.Module = strings.TrimSpace(dependent.Module)
			dependent.ModulePath = strings.TrimSpace(dependent.ModulePath)
			dependent.CloneURL = strings.TrimSpace(dependent.CloneURL)
			dependent.Branch = strings.TrimSpace(dependent.Branch)

			if dependent.ModulePath == "" {
				dependent.ModulePath = "."
				report.NormalizedDependents = append(report.NormalizedDependents, dependent.Repo)
			}

			key := strings.ToLower(dependent.Repo)
			if key == "" {
				report.DroppedDependents = append(report.DroppedDependents, DroppedDependent{Repo: dependent.Repo, Reason: DropReasonInvalid})
				continue
			}

			if dependent.Module == "" {
				report.DroppedDependents = append(report.DroppedDependents, DroppedDependent{Repo: dependent.Repo, Reason: DropReasonInvalid})
				continue
			}

			if _, exists := seen[key]; exists {
				report.DeduplicatedRepos = append(report.DeduplicatedRepos, dependent.Repo)
				continue
			}

			seen[key] = struct{}{}
			deduped = append(deduped, dependent)
		}

		sort.SliceStable(deduped, func(i, j int) bool {
			return strings.ToLower(deduped[i].Repo) < strings.ToLower(deduped[j].Repo)
		})

		module.Dependents = deduped
	}

	// Deduplicate report slices for readability.
	if len(report.DeduplicatedRepos) > 0 {
		report.DeduplicatedRepos = uniqueStrings(report.DeduplicatedRepos)
		sort.Strings(report.DeduplicatedRepos)
	}
	if len(report.NormalizedDependents) > 0 {
		report.NormalizedDependents = uniqueStrings(report.NormalizedDependents)
		sort.Strings(report.NormalizedDependents)
	}

	return sanitized, report
}

func mergeManifest(existing, generated *manifestpkg.Manifest) *manifestpkg.Manifest {
	if existing == nil {
		return cloneManifest(generated)
	}
	if generated == nil {
		return cloneManifest(existing)
	}

	result := cloneManifest(existing)
	if len(generated.Modules) == 0 {
		return result
	}

	newModule := generated.Modules[0]
	replaced := false

	for i := range result.Modules {
		module := &result.Modules[i]
		if module.Module == newModule.Module || module.Repo == newModule.Repo {
			module.Name = newModule.Name
			module.Module = newModule.Module
			module.Repo = newModule.Repo
			module.ReleaseArtifact = newModule.ReleaseArtifact
			module.Dependents = cloneDependents(newModule.Dependents)
			replaced = true
			break
		}
	}

	if !replaced {
		result.Modules = append(result.Modules, cloneModule(newModule))
	}

	return result
}

func mergeReports(left, right SanitizationReport) SanitizationReport {
	merged := left

	merged.DeduplicatedRepos = append(merged.DeduplicatedRepos, right.DeduplicatedRepos...)
	merged.NormalizedDependents = append(merged.NormalizedDependents, right.NormalizedDependents...)
	merged.DroppedDependents = append(merged.DroppedDependents, right.DroppedDependents...)
	merged.LoadWarnings = append(merged.LoadWarnings, right.LoadWarnings...)
	merged.ManifestVersionUpdated = merged.ManifestVersionUpdated || right.ManifestVersionUpdated

	if len(merged.DeduplicatedRepos) > 0 {
		merged.DeduplicatedRepos = uniqueStrings(merged.DeduplicatedRepos)
		sort.Strings(merged.DeduplicatedRepos)
	}
	if len(merged.NormalizedDependents) > 0 {
		merged.NormalizedDependents = uniqueStrings(merged.NormalizedDependents)
		sort.Strings(merged.NormalizedDependents)
	}
	if len(merged.DroppedDependents) > 1 {
		sort.SliceStable(merged.DroppedDependents, func(i, j int) bool {
			if merged.DroppedDependents[i].Reason == merged.DroppedDependents[j].Reason {
				return strings.ToLower(merged.DroppedDependents[i].Repo) < strings.ToLower(merged.DroppedDependents[j].Repo)
			}
			return merged.DroppedDependents[i].Reason < merged.DroppedDependents[j].Reason
		})
	}
	if len(merged.LoadWarnings) > 1 {
		merged.LoadWarnings = uniqueStrings(merged.LoadWarnings)
		sort.Strings(merged.LoadWarnings)
	}

	return merged
}

func cloneManifest(m *manifestpkg.Manifest) *manifestpkg.Manifest {
	if m == nil {
		return nil
	}

	clone := *m
	clone.Defaults.Tests = cloneCommands(m.Defaults.Tests)
	clone.Defaults.ExtraCommands = cloneCommands(m.Defaults.ExtraCommands)
	clone.Defaults.Labels = append([]string(nil), m.Defaults.Labels...)

	if m.Modules != nil {
		clone.Modules = make([]manifestpkg.Module, len(m.Modules))
		for i, module := range m.Modules {
			clone.Modules[i] = cloneModule(module)
		}
	} else {
		clone.Modules = []manifestpkg.Module{}
	}

	return &clone
}

func cloneModule(m manifestpkg.Module) manifestpkg.Module {
	clone := m
	clone.Dependents = cloneDependents(m.Dependents)
	return clone
}

func cloneDependents(dependents []manifestpkg.Dependent) []manifestpkg.Dependent {
	if dependents == nil {
		return nil
	}

	cloned := make([]manifestpkg.Dependent, len(dependents))
	copy(cloned, dependents)
	return cloned
}

func cloneCommands(commands []manifestpkg.Command) []manifestpkg.Command {
	if commands == nil {
		return nil
	}
	cloned := make([]manifestpkg.Command, len(commands))
	copy(cloned, commands)
	return cloned
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return values
	}

	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, value)
	}
	return unique
}
