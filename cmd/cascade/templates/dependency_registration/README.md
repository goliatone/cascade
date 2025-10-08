# Dependency Registration Automation

Cascade generated this file to explain the dependency-registration workflow files.

## Files

- `.github/workflows/dependency-registration.yml` — GitHub Actions workflow that detects new internal dependencies and notifies upstream repositories.
- `.github/dependency-registration.yml` — Configuration stub for skips, alternate workflow names, and dry-run defaults.
- `scripts/detect-new-goliatone-deps.sh` — Thin wrapper that runs the detection CLI locally.

## Usage

1. Update `.github/dependency-registration.yml` if you need to ignore specific modules, customize the dependent workflow name, or override the branch used for workflow dispatches.
2. Ensure `CASCADE_DEP_NOTIFY_TOKEN` is configured in repo secrets with `repo`, `workflow`, and `issues:write` scopes.
3. On every PR that touches `go.mod`, the workflow will detect new `github.com/goliatone/*` dependencies and notify the owning repository.
4. Use the shell script during development to preview detection output:
   ```bash
   ./scripts/detect-new-goliatone-deps.sh --base origin/main --head HEAD
   ```
