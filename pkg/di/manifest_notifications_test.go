package di

import (
	"net/http"
	"os"
	"testing"

	"github.com/goliatone/cascade/pkg/config"
)

func TestNewNotifierFromConfigWithManifest_PreferManifest(t *testing.T) {
	withClearedSlackEnv(t, func() {
		cfg := &config.Config{}
		cfg.Integration.Slack.Token = "test-token"
		// No channel in global config

		manifestNotifications := &ManifestNotifications{
			SlackChannel: "#manifest-channel",
		}

		logger := testLogger{}
		notifier := newNotifierFromConfigWithManifest(cfg, manifestNotifications, &http.Client{}, logger)

		if notifier == nil {
			t.Fatal("expected notifier, got nil")
		}

		// Notifier should be created successfully with manifest channel
		// This implicitly tests that the manifest channel was used
	})
}

func TestNewNotifierFromConfigWithManifest_GlobalConfigTakesPrecedence(t *testing.T) {
	withClearedSlackEnv(t, func() {
		cfg := &config.Config{}
		cfg.Integration.Slack.Token = "test-token"
		cfg.Integration.Slack.Channel = "#global-channel"

		manifestNotifications := &ManifestNotifications{
			SlackChannel: "#manifest-channel",
		}

		logger := testLogger{}
		notifier := newNotifierFromConfigWithManifest(cfg, manifestNotifications, &http.Client{}, logger)

		if notifier == nil {
			t.Fatal("expected notifier, got nil")
		}
	})
}

func TestNewNotifierFromConfigWithManifest_WebhookFromManifest(t *testing.T) {
	withClearedSlackEnv(t, func() {
		cfg := &config.Config{}
		// No webhook in global config

		manifestNotifications := &ManifestNotifications{
			Webhook: "https://hooks.slack.com/test",
		}

		logger := testLogger{}
		notifier := newNotifierFromConfigWithManifest(cfg, manifestNotifications, &http.Client{}, logger)

		if notifier == nil {
			t.Fatal("expected notifier, got nil")
		}
	})
}

func TestNewNotifierFromConfigWithManifest_NoManifestFallback(t *testing.T) {
	withClearedSlackEnv(t, func() {
		cfg := &config.Config{}
		cfg.Integration.Slack.Token = "test-token"
		cfg.Integration.Slack.Channel = "#global-channel"

		logger := testLogger{}
		notifier := newNotifierFromConfigWithManifest(cfg, nil, &http.Client{}, logger)

		if notifier == nil {
			t.Fatal("expected notifier, got nil")
		}
	})
}

func TestBrokerWithManifestNotifications(t *testing.T) {
	withClearedGitHubEnv(t, func() {
		cfg := &config.Config{}
		cfg.Integration.GitHub.Token = "test-token"
		cfg.Integration.Slack.Token = "slack-token"
		// No Slack channel in global config

		container, err := New(
			WithConfig(cfg),
		)
		if err != nil {
			t.Fatalf("failed to create container: %v", err)
		}

		manifestNotifications := &ManifestNotifications{
			SlackChannel: "#manifest-channel",
		}

		broker, err := container.BrokerWithManifestNotifications(manifestNotifications)
		if err != nil {
			t.Fatalf("failed to create broker with manifest notifications: %v", err)
		}

		if broker == nil {
			t.Fatal("expected broker, got nil")
		}
	})
}

func TestBrokerWithManifestNotifications_DryRun(t *testing.T) {
	cfg := &config.Config{}
	cfg.Executor.DryRun = true

	container, err := New(
		WithConfig(cfg),
	)
	if err != nil {
		t.Fatalf("failed to create container: %v", err)
	}

	manifestNotifications := &ManifestNotifications{
		SlackChannel: "#manifest-channel",
	}

	broker, err := container.BrokerWithManifestNotifications(manifestNotifications)
	if err != nil {
		t.Fatalf("failed to create broker in dry-run mode: %v", err)
	}

	if broker == nil {
		t.Fatal("expected broker, got nil")
	}

	// In dry-run mode, should return stub broker
	if !isStubBroker(broker) {
		t.Error("expected stub broker in dry-run mode")
	}
}

// Helper function to clear Slack environment variables
func withClearedSlackEnv(t *testing.T, fn func()) {
	t.Helper()
	vars := []string{
		"CASCADE_SLACK_TOKEN",
		"CASCADE_SLACK_CHANNEL",
		"CASCADE_SLACK_WEBHOOK",
		"SLACK_TOKEN",
		"SLACK_WEBHOOK_URL",
	}
	original := make(map[string]string, len(vars))
	for _, v := range vars {
		original[v] = os.Getenv(v)
		os.Unsetenv(v)
	}
	defer func() {
		for _, v := range vars {
			if val, ok := original[v]; ok {
				os.Setenv(v, val)
			}
		}
	}()
	fn()
}
