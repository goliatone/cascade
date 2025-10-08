package depregistration

import (
	"bufio"
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// DiffDetector parses go.mod diffs to discover new internal dependencies.
type DiffDetector struct {
	fetcher   DiffFetcher
	skipExprs []*regexp.Regexp
	matchExpr *regexp.Regexp
}

// DiffDetectorOption configures optional behaviour for the detector.
type DiffDetectorOption func(*DiffDetector)

// WithSkipPatterns configures regex patterns that, when matched, cause modules
// to be ignored.
func WithSkipPatterns(patterns ...string) DiffDetectorOption {
	return func(d *DiffDetector) {
		for _, pattern := range patterns {
			re, err := regexp.Compile(pattern)
			if err != nil {
				continue
			}
			d.skipExprs = append(d.skipExprs, re)
		}
	}
}

// NewDiffDetector constructs a detector backed by the supplied diff fetcher.
func NewDiffDetector(fetcher DiffFetcher, opts ...DiffDetectorOption) Detector {
	dd := &DiffDetector{
		fetcher:   fetcher,
		matchExpr: regexp.MustCompile(`^(github\.com/goliatone/[^\s]+)`),
	}

	for _, opt := range opts {
		opt(dd)
	}

	return dd
}

// Detect implements the Detector interface.
func (d *DiffDetector) Detect(ctx context.Context, baseRef, headRef string) ([]DependencyDelta, error) {
	if d.fetcher == nil {
		return nil, fmt.Errorf("diff detector requires fetcher")
	}

	diff, err := d.fetcher.Diff(ctx, baseRef, headRef)
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(strings.NewReader(string(diff)))

	const maxTokenSize = 1024 * 1024
	scanner.Buffer(make([]byte, 0, 64*1024), maxTokenSize)

	currentPath := ""
	moduleSeen := make(map[string]struct{})
	var results []DependencyDelta

	diffHeader := regexp.MustCompile(`^diff --git a/(.+) b/(.+)$`)

	for scanner.Scan() {
		line := scanner.Text()

		if matches := diffHeader.FindStringSubmatch(line); len(matches) == 3 {
			// Track the target file the hunk applies to.
			currentPath = matches[2]
			continue
		}

		if currentPath == "" || !strings.HasSuffix(currentPath, "go.mod") {
			continue
		}

		if strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---") || strings.HasPrefix(line, "@@") {
			continue
		}

		if !strings.HasPrefix(line, "+") {
			continue
		}

		trimmed := strings.TrimSpace(strings.TrimPrefix(line, "+"))
		if trimmed == "" {
			continue
		}

		if strings.HasPrefix(trimmed, "replace ") {
			continue
		}

		if strings.HasPrefix(trimmed, "require (") || strings.HasPrefix(trimmed, "require(") {
			// The actual module line will appear in subsequent lines; skip.
			continue
		}

		if strings.HasPrefix(trimmed, "require ") {
			trimmed = strings.TrimPrefix(trimmed, "require ")
		}

		match := d.matchExpr.FindString(trimmed)
		if match == "" {
			continue
		}

		if d.shouldSkip(match) {
			continue
		}

		fields := strings.Fields(trimmed)
		if len(fields) < 2 {
			continue
		}

		modulePath := fields[0]
		version := fields[1]

		if idx := strings.Index(version, "//"); idx >= 0 {
			version = strings.TrimSpace(version[:idx])
		}

		key := fmt.Sprintf("%s@%s#%s", currentPath, modulePath, version)
		if _, ok := moduleSeen[key]; ok {
			continue
		}
		moduleSeen[key] = struct{}{}

		results = append(results, DependencyDelta{
			Module:    modulePath,
			Version:   version,
			GoModPath: currentPath,
			Change:    ChangeTypeAdded,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	sort.SliceStable(results, func(i, j int) bool {
		if results[i].GoModPath == results[j].GoModPath {
			return results[i].Module < results[j].Module
		}
		return results[i].GoModPath < results[j].GoModPath
	})

	if results == nil {
		results = []DependencyDelta{}
	}

	return results, nil
}

func (d *DiffDetector) shouldSkip(module string) bool {
	for _, re := range d.skipExprs {
		if re.MatchString(module) {
			return true
		}
	}
	return false
}
