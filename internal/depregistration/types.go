package depregistration

import "context"

// Detector defines the contract for discovering newly introduced internal
// dependencies between two git references.
type Detector interface {
	Detect(ctx context.Context, baseRef, headRef string) ([]DependencyDelta, error)
}

// ChangeType captures the kind of dependency change identified by the detector.
type ChangeType string

const (
	// ChangeTypeAdded indicates a new dependency requirement was added.
	ChangeTypeAdded ChangeType = "added"
)

// DependencyDelta represents a dependency mutation detected between two
// revisions. Additional fields can be added as the implementation evolves.
type DependencyDelta struct {
	Module    string     `json:"module"`
	Version   string     `json:"version"`
	GoModPath string     `json:"go_mod_path"`
	Change    ChangeType `json:"change_type"`
}
