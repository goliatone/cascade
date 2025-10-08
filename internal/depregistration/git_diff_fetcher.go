package depregistration

import (
	"context"
	"fmt"
	"os/exec"
)

// GitDiffFetcher shells out to git to retrieve the diff between two
// references. It implements the DiffFetcher interface.
type GitDiffFetcher struct {
	workdir string
	extra   []string
}

// NewGitDiffFetcher constructs a fetcher rooted at workdir. Optional pathspecs
// or additional git diff arguments can be provided via extra.
func NewGitDiffFetcher(workdir string, extraArgs ...string) *GitDiffFetcher {
	return &GitDiffFetcher{workdir: workdir, extra: append([]string{}, extraArgs...)}
}

// Diff returns the unified diff between baseRef and headRef focusing on go.mod
// files.
func (f *GitDiffFetcher) Diff(ctx context.Context, baseRef, headRef string) ([]byte, error) {
	if headRef == "" {
		headRef = "HEAD"
	}

	args := []string{"diff", "--unified=0", baseRef, headRef}
	args = append(args, f.extra...)
	// Limit pathspec to go.mod files, but fall back to full diff when extra is
	// specified.
	if len(f.extra) == 0 {
		args = append(args, "--", "go.mod", ":(glob)**/go.mod")
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = f.workdir

	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %v: %w", args, err)
	}

	return out, nil
}
