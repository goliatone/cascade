package executor

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/mod/modfile"
)

// detectDependencyVersion returns the version information for a module declared in go.mod.
// It returns the version (if found), a boolean indicating whether the module declaration
// was detected, and any read/parse error encountered while inspecting go.mod.
func detectDependencyVersion(moduleDir, module string) (string, bool, error) {
	goModPath := filepath.Join(moduleDir, "go.mod")

	data, err := os.ReadFile(goModPath)
	if err != nil {
		return "", false, fmt.Errorf("read go.mod: %w", err)
	}

	file, err := modfile.Parse(goModPath, data, nil)
	if err != nil {
		return "", false, fmt.Errorf("parse go.mod: %w", err)
	}

	for _, req := range file.Require {
		if req.Mod.Path == module {
			return req.Mod.Version, true, nil
		}
	}

	for _, rep := range file.Replace {
		if rep.Old.Path == module {
			if rep.New.Version != "" {
				return rep.New.Version, true, nil
			}
			return rep.New.Path, true, nil
		}
	}

	return "", false, nil
}

func captureOldDependencyVersion(impact *DependencyImpact, moduleDir string) {
	if impact == nil || impact.Module == "" {
		return
	}

	version, detected, err := detectDependencyVersion(moduleDir, impact.Module)
	if err != nil {
		impact.Notes = append(impact.Notes, fmt.Sprintf("before update: %v", err))
		return
	}

	impact.OldVersionDetected = detected
	if detected {
		impact.OldVersion = version
	} else {
		impact.OldVersion = ""
	}
}

func captureNewDependencyVersion(impact *DependencyImpact, moduleDir, phase string) {
	if impact == nil || impact.Module == "" {
		return
	}

	version, detected, err := detectDependencyVersion(moduleDir, impact.Module)
	if err != nil {
		impact.Notes = append(impact.Notes, fmt.Sprintf("%s: %v", phase, err))
		return
	}

	impact.NewVersionDetected = detected
	if detected {
		impact.NewVersion = version
	} else {
		impact.NewVersion = ""
	}
	impact.Applied = impact.NewVersionDetected && impact.NewVersion != impact.OldVersion
}
