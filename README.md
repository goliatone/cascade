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
  --output=.cascade.yaml
```

Highlights:

- **Workspace discovery** scans `$WORKSPACE` for Go modules that already depend on `go-errors` and pre-populates the manifest.
- **GitHub discovery** (enabled by `--github-org` or config defaults) augments the workspace scan by hitting the GitHub API to find other dependents in the organization.
- **Version resolution** understands `--version=latest` or an omitted version flag and resolves the latest published tag, falling back to local usage when offline.
- **Config-driven defaults** for tests, notifications, branch naming, and discovery filters reduce the number of CLI flags you need.

The generated `.cascade.yaml` lands in the current directory unless you pass an absolute `--output` path.

#### 2. Inspect & Adjust

```bash
cat .cascade.yaml
```

Confirm branch names, commands, labels, and notification targets before executing. Edit as needed.
Cascade shows a discovery summary (workspace + GitHub results) and, unless `--yes`/`--non-interactive` is set, prompts for confirmation so you can deselect repositories before generation.

#### 3. Plan the Rollout (Dry Run)

```bash
cascade plan \
  --manifest=.cascade.yaml \
  --module="$TARGET_MODULE" \
  --version="$TARGET_VERSION" \
  --dry-run \
  --quiet
```

The plan output lists repositories, branches, commands, and PR metadata without touching any repositories.

#### 4. Execute the Release

```bash
cascade release \
  --manifest=.cascade.yaml \
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
cascade plan --manifest=.cascade.yaml --dry-run
cascade release --manifest=.cascade.yaml
cascade resume go-errors@v1.4.0
cascade revert go-errors@v1.4.0
```

### Workflow Generation

`cascade workflow generate` scaffolds a GitHub Actions workflow that runs Cascade whenever a release tag is pushed. The command creates `.github/workflows/cascade-release.yml` by default, infers repository metadata, and can be re-run safely to overwrite the workflow when templates change.

```bash
cascade workflow generate --output .github/workflows/cascade-release.yml
```

**Flags:**
- `--output` (default `.github/workflows/cascade-release.yml`) – choose a different path or filename
- `--template` – render an alternate Go `text/template` file instead of the embedded default
- `--module` – override the module detected from `go.mod` (handy for tests or monorepos)
- Global flags like `--dry-run`, `--yes`, and `--force` control preview and confirmation behaviour

**Secrets and tokens:**
- The generated workflow expects a repository secret named `CASCADE_GITHUB_TOKEN`. Map this to a personal access token with the repository scopes Cascade requires (cross-repo cloning, PR creation).
- GitHub automatically exposes `GITHUB_TOKEN`, but it is limited to the current repository. If you only need repo-scoped permissions you can define a secret named `CASCADE_GITHUB_TOKEN` that references `${{ secrets.GITHUB_TOKEN }}`.
- Slack notifications (optional) read from `CASCADE_SLACK_TOKEN`; omit the secret if Slack is not used.

**Custom templates:**
1. Copy `cmd/cascade/templates/workflow/github_actions.yaml.tmpl` to a location you control.
2. Adjust steps, matrix definitions, or secrets as needed.
3. Run `cascade workflow generate --template custom-workflow.tmpl --output .github/workflows/cascade-release.yml` to render your version.

Use `--dry-run` to print the rendered YAML to stdout without touching the filesystem. The command always creates missing parent directories, so it is safe to run in a fresh repository.

## CI/CD Mode

Cascade supports running in CI/CD environments without requiring a local workspace. This enables dependency checking and PR automation directly from your CI pipeline.

### Dependency Checking Strategies

Cascade uses intelligent dependency checking to avoid unnecessary updates:

- **`local`** - Check dependencies using local workspace repositories (fastest, requires workspace)
- **`remote`** - Clone repositories remotely via shallow git clones (works without workspace)
- **`auto`** - Try local first, fall back to remote if unavailable (recommended)

When a local workspace reports that a repository still requires an update, Cascade
confirms the result against the remote repository before scheduling work. This keeps
plan/release runs accurate even if a cached workspace has not been refreshed since the dependent was fixed upstream.

### CI/CD Configuration

Use the following flags to optimize for CI/CD environments:

```bash
cascade release \
  --manifest=.cascade.yaml \
  --check-strategy=remote \
  --check-parallel=8 \
  --check-cache-ttl=10m \
  --skip-up-to-date
```

**Flags:**
- `--check-strategy` - Dependency checking mode (`local`, `remote`, `auto`)
- `--check-parallel` - Number of parallel dependency checks (default: CPU count)
- `--check-cache-ttl` - Cache expiration time (default: 5m)
- `--check-timeout` - Per-repository check timeout (default: 30s)
- `--skip-up-to-date` - Skip repositories already at target version

### Authentication

For private repositories, configure authentication via environment variables:

```bash
# GitHub token (required for private repos)
export CASCADE_GITHUB_TOKEN=ghp_example123
# or
export GITHUB_TOKEN=ghp_example123
# or
export GH_TOKEN=ghp_example123

# SSH key path (optional, defaults to ~/.ssh/id_rsa)
export SSH_KEY_PATH=~/.ssh/cascade_deploy_key
```

### Performance Optimization

**Cache Hit Rate**: Cascade caches dependency information to avoid redundant git operations. Monitor cache performance:

```
Dependency Checking (remote mode):
- Checked 14 repositories (5 cached, 9 fetched)
- 11 repositories up-to-date, skipped
- 3 require updates
- Check duration: 12.3s (parallel: 4)
```

**Tuning Tips:**
- Increase `--check-parallel` for large dependency graphs
- Use `--check-strategy=remote` in CI environments without workspace
- Set `--check-cache-ttl=10m` for repeated CI runs
- Monitor warnings for slow checks (>30s) and low cache hit rates (<50%)

### Example: GitHub Actions Workflow

See the "CI/CD Pipeline Examples" section below for complete workflow configurations.

## Configuration

### Manifest File

The `.cascade.yaml` manifest file defines module dependencies and update rules. You can generate a starter manifest using `cascade manifest generate`, or craft one manually. Each repository can now describe both how it should be released (the top-level `module` block) and how it expects upstream releases to validate against it (entries under the top-level `dependents` map):

```yaml
manifest_version: 1

module:
  module: github.com/goliatone/go-errors
  branch: main
  tests:
    - cmd: [go, test, ./...]
  extra_commands:
    - cmd: [go, vet, ./...]
  notifications:
    slack_channel: "#go-errors"

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
  github_issues:
    enabled: true
    labels:
      - cascade-failure
      - dependencies

dependents:
  github.com/goliatone/go-errors:
    tests:
      - cmd: [task, dependent:test]
    extra_commands:
      - cmd: [task, dependent:lint]
    env:
      CI: "true"
```

When Cascade plans a release it merges configuration in this order:

1. Global `defaults` from the releasing repository
2. The dependent repository's own `module` block (if present in its `.cascade.yaml`)
3. The dependent repository's `dependents[<module>]` override for the module being updated

This precedence keeps legacy manifests working while giving each dependent full control over the tests, extra commands, environment, notifications, and timeouts it requires.

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
    github_issues:
      enabled: true
      labels: ["cascade-failure", "dependencies"]
  discovery:
    enabled: true
    max_depth: 3

The logic that maps configuration into manifest defaults lives in `pkg/config/defaults.go`. Update those helpers to adjust the built-in branch, test command, or discovery filters across the CLI.
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

Cascade resolves the latest tag, discovers dependents in the workspace and GitHub org, applies the default test command, and writes the manifest to `.cascade.yaml`.

### Examples

See the `examples/` directory for complete manifests:
- `basic-manifest.yaml` – minimal configuration
- `full-featured-manifest.yaml` – multiple dependents and notifications
- `custom-templates-manifest.yaml` – advanced templating and workflows

## CI/CD Pipeline Examples

### GitHub Actions

Create `.github/workflows/cascade-release.yml` to run whenever a new tag matching `v*.*.*` is pushed. The workflow builds Cascade from the tagged source, resolves the module/version automatically, and keeps the state directory aligned with the uploaded artifact.

```yaml
name: Cascade Dependency Release

on:
  push:
    tags:
      - 'v*.*.*'

jobs:
  release:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      pull-requests: write
    env:
      TARGET_VERSION: ${{ github.ref_name }}
      CASCADE_STATE_DIR: ${{ github.workspace }}/.cascade/state
      CASCADE_GITHUB_TOKEN: ${{ secrets.CASCADE_GITHUB_TOKEN }}

    steps:
      - name: Checkout
        uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.21'

      - name: Discover module path
        id: module
        run: echo "module=$(go list -m)" >> "$GITHUB_OUTPUT"

      - name: Build Cascade CLI from tag
        run: go build -o /usr/local/bin/cascade ./cmd/cascade

      - name: Generate Manifest
        env:
          GITHUB_OWNER: ${{ github.repository_owner }}
        run: |
          cascade manifest generate \
            --module-path="${{ steps.module.outputs.module }}" \
            --version="${TARGET_VERSION}" \
            --github-org="${GITHUB_OWNER}" \
            --yes \
            --output=.cascade.yaml

      - name: Plan Release
        run: |
          cascade plan \
            --manifest=.cascade.yaml \
            --module="${{ steps.module.outputs.module }}" \
            --version="${TARGET_VERSION}" \
            --check-strategy=remote \
            --check-parallel=8 \
            --skip-up-to-date \
            --quiet

      - name: Execute Release
        env:
          CASCADE_SLACK_TOKEN: ${{ secrets.CASCADE_SLACK_TOKEN }}
        run: |
          cascade release \
            --manifest=.cascade.yaml \
            --module="${{ steps.module.outputs.module }}" \
            --version="${TARGET_VERSION}" \
            --check-strategy=remote \
            --check-parallel=8 \
            --check-cache-ttl=10m \
            --skip-up-to-date \
            --parallel=4 \
            --timeout=20m \
            --verbose

      - name: Upload Cascade state
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: cascade-state
          path: ${{ env.CASCADE_STATE_DIR }}
```

> **Secrets:** Provision `CASCADE_GITHUB_TOKEN` with a PAT that can push branches and open PRs across dependent repositories. `CASCADE_SLACK_TOKEN` remains optional.

### GitLab CI

Create `.gitlab-ci.yml`:

```yaml
cascade:release:
  stage: deploy
  image: golang:1.21
  script:
    - go install github.com/goliatone/cascade@latest
    - |
      cascade manifest generate \
        --module-path="${MODULE}" \
        --version="${VERSION}" \
        --github-org="${CI_PROJECT_NAMESPACE}" \
        --yes \
        --output=.cascade.yaml
    - |
      cascade release \
        --manifest=.cascade.yaml \
        --check-strategy=remote \
        --check-parallel=8 \
        --check-cache-ttl=10m \
        --skip-up-to-date \
        --parallel=4 \
        --timeout=20m \
        --verbose
  variables:
    MODULE: "github.com/example/module"
    VERSION: "v1.0.0"
  artifacts:
    paths:
      - .cascade/state/
    when: always
  only:
    - web
```

### Environment Variables for CI

Configure these secrets in your CI environment:

- `CASCADE_GITHUB_TOKEN` or `GITHUB_TOKEN` - GitHub API access
- `CASCADE_SLACK_TOKEN` - Slack notifications (optional)
- `SSH_KEY_PATH` - Custom SSH key path (optional)

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
