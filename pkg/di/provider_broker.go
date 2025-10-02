package di

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/go-github/v66/github"
	"golang.org/x/oauth2"

	"github.com/goliatone/cascade/internal/broker"
	"github.com/goliatone/cascade/pkg/config"
)

// provideBroker creates a default broker implementation.
// Uses stub implementations for provider and notifier since those require configuration.
func provideBroker() broker.Broker {
	return broker.NewStub()
}

// provideBrokerWithConfig creates a broker implementation configured from config.
// Returns a real broker with GitHub provider and Slack notifier if credentials are available.
// For dry-run operations, returns a stub broker with clear warnings.
// For production operations (release/resume/revert), fails fast if GitHub credentials are missing.
func provideBrokerWithConfig(cfg *config.Config, httpClient *http.Client, logger Logger) broker.Broker {
	return provideBrokerWithConfigAndManifest(cfg, nil, httpClient, logger)
}

// provideBrokerWithConfigAndManifest creates a broker with optional manifest notification settings.
// If manifestNotifications is provided, it will be used as a fallback for notification configuration
// when global config doesn't specify notification targets.
func provideBrokerWithConfigAndManifest(cfg *config.Config, manifestNotifications *ManifestNotifications, httpClient *http.Client, logger Logger) broker.Broker {
	if cfg == nil {
		logger.Warn("No configuration provided, using stub broker")
		return broker.NewStub()
	}

	if cfg.Executor.DryRun {
		logger.Info("Dry-run mode enabled, using stub broker")
		return broker.NewStub()
	}

	provider, err := newGitHubProviderFromConfig(cfg, httpClient, logger)
	if err != nil {
		logger.Error("Failed to initialize GitHub provider", "error", err)
		return broker.NewStub()
	}

	notifier := newNotifierFromConfigWithManifest(cfg, manifestNotifications, httpClient, logger)

	brokerCfg := broker.DefaultConfig()
	brokerCfg.DryRun = cfg.Executor.DryRun

	return broker.New(provider, notifier, brokerCfg, logger)
}

// provideBrokerForProduction creates a broker implementation for production commands.
// Unlike provideBrokerWithConfig, this function returns an error if GitHub credentials
// are missing and dry-run is not enabled, preventing production commands from running
// with a stub broker.
func provideBrokerForProduction(cfg *config.Config, httpClient *http.Client, logger Logger) (broker.Broker, error) {
	return provideBrokerForProductionWithManifest(cfg, nil, httpClient, logger)
}

// provideBrokerForProductionWithManifest creates a production broker with optional manifest notification settings.
// If manifestNotifications is provided, it will be used as a fallback for notification configuration
// when global config doesn't specify notification targets.
func provideBrokerForProductionWithManifest(cfg *config.Config, manifestNotifications *ManifestNotifications, httpClient *http.Client, logger Logger) (broker.Broker, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required for production broker")
	}

	if cfg.Executor.DryRun {
		logger.Info("Dry-run mode enabled, using stub broker")
		return broker.NewStub(), nil
	}

	provider, err := newGitHubProviderFromConfig(cfg, httpClient, logger)
	if err != nil {
		return nil, fmt.Errorf("production commands require GitHub credentials: %w\n\nTo fix this issue:\n  1. Set CASCADE_GITHUB_TOKEN environment variable, or\n  2. Configure integration.github.token in your config file, or\n  3. Use --dry-run flag to test without GitHub integration", err)
	}

	notifier := newNotifierFromConfigWithManifest(cfg, manifestNotifications, httpClient, logger)

	brokerCfg := broker.DefaultConfig()
	brokerCfg.DryRun = cfg.Executor.DryRun

	return broker.New(provider, notifier, brokerCfg, logger), nil
}

func newGitHubProviderFromConfig(cfg *config.Config, baseHTTP *http.Client, logger Logger) (broker.Provider, error) {
	token := strings.TrimSpace(cfg.Integration.GitHub.Token)
	if token == "" {
		if envToken, err := broker.LoadGitHubToken(); err == nil && strings.TrimSpace(envToken) != "" {
			token = strings.TrimSpace(envToken)
			logger.Debug("Using GitHub token from environment variables")
		}
	}

	if token == "" {
		return nil, fmt.Errorf("github token not configured; set integration.github.token or CASCADE_GITHUB_TOKEN")
	}

	oauthClient, err := newGitHubHTTPClient(token, baseHTTP)
	if err != nil {
		return nil, err
	}

	endpoint := strings.TrimSpace(cfg.Integration.GitHub.Endpoint)
	var ghClient *github.Client
	if endpoint == "" {
		ghClient = github.NewClient(oauthClient)
	} else {
		baseURL, uploadURL := normalizeEnterpriseEndpoints(endpoint)
		ghClient, err = github.NewEnterpriseClient(baseURL, uploadURL, oauthClient)
		if err != nil {
			return nil, fmt.Errorf("create github enterprise client: %w", err)
		}
		logger.Info("Configured GitHub Enterprise endpoint", "base", baseURL, "upload", uploadURL)
	}

	return broker.NewGitHubProvider(ghClient), nil
}

func newGitHubHTTPClient(token string, base *http.Client) (*http.Client, error) {
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("github token is required")
	}

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	oauthClient := oauth2.NewClient(context.Background(), ts)

	if base != nil {
		if transport, ok := oauthClient.Transport.(*oauth2.Transport); ok {
			if base.Transport != nil {
				transport.Base = base.Transport
			}
		}
		if base.Timeout > 0 {
			oauthClient.Timeout = base.Timeout
		}
		oauthClient.CheckRedirect = base.CheckRedirect
		oauthClient.Jar = base.Jar
	}

	return oauthClient, nil
}

func normalizeEnterpriseEndpoints(endpoint string) (string, string) {
	base := strings.TrimSpace(endpoint)
	if base == "" {
		return "", ""
	}
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}

	trimmed := strings.TrimSuffix(base, "/")
	if strings.HasSuffix(trimmed, "/api/v3") {
		prefix := strings.TrimSuffix(trimmed, "/api/v3")
		if !strings.HasSuffix(prefix, "/") {
			prefix += "/"
		}
		return prefix + "api/v3/", prefix + "api/uploads/"
	}

	return base, base
}

func newNotifierFromConfig(cfg *config.Config, baseClient *http.Client, logger Logger) broker.Notifier {
	return newNotifierFromConfigWithManifest(cfg, nil, baseClient, logger)
}

func newNotifierFromConfigWithManifest(cfg *config.Config, manifestNotifications *ManifestNotifications, baseClient *http.Client, logger Logger) broker.Notifier {
	notifyCfg := broker.DefaultNotificationConfig()
	var notifiers []broker.Notifier

	var githubDefaults *broker.GitHubIssueConfig
	if manifestNotifications != nil && manifestNotifications.GitHubIssues != nil {
		githubDefaults = &broker.GitHubIssueConfig{
			Enabled: manifestNotifications.GitHubIssues.Enabled,
		}
		if len(manifestNotifications.GitHubIssues.Labels) > 0 {
			githubDefaults.Labels = append([]string(nil), manifestNotifications.GitHubIssues.Labels...)
		}
	}

	// Determine Slack configuration, preferring manifest settings over global config
	slackToken := strings.TrimSpace(cfg.Integration.Slack.Token)
	slackChannel := strings.TrimSpace(cfg.Integration.Slack.Channel)

	// If manifest provides notification settings, use them as fallback for channel
	if manifestNotifications != nil && slackChannel == "" {
		slackChannel = strings.TrimSpace(manifestNotifications.SlackChannel)
		if slackChannel != "" {
			logger.Debug("Using Slack channel from manifest", "channel", slackChannel)
		}
	}

	if slackToken != "" && slackChannel != "" {
		client := cloneHTTPClient(baseClient, notifyCfg.Timeout)
		notifiers = append(notifiers, broker.NewSlackNotifier(slackToken, slackChannel, client, notifyCfg))
	} else if slackToken != "" || slackChannel != "" {
		logger.Warn("Slack integration requires both token and channel; skipping Slack notifier")
	}

	// Determine webhook configuration, preferring manifest settings over global config
	webhook := strings.TrimSpace(cfg.Integration.Slack.WebhookURL)
	if manifestNotifications != nil && webhook == "" {
		webhook = strings.TrimSpace(manifestNotifications.Webhook)
		if webhook != "" {
			logger.Debug("Using webhook URL from manifest", "webhook", webhook)
		}
	}

	if webhook != "" {
		client := cloneHTTPClient(baseClient, notifyCfg.Timeout)
		notifiers = append(notifiers, broker.NewWebhookNotifier(webhook, client, notifyCfg))
	}

	// Configure GitHub issue notifications if credentials are available
	githubToken := strings.TrimSpace(cfg.Integration.GitHub.Token)
	if githubToken == "" {
		if envToken, err := broker.LoadGitHubToken(); err == nil {
			envToken = strings.TrimSpace(envToken)
			if envToken != "" {
				githubToken = envToken
				logger.Debug("Using GitHub token from environment for issue notifications")
			}
		}
	}

	if githubToken != "" {
		oauthClient, err := newGitHubHTTPClient(githubToken, baseClient)
		if err != nil {
			logger.Error("Failed to initialize GitHub HTTP client for issue notifications", "error", err)
		} else {
			endpoint := strings.TrimSpace(cfg.Integration.GitHub.Endpoint)
			var ghClient *github.Client
			if endpoint == "" {
				ghClient = github.NewClient(oauthClient)
			} else {
				baseURL, uploadURL := normalizeEnterpriseEndpoints(endpoint)
				ghClient, err = github.NewEnterpriseClient(baseURL, uploadURL, oauthClient)
				if err != nil {
					logger.Error("Failed to create GitHub Enterprise client for issue notifications", "error", err)
				}
			}

			if err == nil && ghClient != nil {
				notifiers = append(notifiers, broker.NewGitHubIssueNotifier(ghClient.Issues, githubDefaults))
			}
		}
	} else if githubDefaults != nil && githubDefaults.Enabled {
		logger.Warn("GitHub issue notifications enabled but GitHub token not configured; skipping GitHub issue notifier")
	}

	var baseNotifier broker.Notifier
	switch len(notifiers) {
	case 0:
		logger.Info("No notification integrations configured, using no-op notifier")
		baseNotifier = broker.NewNoOpNotifier()
	case 1:
		baseNotifier = notifiers[0]
	default:
		baseNotifier = broker.NewMultiNotifier(notifiers...)
	}

	// Wrap with filtering notifier if manifest specifies on_success/on_failure flags
	if manifestNotifications != nil {
		baseNotifier = NewFilteringNotifier(baseNotifier, manifestNotifications.OnSuccess, manifestNotifications.OnFailure, logger)
	}

	return baseNotifier
}

// ManifestNotifications holds notification settings from manifest defaults.
// This allows manifest-level notification configuration to be used when
// global config doesn't specify notification targets.
type ManifestNotifications struct {
	SlackChannel string
	OnFailure    bool
	OnSuccess    bool
	Webhook      string
	GitHubIssues *ManifestGitHubIssues
}

// ManifestGitHubIssues captures default GitHub issue configuration from the manifest.
type ManifestGitHubIssues struct {
	Enabled bool
	Labels  []string
}

func cloneHTTPClient(base *http.Client, timeout time.Duration) *http.Client {
	if base == nil {
		client := &http.Client{Timeout: timeout}
		client.Transport = newHeaderRoundTripper(nil, defaultHTTPHeaders(nil))
		return client
	}
	clone := *base
	if clone.Timeout == 0 && timeout > 0 {
		clone.Timeout = timeout
	}
	if clone.Transport == nil {
		clone.Transport = newHeaderRoundTripper(nil, defaultHTTPHeaders(nil))
	}
	return &clone
}
