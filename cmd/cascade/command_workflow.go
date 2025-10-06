package main

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/goliatone/cascade/pkg/config"
	"github.com/goliatone/cascade/pkg/util/modpath"
	"github.com/spf13/cobra"
)

const (
	defaultWorkflowOutputPath  = ".github/workflows/cascade-release.yml"
	defaultWorkflowTagPattern  = "v*.*.*"
	defaultWorkflowGoVersion   = "1.21"
	defaultWorkflowBinaryPath  = "/usr/local/bin/cascade"
	defaultWorkflowStateDir    = "${{ github.workspace }}/.cascade/state"
	defaultWorkflowWorkspace   = "${{ github.workspace }}"
	defaultWorkflowDescription = "Trigger: push tag matching v*.*.*"
	defaultGitHubTokenEnv      = "CASCADE_GITHUB_TOKEN"
	defaultSlackTokenEnv       = "CASCADE_SLACK_TOKEN"
)

type workflowGenerateRequest struct {
	TemplatePath   string
	OutputPath     string
	Module         string
	Version        string
	Force          bool
	Yes            bool
	NonInteractive bool
}

type workflowTemplateData struct {
	ModulePath         string
	RepoOwner          string
	RepoName           string
	GoVersion          string
	WorkflowFile       string
	TagPattern         string
	StateDir           string
	BinaryPath         string
	WorkspaceVar       string
	TriggerDescription string
	Secrets            workflowSecrets
}

type workflowSecrets struct {
	GitHubToken string
	SlackToken  string
}

func newWorkflowCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workflow",
		Short: "Manage CI workflows for Cascade",
		Long:  "Generate and manage CI workflow configurations that automate Cascade releases.",
	}

	cmd.AddCommand(newWorkflowGenerateCommand())
	return cmd
}

func newWorkflowGenerateCommand() *cobra.Command {
	req := workflowGenerateRequest{}

	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate a GitHub Actions workflow for Cascade releases",
		RunE: func(cmd *cobra.Command, args []string) error {
			if req.NonInteractive {
				req.Yes = true
			}
			return runWorkflowGenerate(req)
		},
	}

	cmd.Flags().StringVar(&req.TemplatePath, "template", "", "Path to custom workflow template file")
	cmd.Flags().StringVar(&req.OutputPath, "output", "", "Output workflow file (default: .github/workflows/cascade-release.yml)")
	cmd.Flags().StringVar(&req.Module, "module", "", "Module path override (defaults to detected go.mod module)")
	cmd.Flags().StringVar(&req.Version, "version", "", "Version override (reserved for future use)")
	cmd.Flags().BoolVar(&req.Force, "force", false, "Overwrite existing workflow file without prompting")
	cmd.Flags().BoolVar(&req.Yes, "yes", false, "Automatically confirm all prompts")
	cmd.Flags().BoolVar(&req.NonInteractive, "non-interactive", false, "Run in non-interactive mode (same as --yes)")

	return cmd
}

func runWorkflowGenerate(req workflowGenerateRequest) error {
	logger := container.Logger()
	cfg := container.Config()

	outputPath := strings.TrimSpace(req.OutputPath)
	if outputPath == "" {
		outputPath = defaultWorkflowOutputPath
	}

	modulePath, moduleDir, err := resolveModuleForWorkflow(req, cfg)
	if err != nil {
		return err
	}

	templateBytes, err := loadWorkflowTemplate(req.TemplatePath)
	if err != nil {
		return err
	}

	data, err := buildWorkflowTemplateData(req, cfg, modulePath, moduleDir, outputPath)
	if err != nil {
		return err
	}

	rendered, err := renderWorkflowTemplate(templateBytes, data)
	if err != nil {
		return newExecutionError("failed to render workflow template", err)
	}

	dryRun := cfg != nil && cfg.Executor.DryRun
	if dryRun {
		fmt.Println(rendered)
		if logger != nil {
			logger.Info("Workflow generation dry run", "output", outputPath)
		}
		return nil
	}

	shouldWrite, err := prepareWorkflowOutput(outputPath, req)
	if err != nil {
		return err
	}
	if !shouldWrite {
		if logger != nil {
			logger.Info("Workflow generation cancelled", "output", outputPath)
		}
		return nil
	}

	if err := os.WriteFile(outputPath, []byte(rendered), 0o644); err != nil {
		return newFileError(fmt.Sprintf("failed to write workflow to %s", outputPath), err)
	}

	fmt.Printf("Workflow written to %s\n", outputPath)
	if logger != nil {
		logger.Info("Workflow generated", "output", outputPath)
	}
	return nil
}

func prepareWorkflowOutput(path string, req workflowGenerateRequest) (bool, error) {
	if strings.TrimSpace(path) == "" {
		return false, newValidationError("output path must not be empty", nil)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, newFileError(fmt.Sprintf("failed to create workflow directory %s", dir), err)
	}

	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return true, nil
		}
		return false, newFileError(fmt.Sprintf("failed to inspect existing workflow %s", path), err)
	}

	if info.IsDir() {
		return false, newValidationError(fmt.Sprintf("workflow path %s is a directory", path), nil)
	}

	if req.Force || req.Yes {
		return true, nil
	}

	fmt.Printf("File %s already exists. Overwrite? [y/N]: ", path)
	var response string
	fmt.Scanln(&response)
	switch strings.ToLower(strings.TrimSpace(response)) {
	case "y", "yes":
		return true, nil
	default:
		fmt.Println("Workflow generation cancelled.")
		return false, nil
	}
}

func resolveModuleForWorkflow(req workflowGenerateRequest, cfg *config.Config) (string, string, error) {
	modulePath := strings.TrimSpace(req.Module)
	if modulePath == "" && cfg != nil {
		modulePath = strings.TrimSpace(cfg.Module)
	}

	autoModulePath, autoModuleDir, detectErr := detectModuleInfo()
	moduleDir := autoModuleDir
	if modulePath == "" {
		modulePath = autoModulePath
	}

	if strings.TrimSpace(modulePath) == "" {
		return "", "", newValidationError("module path could not be determined; specify --module or run inside a Go module", detectErr)
	}

	return modulePath, moduleDir, nil
}

func buildWorkflowTemplateData(req workflowGenerateRequest, cfg *config.Config, modulePath, moduleDir, outputPath string) (workflowTemplateData, error) {
	data := workflowTemplateData{
		ModulePath:         modulePath,
		GoVersion:          defaultWorkflowGoVersion,
		WorkflowFile:       outputPath,
		TagPattern:         defaultWorkflowTagPattern,
		StateDir:           defaultWorkflowStateDir,
		BinaryPath:         defaultWorkflowBinaryPath,
		WorkspaceVar:       defaultWorkflowWorkspace,
		TriggerDescription: defaultWorkflowDescription,
		Secrets: workflowSecrets{
			GitHubToken: defaultGitHubTokenEnv,
			SlackToken:  defaultSlackTokenEnv,
		},
	}

	if cfg != nil {
		if strings.TrimSpace(cfg.Module) != "" && data.ModulePath == "" {
			data.ModulePath = strings.TrimSpace(cfg.Module)
		}
	}

	owner, name := deriveRepositoryIdentity(modulePath)
	if owner == "" || name == "" {
		if discoveredOwner, discoveredName, ok := discoverRepoFromGit(moduleDir); ok {
			owner = discoveredOwner
			name = discoveredName
		}
	}
	data.RepoOwner = owner
	data.RepoName = name

	return data, nil
}

func renderWorkflowTemplate(tmplBytes []byte, data workflowTemplateData) (string, error) {
	tmpl, err := template.New("workflow").Option("missingkey=error").Parse(string(tmplBytes))
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func deriveRepositoryIdentity(modulePath string) (string, string) {
	repository := modpath.DeriveRepository(modulePath)
	parts := strings.Split(repository, "/")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", ""
}

func discoverRepoFromGit(moduleDir string) (string, string, bool) {
	dir := strings.TrimSpace(moduleDir)
	if dir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", "", false
		}
		dir = cwd
	}

	cmd := exec.Command("git", "-C", dir, "config", "--get", "remote.origin.url")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := cmd.Output()
	if err != nil {
		return "", "", false
	}
	return parseRemoteURL(string(output))
}

func parseRemoteURL(remote string) (string, string, bool) {
	trimmed := strings.TrimSpace(remote)
	if trimmed == "" {
		return "", "", false
	}
	trimmed = strings.TrimSuffix(trimmed, ".git")

	if strings.HasPrefix(trimmed, "git@") {
		if idx := strings.Index(trimmed, ":"); idx != -1 {
			trimmed = trimmed[idx+1:]
		}
	} else if strings.Contains(trimmed, "://") {
		if u, err := url.Parse(trimmed); err == nil {
			trimmed = strings.TrimPrefix(u.Path, "/")
		}
	}

	parts := strings.Split(trimmed, "/")
	if len(parts) >= 2 {
		owner := parts[len(parts)-2]
		repo := parts[len(parts)-1]
		return owner, repo, true
	}

	return "", "", false
}
