package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/goliatone/cascade/pkg/automation"
	"github.com/spf13/cobra"
)

type watchOptions struct {
	command         string
	debounce        time.Duration
	dir             string
	timeout         time.Duration
	recursive       bool
	once            bool
	allowConcurrent bool
	verbose         bool
}

func newWatchCommand() *cobra.Command {
	opts := watchOptions{debounce: automation.DefaultDebounce}
	cmd := &cobra.Command{
		Use:   "watch <path...> --exec <command>",
		Short: "Run a local command when watched files change",
		Long: `Watch files or directories and run a shell command when changes occur.

The watch command is foreground local automation. It does not install a daemon or
persist workflow configuration.`,
		Example: `  cascade watch .version --exec 'echo changed'
  cascade watch "$WORKSPACE/*/.version" --exec 'codex --ask-for-approval never exec --sandbox workspace-write --cd "$WORKSPACE" --skip-git-repo-check "update GOLIATONE.md"'`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWatch(cmd, args, opts)
		},
	}

	cmd.Flags().StringVar(&opts.command, "exec", "", "shell command to run when a watched path changes")
	cmd.Flags().DurationVar(&opts.debounce, "debounce", automation.DefaultDebounce, "duration used to coalesce bursty file events")
	cmd.Flags().StringVar(&opts.dir, "dir", "", "working directory for the executed command")
	cmd.Flags().DurationVar(&opts.timeout, "timeout", 0, "optional timeout for each command execution")
	cmd.Flags().BoolVar(&opts.recursive, "recursive", false, "watch existing subdirectories under directory targets")
	cmd.Flags().BoolVar(&opts.once, "once", false, "validate inputs and run the command once without starting a watcher")
	cmd.Flags().BoolVar(&opts.allowConcurrent, "allow-concurrent", false, "allow overlapping command executions")
	cmd.Flags().BoolVar(&opts.verbose, "verbose", false, "print watcher lifecycle logs to stderr")

	return cmd
}

func runWatch(cmd *cobra.Command, args []string, opts watchOptions) error {
	ctx, stop := signalAwareContext(commandContext(cmd))
	defer stop()

	targets, err := resolveWatchTargets(args)
	if err != nil {
		return newValidationError("invalid watch targets", err)
	}

	dir, err := expandHome(opts.dir)
	if err != nil {
		return newValidationError("invalid working directory", err)
	}

	workflow := automation.Workflow{
		Targets: targets,
		Exec: automation.ExecSpec{
			Command: opts.command,
			Dir:     dir,
			Timeout: opts.timeout,
			Stdin:   cmd.InOrStdin(),
			Stdout:  cmd.OutOrStdout(),
			Stderr:  cmd.ErrOrStderr(),
		},
		Debounce:        opts.debounce,
		AllowConcurrent: opts.allowConcurrent,
		Recursive:       opts.recursive,
	}

	if err := workflow.Validate(); err != nil {
		return newValidationError("invalid watch workflow", err)
	}
	if err := validateWatchTargetPaths(targets); err != nil {
		return newFileError("invalid watch targets", err)
	}
	if err := validateWatchWorkingDirectory(dir); err != nil {
		return newFileError("invalid watch working directory", err)
	}

	var logger automation.Logger
	if opts.verbose {
		logger = automation.NewTextLogger(cmd.ErrOrStderr())
	}

	if opts.once {
		if logger != nil {
			logger.Printf("starting run-1 for %s", targets[0].Path)
		}
		result, err := automation.NewShellExecutor().Execute(ctx, automation.RunRequest{
			ID: "run-1",
			Event: automation.Event{
				Path:      targets[0].Path,
				Op:        automation.OperationUnknown,
				WatchRoot: targets[0].Path,
				Time:      time.Now(),
			},
			Exec: workflow.Exec,
		})
		if err != nil {
			if logger != nil {
				logger.Printf("failed run-1: %v", err)
			}
			if result.TimedOut || result.Canceled {
				return newExecutionError("watch command execution failed", err)
			}
			return newExecutionError("watch command exited unsuccessfully", err)
		}
		if logger != nil {
			logger.Printf("finished run-1 with exit code %d", result.ExitCode)
		}
		return nil
	}

	runner, err := automation.NewRunner(workflow, automation.RunnerOptions{
		Logger: logger,
	})
	if err != nil {
		return newValidationError("invalid watch workflow", err)
	}
	if err := runner.Run(ctx); err != nil {
		return newExecutionError("watch failed", err)
	}
	return nil
}

func resolveWatchTargets(args []string) ([]automation.WatchTarget, error) {
	var targets []automation.WatchTarget
	seen := make(map[string]struct{})

	for _, arg := range args {
		expanded, err := expandHome(arg)
		if err != nil {
			return nil, err
		}
		if hasGlob(expanded) {
			matches, err := filepath.Glob(expanded)
			if err != nil {
				return nil, fmt.Errorf("invalid glob %q: %w", arg, err)
			}
			if len(matches) == 0 {
				return nil, fmt.Errorf("glob %q matched no paths", arg)
			}
			for _, match := range matches {
				targets = appendWatchTarget(targets, seen, match)
			}
			continue
		}
		targets = appendWatchTarget(targets, seen, expanded)
	}

	if len(targets) == 0 {
		return nil, fmt.Errorf("at least one watch target is required")
	}
	return targets, nil
}

func appendWatchTarget(targets []automation.WatchTarget, seen map[string]struct{}, path string) []automation.WatchTarget {
	clean := filepath.Clean(path)
	if abs, err := filepath.Abs(clean); err == nil {
		clean = abs
	}
	if _, ok := seen[clean]; ok {
		return targets
	}
	seen[clean] = struct{}{}
	return append(targets, automation.WatchTarget{Path: clean})
}

func validateWatchTargetPaths(targets []automation.WatchTarget) error {
	for _, target := range targets {
		if _, err := os.Stat(target.Path); err != nil {
			return fmt.Errorf("stat %q: %w", target.Path, err)
		}
	}
	return nil
}

func validateWatchWorkingDirectory(dir string) error {
	if strings.TrimSpace(dir) == "" {
		return nil
	}
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("stat %q: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%q is not a directory", dir)
	}
	return nil
}

func expandHome(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	if path == "~" {
		return home, nil
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~/")), nil
}

func hasGlob(path string) bool {
	return strings.ContainsAny(path, "*?[")
}
