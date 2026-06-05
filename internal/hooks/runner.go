package hooks

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/goliatone/cascade/pkg/config"
)

const defaultTimeout = 5 * time.Minute

type Runner struct {
	DefaultTimeout time.Duration
}

func NewRunner(defaultTimeout time.Duration) Runner {
	return Runner{DefaultTimeout: defaultTimeout}
}

func (r Runner) Run(ctx context.Context, phases []PhaseHooks, hookCtx Context) (Results, error) {
	var results Results
	for _, phase := range phases {
		for _, hook := range phase.Hooks {
			result := r.runHook(ctx, phase.Phase, hook, hookCtx)
			results = append(results, result)
			if result.Failed() {
				break
			}
		}
	}
	if err := results.Err(); err != nil {
		return results, err
	}
	return results, nil
}

func (r Runner) runHook(ctx context.Context, phase Phase, hook config.HookConfig, hookCtx Context) Result {
	result := Result{
		Phase:    phase,
		Hook:     hook,
		Name:     strings.TrimSpace(hook.Name),
		Command:  commandText(hook),
		Dir:      resolveDir(hookCtx.ModuleDir, hook.Dir),
		ExitCode: 0,
	}

	timeout := hook.Timeout
	if timeout <= 0 {
		timeout = r.DefaultTimeout
	}
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := commandArgs(hook)
	if len(args) == 0 {
		result.Err = errors.New("hook command is empty")
		result.ExitCode = -1
		return result
	}

	start := time.Now()
	env := buildEnv(hookCtx, hook.Env)
	program, err := resolveProgram(args[0], env, result.Dir)
	if err != nil {
		result.Duration = time.Since(start)
		result.Err = err
		result.ExitCode = -1
		return result
	}

	cmd := exec.CommandContext(runCtx, program, args[1:]...)
	cmd.Dir = result.Dir
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	result.Duration = time.Since(start)
	result.Output = string(output)
	if err != nil {
		result.ExitCode = exitCode(err)
		result.Timeout = runCtx.Err() == context.DeadlineExceeded
		if result.Timeout {
			result.Err = fmt.Errorf("hook timed out after %s", timeout)
		} else {
			result.Err = err
		}
	}
	return result
}

func commandArgs(hook config.HookConfig) []string {
	if strings.TrimSpace(hook.Run) != "" {
		if runtime.GOOS == "windows" {
			shell := os.Getenv("ComSpec")
			if strings.TrimSpace(shell) == "" {
				shell = "cmd"
			}
			return []string{shell, "/C", hook.Run}
		}
		return []string{"/bin/sh", "-c", hook.Run}
	}
	return hook.Cmd
}

func commandText(hook config.HookConfig) string {
	if strings.TrimSpace(hook.Run) != "" {
		return strings.TrimSpace(hook.Run)
	}
	return strings.Join(hook.Cmd, " ")
}

func resolveDir(moduleDir, hookDir string) string {
	if strings.TrimSpace(hookDir) == "" {
		return moduleDir
	}
	if filepath.IsAbs(hookDir) {
		return filepath.Clean(hookDir)
	}
	return filepath.Join(moduleDir, hookDir)
}

func buildEnv(hookCtx Context, hookEnv map[string]string) []string {
	env := os.Environ()
	for key, value := range hookCtx.Env {
		env = append(env, key+"="+value)
	}
	env = append(env,
		"CASCADE_COMMAND="+valueOrDefault(hookCtx.Command, "update local"),
		"CASCADE_MODULE="+hookCtx.Module,
		"CASCADE_MODULE_DIR="+hookCtx.ModuleDir,
		"CASCADE_WORKSPACE="+hookCtx.Workspace,
		"CASCADE_UPDATE_STATUS="+hookCtx.UpdateStatus,
		"CASCADE_UPDATED_COUNT="+strconv.Itoa(hookCtx.UpdatedCount),
		"CASCADE_TIDY_RAN="+strconv.FormatBool(hookCtx.TidyRan),
		"CASCADE_TIDY_FAILED="+strconv.FormatBool(hookCtx.TidyFailed),
	)
	for key, value := range hookEnv {
		env = append(env, key+"="+value)
	}
	return env
}

func resolveProgram(program string, env []string, workDir string) (string, error) {
	if strings.TrimSpace(program) == "" {
		return "", errors.New("hook command is empty")
	}
	if hasPathSeparator(program) {
		return program, nil
	}

	if pathValue, ok := envValue(env, "PATH"); ok {
		if resolved, err := lookPathWithEnv(program, pathValue, env, workDir); err == nil {
			return resolved, nil
		}
		return "", fmt.Errorf("executable %q not found in hook PATH", program)
	}

	resolved, err := exec.LookPath(program)
	if err != nil {
		return "", err
	}
	return resolved, nil
}

func lookPathWithEnv(program, pathValue string, env []string, workDir string) (string, error) {
	for _, dir := range filepath.SplitList(pathValue) {
		if dir == "" {
			dir = "."
		}
		dir = resolvePathDir(dir, workDir)
		for _, candidate := range executableCandidates(program, env) {
			path := filepath.Join(dir, candidate)
			if isExecutable(path) {
				return path, nil
			}
		}
	}
	return "", exec.ErrNotFound
}

func resolvePathDir(dir, workDir string) string {
	if filepath.IsAbs(dir) {
		return filepath.Clean(dir)
	}
	if strings.TrimSpace(workDir) == "" {
		return filepath.Clean(dir)
	}
	return filepath.Join(workDir, dir)
}

func executableCandidates(program string, env []string) []string {
	if runtime.GOOS != "windows" || filepath.Ext(program) != "" {
		return []string{program}
	}

	pathext, ok := envValue(env, "PATHEXT")
	if !ok || strings.TrimSpace(pathext) == "" {
		pathext = ".COM;.EXE;.BAT;.CMD"
	}

	seen := map[string]bool{strings.ToLower(program): true}
	candidates := []string{program}
	for _, ext := range strings.Split(pathext, ";") {
		ext = strings.TrimSpace(ext)
		if ext == "" {
			continue
		}
		candidate := program + ext
		key := strings.ToLower(candidate)
		if seen[key] {
			continue
		}
		seen[key] = true
		candidates = append(candidates, candidate)
	}
	return candidates
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode().Perm()&0o111 != 0
}

func hasPathSeparator(program string) bool {
	return strings.ContainsRune(program, os.PathSeparator) || (os.PathSeparator != '/' && strings.ContainsRune(program, '/'))
}

func envValue(env []string, key string) (string, bool) {
	prefix := key + "="
	if runtime.GOOS == "windows" {
		for i := len(env) - 1; i >= 0; i-- {
			if strings.EqualFold(envName(env[i]), key) {
				return strings.TrimPrefix(env[i], envName(env[i])+"="), true
			}
		}
		return "", false
	}
	for i := len(env) - 1; i >= 0; i-- {
		if strings.HasPrefix(env[i], prefix) {
			return strings.TrimPrefix(env[i], prefix), true
		}
	}
	return "", false
}

func envName(entry string) string {
	name, _, ok := strings.Cut(entry, "=")
	if !ok {
		return entry
	}
	return name
}

func valueOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func exitCode(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}
