package state

// ManagerOption configures the Manager during construction.
type ManagerOption func(*managerConfig)

type managerConfig struct {
	Storage Storage
	Locker  Locker
	Clock   Clock
	Logger  Logger
}

// NewManager constructs a state manager with the supplied options.
func NewManager(opts ...ManagerOption) Manager {
	cfg := managerConfig{}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(&cfg)
	}

	if cfg.Storage == nil {
		cfg.Storage = &nopStorage{}
	}
	if cfg.Locker == nil {
		cfg.Locker = nopLocker{}
	}
	if cfg.Clock == nil {
		cfg.Clock = systemClock{}
	}
	if cfg.Logger == nil {
		cfg.Logger = nopLogger{}
	}

	return &manager{
		storage: cfg.Storage,
		locker:  cfg.Locker,
		clock:   cfg.Clock,
		logger:  cfg.Logger,
	}
}

// WithStorage overrides the storage backend for the manager.
func WithStorage(storage Storage) ManagerOption {
	return func(cfg *managerConfig) {
		cfg.Storage = storage
	}
}

// WithLocker overrides the locking implementation for the manager.
func WithLocker(locker Locker) ManagerOption {
	return func(cfg *managerConfig) {
		cfg.Locker = locker
	}
}

// WithClock overrides the clock implementation for the manager.
func WithClock(clock Clock) ManagerOption {
	return func(cfg *managerConfig) {
		cfg.Clock = clock
	}
}

// WithLogger overrides the logger implementation for the manager.
func WithLogger(logger Logger) ManagerOption {
	return func(cfg *managerConfig) {
		cfg.Logger = logger
	}
}

type manager struct {
	storage Storage
	locker  Locker
	clock   Clock
	logger  Logger
}

func (m *manager) LoadSummary(module, version string) (*Summary, error) {
	return m.storage.LoadSummary(module, version)
}

func (m *manager) SaveSummary(summary *Summary) error {
	return m.storage.SaveSummary(summary)
}

func (m *manager) SaveItemState(module, version string, item ItemState) error {
	return m.storage.SaveItemState(module, version, item)
}

func (m *manager) LoadItemStates(module, version string) ([]ItemState, error) {
	return m.storage.LoadItemStates(module, version)
}
