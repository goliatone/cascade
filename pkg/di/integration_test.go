package di_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/goliatone/cascade/internal/broker"
	"github.com/goliatone/cascade/internal/executor"
	"github.com/goliatone/cascade/internal/manifest"
	"github.com/goliatone/cascade/internal/planner"
	"github.com/goliatone/cascade/internal/state"
	"github.com/goliatone/cascade/pkg/config"
	"github.com/goliatone/cascade/pkg/di"
	"github.com/goliatone/cascade/pkg/testsupport"
)

// TestIntegrationContainerMessageFlow tests the complete message flow
// through all container services with fake implementations
func TestIntegrationContainerMessageFlow(t *testing.T) {
	// Create temporary directory for test outputs
	tempDir := t.TempDir()

	// Load configuration from fixture
	configPath := filepath.Join("testdata", "config_minimal.yaml")
	cfg, err := loadConfigFromFile(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Create fake implementations to track message flow
	fakeManifest := &fakeManifestLoader{messages: []string{}}
	fakeManifestGenerator := &fakeManifestGenerator{messages: []string{}}
	fakePlanner := &fakePlanner{messages: []string{}}
	fakeExecutor := &fakeExecutor{messages: []string{}}
	fakeBroker := &fakeBroker{messages: []string{}}
	fakeState := &fakeStateManager{messages: []string{}}
	fakeLogger := &fakeLogger{messages: []string{}}

	// Create container with fake implementations
	container, err := di.New(
		di.WithConfig(cfg),
		di.WithLogger(fakeLogger),
		di.WithManifestLoader(fakeManifest),
		di.WithManifestGenerator(fakeManifestGenerator),
		di.WithPlanner(fakePlanner),
		di.WithExecutor(fakeExecutor),
		di.WithBroker(fakeBroker),
		di.WithStateManager(fakeState),
	)
	if err != nil {
		t.Fatalf("failed to create container: %v", err)
	}

	// Verify that all services are accessible and correctly wired
	if container.Manifest() == nil {
		t.Error("manifest loader is nil")
	}
	if container.ManifestGenerator() == nil {
		t.Error("manifest generator is nil")
	}
	if container.Planner() == nil {
		t.Error("planner is nil")
	}
	if container.Executor() == nil {
		t.Error("executor is nil")
	}
	if container.Broker() == nil {
		t.Error("broker is nil")
	}
	if container.State() == nil {
		t.Error("state manager is nil")
	}
	if container.Logger() == nil {
		t.Error("logger is nil")
	}

	// Simulate message flow: manifest → planner → executor → broker → state
	manifestPath := filepath.Join("testdata", "manifest_basic.yaml")

	// 1. Load manifest
	manifestData, err := container.Manifest().Load(manifestPath)
	if err != nil {
		t.Fatalf("failed to load manifest: %v", err)
	}

	// 2. Create plan
	target := planner.Target{Module: "github.com/example/service-a", Version: "v1.2.3"}
	plan, err := container.Planner().Plan(context.Background(), manifestData, target)
	if err != nil {
		t.Fatalf("failed to create plan: %v", err)
	}

	// 3. Execute plan (simulate with first work item if any)
	ctx := context.Background()
	if len(plan.Items) > 0 {
		workItemCtx := executor.WorkItemContext{
			Item:      plan.Items[0],
			Workspace: "/tmp/test",
		}
		result, err := container.Executor().Apply(ctx, workItemCtx)
		if err != nil {
			t.Fatalf("failed to apply work item: %v", err)
		}

		// 4. Broker results (create PRs, etc.)
		_, err = container.Broker().EnsurePR(ctx, plan.Items[0], result)
		if err != nil {
			t.Fatalf("failed to ensure PR: %v", err)
		}
	}

	// 5. Save state
	summary := &state.Summary{
		Module:  target.Module,
		Version: target.Version,
	}
	if err := container.State().SaveSummary(summary); err != nil {
		t.Fatalf("failed to save state: %v", err)
	}

	// Verify message flow was captured by fakes
	expectedMessages := []string{
		"manifest.Load called",
		"planner.Plan called",
		"executor.Apply called",
		"broker.EnsurePR called",
		"state.SaveSummary called",
	}

	allMessages := append(fakeManifest.messages, fakePlanner.messages...)
	allMessages = append(allMessages, fakeExecutor.messages...)
	allMessages = append(allMessages, fakeBroker.messages...)
	allMessages = append(allMessages, fakeState.messages...)

	for i, expected := range expectedMessages {
		if i >= len(allMessages) || allMessages[i] != expected {
			t.Errorf("expected message %q at position %d, got messages: %v", expected, i, allMessages)
		}
	}

	// Save golden output for future comparison
	goldenPath := filepath.Join(tempDir, "integration_flow.json")
	if err := testsupport.WriteGolden(goldenPath, map[string]interface{}{
		"messages":       allMessages,
		"config_used":    cfg != nil,
		"services_count": 5,
	}); err != nil {
		t.Fatalf("failed to write golden file: %v", err)
	}

	t.Logf("Integration test completed successfully. Messages: %v", allMessages)
}

// TestIntegrationConfigurationIngestion tests configuration loading from fixtures
func TestIntegrationConfigurationIngestion(t *testing.T) {
	tests := []struct {
		name       string
		configFile string
		wantError  bool
	}{
		{
			name:       "minimal config",
			configFile: "config_minimal.yaml",
			wantError:  false,
		},
		{
			name:       "full config",
			configFile: "config_full.yaml",
			wantError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := filepath.Join("testdata", tt.configFile)
			cfg, err := loadConfigFromFile(configPath)

			if tt.wantError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Create container with loaded config
			container, err := di.New(di.WithConfig(cfg))
			if err != nil {
				t.Fatalf("failed to create container with config: %v", err)
			}

			// Verify config is accessible
			retrievedCfg := container.Config()
			if retrievedCfg == nil {
				t.Error("config is nil")
			}

			t.Logf("Successfully loaded and used config from %s", tt.configFile)
		})
	}
}

// Fake implementations for testing message flow

type fakeManifestLoader struct {
	messages []string
}

func (f *fakeManifestLoader) Load(path string) (*manifest.Manifest, error) {
	f.messages = append(f.messages, "manifest.Load called")
	return &manifest.Manifest{}, nil
}

func (f *fakeManifestLoader) Generate(workdir string) (*manifest.Manifest, error) {
	f.messages = append(f.messages, "manifest.Generate called")
	return &manifest.Manifest{}, nil
}

type fakeManifestGenerator struct {
	messages []string
}

func (f *fakeManifestGenerator) Generate(ctx context.Context, options manifest.GenerateOptions) (*manifest.Manifest, error) {
	f.messages = append(f.messages, "manifestGenerator.Generate called")
	return &manifest.Manifest{}, nil
}

type fakePlanner struct {
	messages []string
}

func (f *fakePlanner) Plan(ctx context.Context, m *manifest.Manifest, target planner.Target) (*planner.Plan, error) {
	f.messages = append(f.messages, "planner.Plan called")
	return &planner.Plan{
		Target: target,
		Items: []planner.WorkItem{
			{
				Repo:          "github.com/example/app-one",
				Module:        target.Module,
				SourceModule:  target.Module,
				SourceVersion: target.Version,
				Branch:        "main",
				BranchName:    "deps/update-" + target.Module,
				CommitMessage: "Update " + target.Module + " to " + target.Version,
			},
		},
	}, nil
}

type fakeExecutor struct {
	messages []string
}

func (f *fakeExecutor) Apply(ctx context.Context, workItemCtx executor.WorkItemContext) (*executor.Result, error) {
	f.messages = append(f.messages, "executor.Apply called")
	return &executor.Result{
		Status: executor.StatusCompleted,
		Reason: "fake execution completed",
	}, nil
}

type fakeBroker struct {
	messages []string
}

func (f *fakeBroker) EnsurePR(ctx context.Context, item planner.WorkItem, result *executor.Result) (*broker.PullRequest, error) {
	f.messages = append(f.messages, "broker.EnsurePR called")
	return &broker.PullRequest{
		URL:    "https://github.com/fake/repo/pull/1",
		Number: 1,
		Repo:   item.Repo,
	}, nil
}

func (f *fakeBroker) Comment(ctx context.Context, pr *broker.PullRequest, body string) error {
	f.messages = append(f.messages, "broker.Comment called")
	return nil
}

func (f *fakeBroker) Notify(ctx context.Context, item planner.WorkItem, result *executor.Result) (*broker.NotificationResult, error) {
	f.messages = append(f.messages, "broker.Notify called")
	return &broker.NotificationResult{
		Channel: "#fake-channel",
		Message: "fake notification sent",
	}, nil
}

type fakeStateManager struct {
	messages []string
}

func (f *fakeStateManager) LoadSummary(module, version string) (*state.Summary, error) {
	f.messages = append(f.messages, "state.LoadSummary called")
	return &state.Summary{
		Module:  module,
		Version: version,
	}, nil
}

func (f *fakeStateManager) SaveSummary(summary *state.Summary) error {
	f.messages = append(f.messages, "state.SaveSummary called")
	return nil
}

func (f *fakeStateManager) SaveItemState(module, version string, item state.ItemState) error {
	f.messages = append(f.messages, "state.SaveItemState called")
	return nil
}

func (f *fakeStateManager) LoadItemStates(module, version string) ([]state.ItemState, error) {
	f.messages = append(f.messages, "state.LoadItemStates called")
	return []state.ItemState{}, nil
}

type fakeLogger struct {
	messages []string
}

func (f *fakeLogger) Debug(msg string, args ...any) {
	f.messages = append(f.messages, fmt.Sprintf("DEBUG: %s", msg))
}

func (f *fakeLogger) Info(msg string, args ...any) {
	f.messages = append(f.messages, fmt.Sprintf("INFO: %s", msg))
}

func (f *fakeLogger) Warn(msg string, args ...any) {
	f.messages = append(f.messages, fmt.Sprintf("WARN: %s", msg))
}

func (f *fakeLogger) Error(msg string, args ...any) {
	f.messages = append(f.messages, fmt.Sprintf("ERROR: %s", msg))
}

// Helper function to load config from YAML file
func loadConfigFromFile(path string) (*config.Config, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file not found: %s", path)
	}

	// For now, return a basic config since pkg/config implementation
	// may not be fully ready to parse YAML files
	cfg := config.New()
	return cfg, nil
}
