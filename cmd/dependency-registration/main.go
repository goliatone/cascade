package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/goliatone/cascade/internal/depregistration"
	depconfig "github.com/goliatone/cascade/internal/depregistration/config"
)

type stringList []string

func (s *stringList) String() string {
	return strings.Join(*s, ",")
}

func (s *stringList) Set(value string) error {
	*s = append(*s, value)
	return nil
}

type detectionSummary struct {
	BaseRef       string                            `json:"base_ref"`
	HeadRef       string                            `json:"head_ref"`
	BaseBranch    string                            `json:"base_branch,omitempty"`
	GeneratedAt   time.Time                         `json:"generated_at"`
	Dependencies  []depregistration.DependencyDelta `json:"dependencies"`
	DryRun        bool                              `json:"dry_run"`
	Workflow      string                            `json:"workflow,omitempty"`
	DefaultBranch string                            `json:"default_branch,omitempty"`
}

func main() {
	log.SetFlags(0)

	var (
		baseRef    = flag.String("base", "", "git reference serving as diff base (required)")
		headRef    = flag.String("head", "HEAD", "git reference serving as diff head")
		baseBranch = flag.String("base-branch", "", "git branch associated with the base ref (optional)")
		configPath = flag.String("config", "", "optional dependency-registration config file")
		outputPath = flag.String("output", "dependency-registration.json", "path to write detection summary")
	)

	var skipPatterns stringList
	flag.Var(&skipPatterns, "skip-pattern", "regex pattern to skip modules (repeatable)")

	flag.Parse()

	if *baseRef == "" {
		logFatal("missing required --base ref")
	}

	ctx := context.Background()
	root := repoRoot()

	cfgPath := *configPath
	if cfgPath != "" {
		if err := ensureConfigReadable(cfgPath); err != nil {
			logFatal(err.Error())
		}
	} else {
		cfgPath = filepath.Join(root, ".github", "dependency-registration.yml")
	}

	cfg, err := depconfig.Load(cfgPath)
	if err != nil {
		logFatal(fmt.Sprintf("load config: %v", err))
	}

	fetcher := depregistration.NewGitDiffFetcher(root)
	opts := []depregistration.DiffDetectorOption{}
	patterns := append([]string{}, cfg.SkipPatterns...)
	patterns = append(patterns, skipPatterns...)
	if len(patterns) > 0 {
		opts = append(opts, depregistration.WithSkipPatterns(patterns...))
	}

	detector := depregistration.NewDiffDetector(fetcher, opts...)

	deltas, err := detector.Detect(ctx, *baseRef, *headRef)
	if err != nil {
		logFatal(fmt.Sprintf("detect dependencies: %v", err))
	}

	summary := detectionSummary{
		BaseRef:       *baseRef,
		HeadRef:       *headRef,
		BaseBranch:    *baseBranch,
		GeneratedAt:   time.Now().UTC(),
		Dependencies:  deltas,
		DryRun:        cfg.DryRun,
		Workflow:      cfg.Workflow,
		DefaultBranch: cfg.DefaultBranch,
	}

	if err := writeSummary(*outputPath, summary); err != nil {
		logFatal(fmt.Sprintf("write summary: %v", err))
	}

	printSummary(summary)
}

func ensureConfigReadable(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("config file %q not found", path)
		}
		return fmt.Errorf("config file %q: %w", path, err)
	}
	if fi.IsDir() {
		return fmt.Errorf("config path %q is a directory", path)
	}
	return nil
}

func repoRoot() string {
	root, err := os.Getwd()
	if err != nil {
		return "."
	}
	return root
}

func writeSummary(path string, summary detectionSummary) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil && !errors.Is(err, os.ErrExist) && filepath.Dir(path) != "." {
		return err
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(summary)
}

func printSummary(summary detectionSummary) {
	if len(summary.Dependencies) == 0 {
		fmt.Println("No new internal dependencies detected.")
		return
	}

	fmt.Printf("Detected %d new internal dependencies between %s and %s\n", len(summary.Dependencies), summary.BaseRef, summary.HeadRef)
	for _, dep := range summary.Dependencies {
		fmt.Printf("- %s %s (%s)\n", dep.Module, dep.Version, dep.GoModPath)
	}
}

func logFatal(msg string) {
	log.Printf("error: %s", msg)
	os.Exit(1)
}
