package di

import (
	"fmt"

	"github.com/goliatone/cascade/internal/broker"
	"github.com/goliatone/cascade/internal/executor"
	"github.com/goliatone/cascade/internal/manifest"
	"github.com/goliatone/cascade/internal/planner"
	"github.com/goliatone/cascade/internal/state"
	"github.com/goliatone/cascade/pkg/config"
)

// Container exposes resolved dependencies for the CLI orchestration layer.
type Container interface {
	Manifest() manifest.Loader
	Planner() planner.Planner
	Executor() executor.Executor
	Broker() broker.Broker
	State() state.Manager
	Config() *config.Config
}

// Option customises container construction.
type Option func(*builder) error

// New creates a container with default wiring.
func New(opts ...Option) (Container, error) {
	b := &builder{}
	for _, opt := range opts {
		if err := opt(b); err != nil {
			return nil, err
		}
	}
	return b.build()
}

type builder struct {
	cfg *config.Config
}

func (b *builder) build() (Container, error) {
	return nil, fmt.Errorf("di: container build not implemented")
}

// WithConfig injects an explicit configuration object into the container.
func WithConfig(cfg *config.Config) Option {
	return func(b *builder) error {
		b.cfg = cfg
		return nil
	}
}
