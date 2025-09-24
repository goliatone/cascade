package state

// Storage persists summaries and item states for cascade executions.
type Storage interface {
	LoadSummary(module, version string) (*Summary, error)
	SaveSummary(summary *Summary) error
	SaveItemState(module, version string, item ItemState) error
	LoadItemStates(module, version string) ([]ItemState, error)
}

type nopStorage struct{}

func (n *nopStorage) LoadSummary(module, version string) (*Summary, error) {
	return nil, nil
}

func (n *nopStorage) SaveSummary(summary *Summary) error {
	return nil
}

func (n *nopStorage) SaveItemState(module, version string, item ItemState) error {
	return nil
}

func (n *nopStorage) LoadItemStates(module, version string) ([]ItemState, error) {
	return nil, nil
}
