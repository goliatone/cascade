# Cascade

A Go CLI tool that orchestrates automated dependency updates across multiple Go repositories.

## Purpose

Cascade manages dependency updates by coordinating operations across multiple Go repositories. It reads a manifest file that defines module dependencies and their relationships, then plans and executes update operations through GitHub pull requests.

## How It Works

Cascade follows a unidirectional data flow:
1. Load dependency manifest (YAML configuration)
2. Plan update operations based on dependency graph
3. Execute Git and Go module operations
4. Create and manage GitHub pull requests
5. Track operation state and checkpoints

## Architecture

Packages are segmented by responsibility:

- `internal/manifest` – manifest loading, validation, (future) generation
- `internal/planner` – computes deterministic work items
- `internal/executor` – performs git/go/command execution
- `internal/broker` – manages PR lifecycle and notifications
- `internal/state` – persists run summaries and item state for resume/revert
- `pkg/di` – dependency injection container wiring CLI to implementations

## Installation

```bash
go install github.com/goliatone/cascade@latest
```

## Usage

### End-to-End Example: Updating `github.com/goliatone/go-errors`

The following workflow shows how to bootstrap, plan, execute, and monitor a dependency rollout for the local `go-errors` module located at `~/Development/GO/src/github.com/goliatone/go-errors`.

> **Prerequisites**
> - Go 1.21+
> - GitHub token with `repo` scope exported as `CASCADE_GITHUB_TOKEN`
> - Optional Slack webhook/token for notifications
>
> ```bash
> export CASCADE_GITHUB_TOKEN=ghp_example123
> export CASCADE_SLACK_TOKEN=xoxb-example # optional
> ```

#### 1. Generate a Manifest

```bash
WORKSPACE=$HOME/.cache/cascade
TARGET_MODULE=github.com/goliatone/go-errors
TARGET_VERSION=v1.4.0

cascade manifest generate \
  --module-path="$TARGET_MODULE" \
  --version="$TARGET_VERSION" \
  --workspace="$WORKSPACE" \
  --github-org=goliatone \
  --yes \
  --output=deps.yaml
```

Highlights:

- **Workspace discovery** scans `$WORKSPACE` for Go modules that already depend on `go-errors` and pre-populates the manifest.
- **GitHub discovery** (enabled by `--github-org` or config defaults) augments the workspace scan by hitting the GitHub API to find other dependents in the organization.
- **Version resolution** understands `--version=latest` or an omitted version flag and resolves the latest published tag, falling back to local usage when offline.
- **Config-driven defaults** for tests, notifications, branch naming, and discovery filters reduce the number of CLI flags you need.

The generated `deps.yaml` lands in the current directory unless you pass an absolute `--output` path.

#### 2. Inspect & Adjust

```bash
cat deps.yaml
```

Confirm branch names, commands, labels, and notification targets before executing. Edit as needed.
Cascade shows a discovery summary (workspace + GitHub results) and, unless `--yes`/`--non-interactive` is set, prompts for confirmation so you can deselect repositories before generation.

#### 3. Plan the Rollout (Dry Run)

```bash
cascade plan \
  --manifest=deps.yaml \
  --module="$TARGET_MODULE" \
  --version="$TARGET_VERSION" \
  --dry-run \
  --quiet
```

The plan output lists repositories, branches, commands, and PR metadata without touching any repositories.

#### 4. Execute the Release

```bash
cascade release \
  --manifest=deps.yaml \
  --workspace="$WORKSPACE" \
  --parallel=2 \
  --timeout=15m \
  --state-dir="$WORKSPACE/state" \
  --verbose
```

Cascade will clone or update repos under the workspace, create branches like `auto/go-errors-v1.4.0`, update Go modules, run tests/commands, push commits, open PRs, and (if configured) notify Slack. Run state is persisted for recovery.

#### 5. Monitor & Recover

```bash
# Preview stored state without mutating anything
cascade resume go-errors@v1.4.0 --dry-run

# Continue an interrupted run
cascade resume go-errors@v1.4.0

# Roll back branches/PRs recorded in state
cascade revert go-errors@v1.4.0
```

### Command Reference

- `cascade manifest generate` – scaffold manifests with defaults, dependents, and notifications
- `cascade plan` – preview work items from a manifest or flags
- `cascade release` – execute the plan (honors `--dry-run`)
- `cascade resume` – resume an interrupted release using `module@version`
- `cascade revert` – delete branches/PRs captured in state summaries

```bash
# Quick cheatsheet
cascade manifest generate --module-path=$TARGET_MODULE --version=latest --github-org=goliatone --yes --dry-run
cascade plan --manifest=deps.yaml --dry-run
cascade release --manifest=deps.yaml
cascade resume go-errors@v1.4.0
cascade revert go-errors@v1.4.0
```

## Configuration

### Manifest File

The `deps.yaml` manifest file defines module dependencies and update rules. You can generate a starter manifest using `cascade manifest generate`, or craft one manually:

```yaml
manifest_version: 1

defaults:
  branch: main
  tests:
    - cmd: [go, test, ./..., -race]
  commit_template: "chore(deps): bump {{ .Module }} to {{ .Version }}"
  pr:
    title_template: "chore(deps): bump {{ .Module }} to {{ .Version }}"
    body_template: |
      Automated update for {{ .Module }} to {{ .Version }}.

      **Changes:**
      - {{ .Module }}: {{ .OldVersion }} → {{ .Version }}

modules:
  - name: go-errors
    module: github.com/goliatone/go-errors
    repo: goliatone/go-errors
    dependents:
      - repo: goliatone/go-logger
        module: github.com/goliatone/go-logger
        tests:
          - cmd: [go, test, ./...]
      - repo: goliatone/go-router
        module: github.com/goliatone/go-router
        tests:
          - cmd: [go, test, ./...]
          - cmd: [go, test, ./...]
            dir: router/

notifications:
  slack:
    channel: "#releases"
```

### Configuration Sources

Cascade uses the following precedence (highest to lowest):
1. Command-line flags
2. Environment variables (`CASCADE_*`)
3. Configuration files (`~/.config/cascade/config.yaml`)
4. Built-in defaults

### Manifest Generator Defaults

Populate `manifest_generator` in `config.yaml` to predefine discovery behavior, test commands, and notifications:

```yaml
manifest_generator:
  default_workspace: /Users/you/.cache/cascade
  default_branch: main
  tests:
    command: go test ./... -race -count=1
  notifications:
    enabled: true
    channels: ["#releases"]
    on_success: false
    on_failure: true
  discovery:
    enabled: true
    max_depth: 3
    include_patterns: ["services/*"]
    exclude_patterns: ["vendor/*", ".git/*", "node_modules/*"]
    github:
      enabled: true
      organization: goliatone
      include_patterns: ["go-*", "lib-*"]

integration:
  github:
    token: ${CASCADE_GITHUB_TOKEN}
```

With this configuration in place, manifest generation typically only needs the module path and desired version:

```bash
cascade manifest generate --module-path=$TARGET_MODULE --version=latest --yes
```

Cascade resolves the latest tag, discovers dependents in the workspace and GitHub org, applies the default test command, and writes the manifest to `deps.yaml`.

### Examples

See the `examples/` directory for complete manifests:
- `basic-manifest.yaml` – minimal configuration
- `full-featured-manifest.yaml` – multiple dependents and notifications
- `custom-templates-manifest.yaml` – advanced templating and workflows

## Development

### Prerequisites

- Go 1.21 or later
- Git
- GitHub access token (for PR operations)

### Commands

```bash
# Run tests
./taskfile dev:test

# Run tests with coverage
./taskfile dev:cover

# Install development tools
./taskfile dev:install

# Build binary
go build -o cascade ./cmd/cascade
```

### Testing

The project uses fixture-driven testing with contract tests. Test data is stored in `testdata/` directories within each package.

## Status

This is an early-stage project. Core interfaces are defined and the CLI supports planning, releasing, resuming, and reverting cascades, but expect rough edges while tasks in the *_TSK.md files are completed.

## License

MIT License — see LICENSE for details.
