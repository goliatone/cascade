package di

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/google/go-github/v66/github"
	"golang.org/x/oauth2"

	"github.com/goliatone/cascade/internal/broker"
	"github.com/goliatone/cascade/internal/executor"
	"github.com/goliatone/cascade/internal/manifest"
	"github.com/goliatone/cascade/internal/planner"
	"github.com/goliatone/cascade/internal/state"
	"github.com/goliatone/cascade/pkg/config"
)

// provideManifest creates a default manifest loader implementation.
// Uses the basic file-based loader that reads YAML manifests from disk.
func provideManifest() manifest.Loader {
	return manifest.NewLoader()
}

// provideManifestGenerator creates a default manifest generator implementation.
// Uses the basic generator that creates manifest structures from options.
func provideManifestGenerator() manifest.Generator {
	return manifest.NewGenerator()
}

// provideManifestGeneratorWithConfig creates a manifest generator with configuration-driven defaults.
// Maps from pkg/config types to manifest generator config types.
func provideManifestGeneratorWithConfig(cfg *config.Config, logger Logger) manifest.Generator {
	if cfg == nil {
		logger.Warn("No configuration provided, using default manifest generator")
		return manifest.NewGenerator()
	}

	manifestConfig := &manifest.GeneratorConfig{
		DefaultWorkspace: cfg.ManifestGenerator.DefaultWorkspace,
		DefaultBranch:    cfg.ManifestGenerator.DefaultBranch,
		Tests: manifest.TestsConfig{
			Command:          cfg.ManifestGenerator.Tests.Command,
			Timeout:          cfg.ManifestGenerator.Tests.Timeout,
			WorkingDirectory: cfg.ManifestGenerator.Tests.WorkingDirectory,
		},
		Notifications: manifest.NotificationsConfig{
			Enabled:   cfg.ManifestGenerator.Notifications.Enabled,
			Channels:  cfg.ManifestGenerator.Notifications.Channels,
			OnSuccess: cfg.ManifestGenerator.Notifications.OnSuccess,
			OnFailure: cfg.ManifestGenerator.Notifications.OnFailure,
		},
		Discovery: manifest.DiscoveryConfig{
			Enabled:         cfg.ManifestGenerator.Discovery.Enabled,
			MaxDepth:        cfg.ManifestGenerator.Discovery.MaxDepth,
			IncludePatterns: cfg.ManifestGenerator.Discovery.IncludePatterns,
			ExcludePatterns: cfg.ManifestGenerator.Discovery.ExcludePatterns,
			Interactive:     cfg.ManifestGenerator.Discovery.Interactive,
		},
	}

	logger.Debug("Created manifest generator with config",
		"default_workspace", manifestConfig.DefaultWorkspace,
		"default_branch", manifestConfig.DefaultBranch,
		"test_command", manifestConfig.Tests.Command,
		"discovery_enabled", manifestConfig.Discovery.Enabled,
	)

	return manifest.NewGeneratorWithConfig(manifestConfig)
}

// providePlanner creates a default planner implementation.
// The planner computes cascade plans from manifests and targets.
func providePlanner() planner.Planner {
	return planner.New()
}

// providePlannerWithConfig creates a planner with configuration-driven dependency checking.
// When SkipUpToDate is enabled (and ForceAll is false), the planner checks if dependents
// already have the target dependency version and skips them if no update is needed.
func providePlannerWithConfig(cfg *config.Config, logger Logger) planner.Planner {
	if cfg == nil {
		logger.Warn("No configuration provided, using default planner")
		return planner.New()
	}

	opts := []planner.Option{}

	// Only enable dependency checking if SkipUpToDate is true and ForceAll is false
	if cfg.Executor.SkipUpToDate && !cfg.Executor.ForceAll {
		// Set default values if not configured
		strategy := cfg.Executor.CheckStrategy
		if strategy == "" {
			strategy = "auto"
		}

		cacheTTL := cfg.Executor.CheckCacheTTL
		if cacheTTL == 0 {
			cacheTTL = 5 * time.Minute
		}

		parallel := cfg.Executor.CheckParallel
		if parallel == 0 {
			parallel = runtime.NumCPU()
		}

		timeout := cfg.Executor.CheckTimeout
		if timeout == 0 {
			timeout = 30 * time.Second
		}

		checkOpts := planner.CheckOptions{
			Strategy:       planner.CheckStrategy(strategy),
			CacheEnabled:   true,
			CacheTTL:       cacheTTL,
			ParallelChecks: parallel,
			Timeout:        timeout,
		}

		// Create checkers based on strategy
		var checker planner.DependencyChecker
		localChecker := planner.NewDependencyChecker(logger)
		remoteChecker := planner.NewRemoteDependencyChecker(checkOpts, logger)

		switch checkOpts.Strategy {
		case planner.CheckStrategyLocal:
			if cfg.Workspace.Path == "" {
				logger.Warn("Local strategy requested but workspace path not configured, using remote")
				checker = remoteChecker
			} else {
				logger.Debug("Using local dependency checking", "workspace", cfg.Workspace.Path)
				checker = localChecker
			}

		case planner.CheckStrategyRemote:
			logger.Debug("Using remote dependency checking",
				"cache_ttl", cacheTTL,
				"parallel", parallel,
				"timeout", timeout)
			checker = remoteChecker

		case planner.CheckStrategyAuto:
			logger.Debug("Using auto dependency checking (local with remote fallback)",
				"workspace", cfg.Workspace.Path,
				"cache_ttl", cacheTTL,
				"parallel", parallel,
				"timeout", timeout)
			checker = planner.NewHybridDependencyChecker(
				localChecker,
				remoteChecker,
				checkOpts.Strategy,
				cfg.Workspace.Path,
				logger,
			)

		default:
			logger.Warn("Unknown check strategy, using auto", "strategy", strategy)
			checker = planner.NewHybridDependencyChecker(
				localChecker,
				remoteChecker,
				planner.CheckStrategyAuto,
				cfg.Workspace.Path,
				logger,
			)
		}

		// Wrap in parallel checker if concurrency > 1
		if checkOpts.ParallelChecks > 1 {
			logger.Debug("Enabling parallel dependency checking", "concurrency", checkOpts.ParallelChecks)
			checker = planner.NewParallelDependencyChecker(
				checker,
				checkOpts.ParallelChecks,
				logger,
			)
		}

		opts = append(opts,
			planner.WithDependencyChecker(checker),
			planner.WithWorkspace(cfg.Workspace.Path))
	} else if cfg.Executor.ForceAll {
		logger.Debug("ForceAll enabled, processing all dependents without version checking")
	} else {
		logger.Debug("SkipUpToDate disabled, processing all dependents")
	}

	return planner.New(opts...)
}

// provideExecutor creates a default executor implementation.
// The executor orchestrates git operations, dependency updates, and command execution.
func provideExecutor() executor.Executor {
	return executor.New()
}

// provideExecutorWithConfig creates an executor with configuration-driven timeouts and settings.
// The executor implementation itself doesn't need config changes, but this documents the intent
// for future expansion when we add timeout configuration.
func provideExecutorWithConfig(cfg *config.Config, logger Logger) executor.Executor {
	if cfg == nil {
		logger.Warn("No configuration provided, using default executor")
		return executor.New()
	}

	// The current executor implementation doesn't take configuration,
	// but we can log the intended timeout settings
	if cfg.Executor.Timeout > 0 {
		logger.Debug("Executor configured with timeout", "timeout", cfg.Executor.Timeout)
	}
	if cfg.Executor.ConcurrentLimit > 0 {
		logger.Debug("Executor configured with concurrency limit", "limit", cfg.Executor.ConcurrentLimit)
	}

	return executor.New()
}

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

	switch len(notifiers) {
	case 0:
		logger.Info("No notification integrations configured, using no-op notifier")
		return broker.NewNoOpNotifier()
	case 1:
		return notifiers[0]
	default:
		return broker.NewMultiNotifier(notifiers...)
	}
}

// ManifestNotifications holds notification settings from manifest defaults.
// This allows manifest-level notification configuration to be used when
// global config doesn't specify notification targets.
type ManifestNotifications struct {
	SlackChannel string
	Webhook      string
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

// provideState creates a default state manager implementation.
// Uses nop implementations for storage and locking, suitable for basic operation.
func provideState() state.Manager {
	return state.NewManager()
}

// provideStateWithConfig creates a state manager with filesystem storage and locking.
// Uses configuration to determine storage directory and other state settings.
// State persistence is enabled by default unless explicitly disabled by the user.
func provideStateWithConfig(cfg *config.Config, logger Logger) state.Manager {
	if cfg == nil {
		logger.Warn("No configuration provided, using nop state manager")
		return state.NewManager()
	}

	// Apply defaults for state configuration if not explicitly set by user.
	// This ensures state persistence is enabled by default as documented.
	stateDir := cfg.State.Dir
	if stateDir == "" {
		stateDir = getDefaultStateDir()
	}

	// Only disable state if user explicitly disabled it.
	// If Enabled is false but wasn't explicitly set, enable it (default behavior).
	explicitlyDisabled := cfg.State.Enabled == false && cfg.ExplicitlySetStateEnabled()
	if explicitlyDisabled {
		logger.Info("State persistence explicitly disabled, using nop state manager")
		return state.NewManager()
	}

	// Create filesystem storage
	stateStorage, err := state.NewFilesystemStorage(stateDir, logger)
	if err != nil {
		logger.Error("Failed to create filesystem storage, using nop state manager", "error", err)
		return state.NewManager()
	}

	// Create filesystem locker
	stateLocker := state.NewFilesystemLocker(stateDir, logger)

	logger.Debug("State persistence enabled", "dir", stateDir)

	return state.NewManager(
		state.WithStorage(stateStorage),
		state.WithLocker(stateLocker),
		state.WithLogger(logger),
	)
}

// getDefaultStateDir returns the default state directory following XDG Base Directory spec.
func getDefaultStateDir() string {
	// Follow XDG Base Directory specification
	if xdgState := os.Getenv("XDG_STATE_HOME"); xdgState != "" {
		return filepath.Join(xdgState, "cascade")
	}

	// Fallback to ~/.local/state/cascade
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".local", "state", "cascade")
	}

	// Last resort fallback
	return filepath.Join(os.TempDir(), "cascade-state")
}

// provideConfig creates a default configuration.
// Loads configuration from environment variables and defaults.
func provideConfig() *config.Config {
	cfg := config.New()
	// Configuration loading is handled by pkg/config
	return cfg
}

// provideConfigWithDefaults creates a configuration with defaults applied.
func provideConfigWithDefaults() (*config.Config, error) {
	return config.NewWithDefaults()
}

// provideLogger creates a default structured logger implementation.
// Uses the standard library slog for structured logging output.
func provideLogger() Logger {
	return &slogAdapter{
		logger: slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})),
	}
}

// provideLoggerWithConfig creates a logger configured from the logging config.
// Respects log level, format (text/json), verbose, and quiet settings.
func provideLoggerWithConfig(cfg *config.Config) Logger {
	if cfg == nil {
		return provideLogger()
	}

	// Determine log level from configuration
	var level slog.Level
	if cfg.Logging.Quiet {
		level = slog.LevelWarn
	} else if cfg.Logging.Verbose {
		level = slog.LevelDebug
	} else {
		switch cfg.Logging.Level {
		case "debug":
			level = slog.LevelDebug
		case "info":
			level = slog.LevelInfo
		case "warn":
			level = slog.LevelWarn
		case "error":
			level = slog.LevelError
		default:
			level = slog.LevelInfo
		}
	}

	// Create appropriate handler based on format
	var handler slog.Handler
	if cfg.Logging.Format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: level,
		})
	} else {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: level,
		})
	}

	return &slogAdapter{
		logger: slog.New(handler),
	}
}

// provideHTTPClient creates a default HTTP client implementation.
// Configured with reasonable defaults for API calls and timeouts.
func provideHTTPClient() *http.Client {
	return &http.Client{
		Transport: newHeaderRoundTripper(nil, defaultHTTPHeaders(nil)),
	}
}

// provideHTTPClientWithConfig creates an HTTP client with configuration-driven timeouts.
// Respects executor timeout settings and sets appropriate user agent.
func provideHTTPClientWithConfig(cfg *config.Config) *http.Client {
	if cfg == nil {
		return provideHTTPClient()
	}

	// Use executor timeout as base for HTTP timeout, with reasonable default
	timeout := 30 * time.Second // Default timeout
	if cfg.Executor.Timeout > 0 {
		// Use 80% of executor timeout to leave buffer for retries
		timeout = time.Duration(float64(cfg.Executor.Timeout) * 0.8)
		if timeout < 10*time.Second {
			timeout = 10 * time.Second // Minimum timeout
		}
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: newHeaderRoundTripper(nil, defaultHTTPHeaders(cfg)),
	}
}

const (
	defaultUserAgent = "cascade-cli (+https://github.com/goliatone/cascade)"
	defaultAccept    = "application/json"
)

func defaultHTTPHeaders(cfg *config.Config) http.Header {
	headers := make(http.Header)
	userAgent := buildUserAgent(cfg)
	headers.Set("User-Agent", userAgent)
	headers.Set("Accept", defaultAccept)
	return headers
}

func buildUserAgent(cfg *config.Config) string {
	userAgent := defaultUserAgent
	if cfg != nil {
		if org := strings.TrimSpace(cfg.Integration.GitHub.Organization); org != "" {
			userAgent = fmt.Sprintf("%s org/%s", defaultUserAgent, org)
		}
	}
	return fmt.Sprintf("%s go/%s %s/%s", userAgent, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}

type headerRoundTripper struct {
	base    http.RoundTripper
	headers http.Header
}

func newHeaderRoundTripper(base http.RoundTripper, headers http.Header) http.RoundTripper {
	if headers == nil {
		headers = make(http.Header)
	}
	var underlying http.RoundTripper = http.DefaultTransport
	if base != nil {
		underlying = base
	} else if transport, ok := http.DefaultTransport.(*http.Transport); ok {
		underlying = transport.Clone()
	}
	return &headerRoundTripper{base: underlying, headers: headers}
}

func (h *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if h == nil {
		return http.DefaultTransport.RoundTrip(req)
	}
	clone := req.Clone(req.Context())
	for key, values := range h.headers {
		if clone.Header.Get(key) != "" {
			continue
		}
		for _, value := range values {
			clone.Header.Add(key, value)
		}
	}
	return h.base.RoundTrip(clone)
}

// slogAdapter adapts slog.Logger to implement our Logger interface.
type slogAdapter struct {
	logger *slog.Logger
}

func (s *slogAdapter) Debug(msg string, args ...any) {
	s.logger.Debug(msg, args...)
}

func (s *slogAdapter) Info(msg string, args ...any) {
	s.logger.Info(msg, args...)
}

func (s *slogAdapter) Warn(msg string, args ...any) {
	s.logger.Warn(msg, args...)
}

func (s *slogAdapter) Error(msg string, args ...any) {
	s.logger.Error(msg, args...)
}
