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
	CloneURL      string            `yaml:"clone_url,omitempty"`
	Module        string            `yaml:"module"`
	ModulePath    string            `yaml:"module_path"`
	Branch        string            `yaml:"branch,omitempty"`
	Tests         []Command         `yaml:"tests,omitempty"`
	ExtraCommands []Command         `yaml:"extra_commands,omitempty"`
	Labels        []string          `yaml:"labels,omitempty"`
	Notifications Notifications     `yaml:"notifications,omitempty"`
	PR            PRConfig          `yaml:"pr,omitempty"`
	Canary        bool              `yaml:"canary,omitempty"`
	Skip          bool              `yaml:"skip,omitempty"`
	Env           map[string]string `yaml:"env,omitempty"`
	Timeout       time.Duration     `yaml:"timeout,omitempty"`
}

// Command represents an executable command.
type Command struct {
	Cmd []string `yaml:"cmd"`
	Dir string   `yaml:"dir,omitempty"`
}

// PRConfig customises PR metadata.
type PRConfig struct {
	TitleTemplate string   `yaml:"title,omitempty"`
	BodyTemplate  string   `yaml:"body_template,omitempty"`
	Reviewers     []string `yaml:"reviewers,omitempty"`
	TeamReviewers []string `yaml:"team_reviewers,omitempty"`
}

// Notifications holds optional notification targets.
type Notifications struct {
	SlackChannel string `yaml:"slack_channel,omitempty"`
	OnFailure    bool   `yaml:"on_failure,omitempty"`
	OnSuccess    bool   `yaml:"on_success,omitempty"`
	Webhook      string `yaml:"webhook,omitempty"`
}
