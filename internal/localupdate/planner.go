package localupdate

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/goliatone/cascade/internal/planner"
	"github.com/goliatone/cascade/pkg/workspace"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
)

type WorkspaceResolution struct {
	Path          string
	CacheFallback bool
}

type WorkspaceResolver func(req Request, currentModule, moduleDir string) (WorkspaceResolution, error)

type Planner struct {
	ResolveWorkspace WorkspaceResolver
}

func NewPlanner() *Planner {
	return &Planner{ResolveWorkspace: defaultWorkspaceResolver}
}

func PlanLocal(req Request) (Plan, error) {
	return NewPlanner().Plan(req)
}

func (p *Planner) Plan(req Request) (Plan, error) {
	req = NormalizeRequest(req)
	if strings.TrimSpace(req.ModuleDir) == "" {
		return Plan{}, fmt.Errorf("current module directory is required")
	}

	goModPath := filepath.Join(req.ModuleDir, "go.mod")
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return Plan{}, fmt.Errorf("read current go.mod: %w", err)
	}
	file, err := modfile.Parse(goModPath, data, nil)
	if err != nil {
		return Plan{}, fmt.Errorf("parse current go.mod: %w", err)
	}

	currentModule := strings.TrimSpace(req.CurrentModule)
	if currentModule == "" && file.Module != nil {
		currentModule = file.Module.Mod.Path
	}
	if currentModule == "" {
		return Plan{}, fmt.Errorf("current module path is required")
	}

	resolver := p.ResolveWorkspace
	if resolver == nil {
		resolver = defaultWorkspaceResolver
	}
	resolved, err := resolver(req, currentModule, req.ModuleDir)
	if err != nil {
		return Plan{}, err
	}
	if !req.WorkspaceExplicit && resolved.CacheFallback {
		return Plan{}, fmt.Errorf("local workspace could not be detected; pass --workspace to the local command")
	}
	if strings.TrimSpace(resolved.Path) == "" {
		return Plan{}, fmt.Errorf("local workspace could not be detected; pass --workspace to the local command")
	}

	replaces := replaceMap(file)
	items := make([]Item, 0)
	requires := append([]*modfile.Require(nil), file.Require...)
	sort.Slice(requires, func(i, j int) bool {
		return requires[i].Mod.Path < requires[j].Mod.Path
	})

	for _, require := range requires {
		modulePath := require.Mod.Path
		if !matchesAnyPrefix(modulePath, req.Prefixes) {
			continue
		}
		if len(req.Only) > 0 && !matchesFilter(modulePath, req.Only) {
			continue
		}

		item := Item{
			Module:         modulePath,
			CurrentVersion: require.Mod.Version,
			Indirect:       require.Indirect,
		}

		if matchesFilter(modulePath, req.Exclude) {
			item.Status = StatusSkippedFilter
			item.Reason = "excluded by filter"
			items = append(items, item)
			continue
		}
		if require.Indirect && !req.IncludeIndirect {
			item.Status = StatusSkippedIndirect
			item.Reason = "indirect dependency skipped by default"
			items = append(items, item)
			continue
		}
		if replacement, ok := replaces[modulePath]; ok {
			item.Status = StatusSkippedReplace
			item.Reason = replaceReason(replacement)
			items = append(items, item)
			continue
		}

		localModule, err := resolveLocalModule(resolved.Path, modulePath)
		if err != nil {
			if moduleErr, ok := err.(localModuleError); ok {
				item.Status = moduleErr.Status
				item.Reason = moduleErr.Reason
			} else {
				item.Status = StatusMissingLocalRepo
				item.Reason = err.Error()
			}
			items = append(items, item)
			continue
		}
		item.LocalPath = localModule.ModuleDir

		localVersion, err := readLocalVersion(localModule)
		if err != nil {
			item.Status = StatusMissingVersionFile
			item.Reason = err.Error()
			items = append(items, item)
			continue
		}
		item.LocalVersion = localVersion

		if err := module.Check(modulePath, localVersion); err != nil {
			item.Status = StatusInvalidVersion
			item.Reason = fmt.Sprintf("invalid local version %q: %v", localVersion, err)
			items = append(items, item)
			continue
		}

		needsUpdate, err := planner.CompareVersions(item.CurrentVersion, localVersion)
		if err != nil {
			item.Status = StatusComparisonFailed
			item.Reason = err.Error()
			items = append(items, item)
			continue
		}
		if needsUpdate {
			item.Status = StatusUpdate
			item.NeedsUpdate = true
			item.Reason = "local sibling version is newer"
		} else {
			item.Status = StatusCurrent
			item.Reason = "current version is up to date"
		}
		items = append(items, item)
	}

	return Plan{
		CurrentModule: currentModule,
		ModuleDir:     req.ModuleDir,
		Workspace:     resolved.Path,
		Items:         items,
	}, nil
}

func defaultWorkspaceResolver(req Request, currentModule, moduleDir string) (WorkspaceResolution, error) {
	if strings.TrimSpace(req.Workspace) != "" {
		resolved := req.Workspace
		if !filepath.IsAbs(resolved) {
			abs, err := filepath.Abs(resolved)
			if err != nil {
				return WorkspaceResolution{}, fmt.Errorf("resolve workspace path: %w", err)
			}
			resolved = abs
		}
		return WorkspaceResolution{Path: resolved}, nil
	}

	resolved := workspace.Resolve("", nil, currentModule, moduleDir)
	return WorkspaceResolution{
		Path:          resolved,
		CacheFallback: samePath(resolved, defaultCacheWorkspace()),
	}, nil
}

func defaultCacheWorkspace() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".cache", "cascade")
	}
	return ""
}

func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	if errA == nil {
		a = absA
	}
	if errB == nil {
		b = absB
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func replaceMap(file *modfile.File) map[string]modfile.Replace {
	out := map[string]modfile.Replace{}
	for _, replacement := range file.Replace {
		out[replacement.Old.Path] = *replacement
	}
	return out
}

func replaceReason(replacement modfile.Replace) string {
	target := replacement.New.Path
	if replacement.New.Version != "" {
		target = target + "@" + replacement.New.Version
	}
	if target == "" {
		target = replacement.Old.Path
	}
	return "dependency has replace directive to " + target
}

func matchesAnyPrefix(modulePath string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(modulePath, prefix) {
			return true
		}
	}
	return false
}

func matchesFilter(modulePath string, filters []string) bool {
	if len(filters) == 0 {
		return false
	}
	base := path.Base(modulePath)
	for _, filter := range filters {
		if modulePath == filter || base == filter {
			return true
		}
	}
	return false
}
