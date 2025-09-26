package di

import (
	"fmt"
	"net/http"
	"time"

	"github.com/goliatone/cascade/internal/broker"
	"github.com/goliatone/cascade/internal/executor"
	"github.com/goliatone/cascade/internal/manifest"
	"github.com/goliatone/cascade/internal/planner"
	"github.com/goliatone/cascade/internal/state"
	"github.com/goliatone/cascade/pkg/config"
)

// Logger defines the logging interface used throughout the application.
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

// Container exposes resolved dependencies for the CLI orchestration layer.
// All methods return interfaces to prevent leaking concrete implementations.
type Container interface {
	// Core service accessors
	Manifest() manifest.Loader
	ManifestGenerator() manifest.Generator
	Planner() planner.Planner
	Executor() executor.Executor
	Broker() broker.Broker
	State() state.Manager

	// Configuration and infrastructure
	Config() *config.Config
	Logger() Logger
	HTTPClient() *http.Client

	// Resource management
	Close() error
}

// Option customises container construction using the functional options pattern.
// Options allow overriding default dependencies for testing and customization.
type Option func(*builder) error

// New creates a container with default wiring and applies the provided options.
// It returns an error if required dependencies are missing or if any option fails.
func New(opts ...Option) (Container, error) {
	b := &builder{}

	// Apply options
	for _, opt := range opts {
		if err := opt(b); err != nil {
			return nil, fmt.Errorf("di: failed to apply option: %w", err)
		}
	}

	return b.build()
}

// builder holds the dependencies being assembled into a container.
// It uses the builder pattern with functional options for flexible construction.
type builder struct {
	// Configuration
	cfg *config.Config

	// Build options
	requireProductionCredentials bool
	enableInstrumentation        bool

	// Infrastructure dependencies
	logger     Logger
	httpClient *http.Client

	// Core service dependencies
	manifestLoader    manifest.Loader
	manifestGenerator manifest.Generator
	planner           planner.Planner
	executor          executor.Executor
	broker            broker.Broker
	stateManager      state.Manager
}

// container implements the Container interface with concrete dependencies.
// It holds all resolved services and provides access through interface methods.
type container struct {
	cfg               *config.Config
	logger            Logger
	httpClient        *http.Client
	manifestLoader    manifest.Loader
	manifestGenerator manifest.Generator
	planner           planner.Planner
	executor          executor.Executor
	broker            broker.Broker
	stateManager      state.Manager
}

// Core service accessors
func (c *container) Manifest() manifest.Loader             { return c.manifestLoader }
func (c *container) ManifestGenerator() manifest.Generator { return c.manifestGenerator }
func (c *container) Planner() planner.Planner              { return c.planner }
func (c *container) Executor() executor.Executor           { return c.executor }
func (c *container) Broker() broker.Broker                 { return c.broker }
func (c *container) State() state.Manager                  { return c.stateManager }

// Configuration and infrastructure accessors
func (c *container) Config() *config.Config   { return c.cfg }
func (c *container) Logger() Logger           { return c.logger }
func (c *container) HTTPClient() *http.Client { return c.httpClient }

// Close performs cleanup of container resources.
// It attempts to close any services that implement io.Closer,
// logging any errors that occur during cleanup.
func (c *container) Close() error {
	var errs []error

	// Check each service for io.Closer interface and close if implemented
	if closer, ok := c.stateManager.(interface{ Close() error }); ok {
		if err := closer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("state manager close: %w", err))
		}
	}

	if closer, ok := c.broker.(interface{ Close() error }); ok {
		if err := closer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("broker close: %w", err))
		}
	}

	if closer, ok := c.executor.(interface{ Close() error }); ok {
		if err := closer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("executor close: %w", err))
		}
	}

	if closer, ok := c.manifestLoader.(interface{ Close() error }); ok {
		if err := closer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("manifest loader close: %w", err))
		}
	}

	if closer, ok := c.manifestGenerator.(interface{ Close() error }); ok {
		if err := closer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("manifest generator close: %w", err))
		}
	}

	// Close HTTP client transport if it implements closer
	if c.httpClient.Transport != nil {
		if closer, ok := c.httpClient.Transport.(interface{ Close() error }); ok {
			if err := closer.Close(); err != nil {
				errs = append(errs, fmt.Errorf("http client transport close: %w", err))
			}
		}
	}

	// Return combined errors if any occurred
	if len(errs) > 0 {
		return fmt.Errorf("container close errors: %v", errs)
	}

	return nil
}

// build assembles the container with all dependencies resolved.
// It validates that required dependencies are present and creates default implementations
// for any that are missing but can be auto-constructed.
func (b *builder) build() (Container, error) {
	start := time.Now()
	// Provide defaults for missing dependencies
	// Configuration must be resolved first as other services depend on it
	if b.cfg == nil {
		var err error
		b.cfg, err = provideConfigWithDefaults()
		if err != nil {
			return nil, fmt.Errorf("di: failed to provide default config: %w", err)
		}
	}

	// Logger depends on config for level/format settings
	if b.logger == nil {
		b.logger = provideLoggerWithConfig(b.cfg)
	}

	// HTTP client depends on config for timeout settings
	if b.httpClient == nil {
		b.httpClient = provideHTTPClientWithConfig(b.cfg)
	}

	if b.manifestLoader == nil {
		b.manifestLoader = provideManifest()
	}

	if b.manifestGenerator == nil {
		b.manifestGenerator = provideManifestGeneratorWithConfig(b.cfg, b.logger)
	}

	if b.planner == nil {
		b.planner = providePlanner()
	}

	// Executor can use config for timeout settings
	if b.executor == nil {
		b.executor = provideExecutorWithConfig(b.cfg, b.logger)
	}

	// Broker depends on config for GitHub/Slack credentials and dry-run mode
	if b.broker == nil {
		if b.requireProductionCredentials {
			broker, err := provideBrokerForProduction(b.cfg, b.httpClient, b.logger)
			if err != nil {
				return nil, fmt.Errorf("di: failed to provide production broker: %w", err)
			}
			b.broker = broker
		} else {
			b.broker = provideBrokerWithConfig(b.cfg, b.httpClient, b.logger)
		}
	}

	// State manager depends on config for storage directory and settings
	if b.stateManager == nil {
		b.stateManager = provideStateWithConfig(b.cfg, b.logger)
	}

	// Validate that all required dependencies are present
	if b.cfg == nil {
		return nil, fmt.Errorf("di: config is required")
	}
	if b.logger == nil {
		return nil, fmt.Errorf("di: logger is required")
	}
	if b.httpClient == nil {
		return nil, fmt.Errorf("di: http client is required")
	}
	if b.manifestLoader == nil {
		return nil, fmt.Errorf("di: manifest loader is required")
	}
	if b.manifestGenerator == nil {
		return nil, fmt.Errorf("di: manifest generator is required")
	}
	if b.planner == nil {
		return nil, fmt.Errorf("di: planner is required")
	}
	if b.executor == nil {
		return nil, fmt.Errorf("di: executor is required")
	}
	if b.broker == nil {
		return nil, fmt.Errorf("di: broker is required")
	}
	if b.stateManager == nil {
		return nil, fmt.Errorf("di: state manager is required")
	}

	// Create and return the container with all dependencies wired
	c := &container{
		cfg:               b.cfg,
		logger:            b.logger,
		httpClient:        b.httpClient,
		manifestLoader:    b.manifestLoader,
		manifestGenerator: b.manifestGenerator,
		planner:           b.planner,
		executor:          b.executor,
		broker:            b.broker,
		stateManager:      b.stateManager,
	}

	// Log container creation metrics if instrumentation is enabled
	if b.enableInstrumentation && b.logger != nil {
		duration := time.Since(start)
		b.logger.Debug("DI container created",
			"duration_ms", duration.Milliseconds(),
			"config_present", b.cfg != nil,
			"production_mode", b.requireProductionCredentials,
			"services_count", 6, // manifest (loader, generator), planner, executor, broker, state
		)
	}

	return c, nil
}

// Configuration options

// WithConfig injects an explicit configuration object into the container.
// If not provided, the container will attempt to load configuration from
// environment variables and default values.
func WithConfig(cfg *config.Config) Option {
	return func(b *builder) error {
		if cfg == nil {
			return fmt.Errorf("config cannot be nil")
		}
		b.cfg = cfg
		return nil
	}
}

// WithLogger injects a custom logger into the container.
// Useful for testing or when using a specific logging framework.
func WithLogger(logger Logger) Option {
	return func(b *builder) error {
		if logger == nil {
			return fmt.Errorf("logger cannot be nil")
		}
		b.logger = logger
		return nil
	}
}

// WithHTTPClient injects a custom HTTP client into the container.
// Useful for testing with mock clients or custom transport configurations.
func WithHTTPClient(client *http.Client) Option {
	return func(b *builder) error {
		if client == nil {
			return fmt.Errorf("http client cannot be nil")
		}
		b.httpClient = client
		return nil
	}
}

// Core service override options for testing

// WithManifestLoader injects a custom manifest loader implementation.
func WithManifestLoader(loader manifest.Loader) Option {
	return func(b *builder) error {
		if loader == nil {
			return fmt.Errorf("manifest loader cannot be nil")
		}
		b.manifestLoader = loader
		return nil
	}
}

// WithManifestGenerator injects a custom manifest generator implementation.
func WithManifestGenerator(generator manifest.Generator) Option {
	return func(b *builder) error {
		if generator == nil {
			return fmt.Errorf("manifest generator cannot be nil")
		}
		b.manifestGenerator = generator
		return nil
	}
}

// WithPlanner injects a custom planner implementation.
func WithPlanner(p planner.Planner) Option {
	return func(b *builder) error {
		if p == nil {
			return fmt.Errorf("planner cannot be nil")
		}
		b.planner = p
		return nil
	}
}

// WithExecutor injects a custom executor implementation.
func WithExecutor(exec executor.Executor) Option {
	return func(b *builder) error {
		if exec == nil {
			return fmt.Errorf("executor cannot be nil")
		}
		b.executor = exec
		return nil
	}
}

// WithBroker injects a custom broker implementation.
func WithBroker(b broker.Broker) Option {
	return func(builder *builder) error {
		if b == nil {
			return fmt.Errorf("broker cannot be nil")
		}
		builder.broker = b
		return nil
	}
}

// WithStateManager injects a custom state manager implementation.
func WithStateManager(mgr state.Manager) Option {
	return func(b *builder) error {
		if mgr == nil {
			return fmt.Errorf("state manager cannot be nil")
		}
		b.stateManager = mgr
		return nil
	}
}

// Build options

// WithProductionCredentials requires that production-level credentials (GitHub token)
// be available for commands that create PRs or make API calls. If credentials are
// missing and dry-run is not enabled, container creation will fail.
func WithProductionCredentials() Option {
	return func(b *builder) error {
		b.requireProductionCredentials = true
		return nil
	}
}

// WithInstrumentation enables logging hooks and metrics for DI container creation
// and service usage. This is useful for debugging and monitoring container
// performance in production environments.
func WithInstrumentation() Option {
	return func(b *builder) error {
		b.enableInstrumentation = true
		return nil
	}
}
