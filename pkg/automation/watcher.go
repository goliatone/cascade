package automation

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
)

type FilesystemEventSource struct {
	Recursive bool
}

type watchBinding struct {
	targetPath string
	watchRoot  string
	isDir      bool
}

func NewFilesystemEventSource() *FilesystemEventSource {
	return &FilesystemEventSource{}
}

func (s *FilesystemEventSource) Watch(ctx context.Context, targets []WatchTarget) (<-chan Event, <-chan error, error) {
	if len(targets) == 0 {
		return nil, nil, fmt.Errorf("%w: at least one watch target is required", ErrWatchFailed)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, nil, fmt.Errorf("%w: create watcher: %v", ErrWatchFailed, err)
	}

	bindings, watchPaths, err := s.prepare(targets)
	if err != nil {
		_ = watcher.Close()
		return nil, nil, err
	}

	for watchPath := range watchPaths {
		if err := watcher.Add(watchPath); err != nil {
			_ = watcher.Close()
			return nil, nil, fmt.Errorf("%w: watch %q: %v", ErrWatchFailed, watchPath, err)
		}
	}

	events := make(chan Event)
	errs := make(chan error, 1)

	go func() {
		defer close(events)
		defer close(errs)
		defer watcher.Close()

		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				op := normalizeOperation(event.Op)
				if op == OperationUnknown {
					continue
				}
				if s.Recursive && (op == OperationCreate || op == OperationRename) {
					if err := addRecursiveWatchDirs(watcher, event.Name, bindings, watchPaths); err != nil {
						select {
						case errs <- err:
						case <-ctx.Done():
						}
						return
					}
				}
				for _, binding := range matchingBindings(event.Name, bindings) {
					select {
					case events <- NewEvent(cleanPath(event.Name), op, binding.watchRoot):
					case <-ctx.Done():
						return
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				select {
				case errs <- fmt.Errorf("%w: %v", ErrWatchFailed, err):
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return events, errs, nil
}

func (s *FilesystemEventSource) prepare(targets []WatchTarget) ([]watchBinding, map[string]struct{}, error) {
	var bindings []watchBinding
	watchPaths := make(map[string]struct{})

	for _, target := range targets {
		targetPath := strings.TrimSpace(target.Path)
		if targetPath == "" {
			return nil, nil, fmt.Errorf("%w: watch target path is empty", ErrWatchFailed)
		}

		absTarget, err := filepath.Abs(targetPath)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: resolve %q: %v", ErrWatchFailed, target.Path, err)
		}
		info, err := os.Stat(absTarget)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: stat %q: %v", ErrWatchFailed, target.Path, err)
		}

		binding := watchBinding{
			targetPath: cleanPath(absTarget),
			watchRoot:  cleanPath(absTarget),
			isDir:      info.IsDir(),
		}
		bindings = append(bindings, binding)

		if info.IsDir() {
			watchPaths[cleanPath(absTarget)] = struct{}{}
			if s.Recursive {
				if err := filepath.WalkDir(absTarget, func(path string, d os.DirEntry, err error) error {
					if err != nil {
						return err
					}
					if d.IsDir() {
						watchPaths[cleanPath(path)] = struct{}{}
					}
					return nil
				}); err != nil {
					return nil, nil, fmt.Errorf("%w: walk %q: %v", ErrWatchFailed, target.Path, err)
				}
			}
			continue
		}

		watchPaths[cleanPath(filepath.Dir(absTarget))] = struct{}{}
	}

	return bindings, watchPaths, nil
}

func addRecursiveWatchDirs(watcher *fsnotify.Watcher, path string, bindings []watchBinding, watched map[string]struct{}) error {
	clean := cleanPath(path)
	if !matchesDirectoryBinding(clean, bindings) {
		return nil
	}

	info, err := os.Stat(clean)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("%w: stat %q: %v", ErrWatchFailed, path, err)
	}
	if !info.IsDir() {
		return nil
	}

	return filepath.WalkDir(clean, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("%w: walk %q: %v", ErrWatchFailed, path, err)
		}
		if !d.IsDir() {
			return nil
		}
		dir := cleanPath(path)
		if _, ok := watched[dir]; ok {
			return nil
		}
		if err := watcher.Add(dir); err != nil {
			return fmt.Errorf("%w: watch %q: %v", ErrWatchFailed, dir, err)
		}
		watched[dir] = struct{}{}
		return nil
	})
}

func matchesDirectoryBinding(path string, bindings []watchBinding) bool {
	for _, binding := range bindings {
		if !binding.isDir {
			continue
		}
		if path == binding.targetPath || strings.HasPrefix(path, binding.targetPath+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

func matchingBindings(path string, bindings []watchBinding) []watchBinding {
	clean := cleanPath(path)
	matches := make([]watchBinding, 0, 1)
	for _, binding := range bindings {
		if binding.isDir {
			if clean == binding.targetPath || strings.HasPrefix(clean, binding.targetPath+string(os.PathSeparator)) {
				matches = append(matches, binding)
			}
			continue
		}
		if clean == binding.targetPath {
			matches = append(matches, binding)
		}
	}
	return matches
}

func normalizeOperation(op fsnotify.Op) Operation {
	switch {
	case op&fsnotify.Create == fsnotify.Create:
		return OperationCreate
	case op&fsnotify.Write == fsnotify.Write:
		return OperationWrite
	case op&fsnotify.Remove == fsnotify.Remove:
		return OperationRemove
	case op&fsnotify.Rename == fsnotify.Rename:
		return OperationRename
	case op&fsnotify.Chmod == fsnotify.Chmod:
		return OperationChmod
	default:
		return OperationUnknown
	}
}

func cleanPath(path string) string {
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(abs)
}
