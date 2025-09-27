package manifest

import "time"

// Manifest is the root structure parsed from .cascade.yaml.
type Manifest struct {
	ManifestVersion int      `yaml:"manifest_version"`
	Defaults        Defaults `yaml:"defaults"`
	Modules         []Module `yaml:"modules"`
}

// Defaults captures project-wide defaults inherited by dependents.
type Defaults struct {
	Branch         string        `yaml:"branch"`
	Tests          []Command     `yaml:"tests"`
	ExtraCommands  []Command     `yaml:"extra_commands"`
	Labels         []string      `yaml:"labels"`
	CommitTemplate string        `yaml:"commit_template"`
	Notifications  Notifications `yaml:"notifications"`
	PR             PRConfig      `yaml:"pr"`
}

// Module describes a releasable module and its dependents.
type Module struct {
	Name            string      `yaml:"name"`
	Module          string      `yaml:"module"`
	Repo            string      `yaml:"repo"`
	ReleaseArtifact string      `yaml:"release_artifact"`
	Dependents      []Dependent `yaml:"dependents"`
}

// Dependent defines a repo that consumes a module.
type Dependent struct {
	Repo          string            `yaml:"repo"`
	Module        string            `yaml:"module"`
	ModulePath    string            `yaml:"module_path"`
	Branch        string            `yaml:"branch"`
	Tests         []Command         `yaml:"tests"`
	ExtraCommands []Command         `yaml:"extra_commands"`
	Labels        []string          `yaml:"labels"`
	Notifications Notifications     `yaml:"notifications"`
	PR            PRConfig          `yaml:"pr"`
	Canary        bool              `yaml:"canary"`
	Skip          bool              `yaml:"skip"`
	Env           map[string]string `yaml:"env"`
	Timeout       time.Duration     `yaml:"timeout"`
}

// Command represents an executable command.
type Command struct {
	Cmd []string `yaml:"cmd"`
	Dir string   `yaml:"dir"`
}

// PRConfig customises PR metadata.
type PRConfig struct {
	TitleTemplate string   `yaml:"title"`
	BodyTemplate  string   `yaml:"body_template"`
	Reviewers     []string `yaml:"reviewers"`
	TeamReviewers []string `yaml:"team_reviewers"`
}

// Notifications holds optional notification targets.
type Notifications struct {
	SlackChannel string `yaml:"slack_channel"`
	Webhook      string `yaml:"webhook"`
}
