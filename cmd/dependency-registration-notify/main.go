package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"

	github "github.com/google/go-github/v66/github"

	"github.com/goliatone/cascade/internal/depregistration"
	"github.com/goliatone/cascade/internal/depregistration/commenter"
	"github.com/goliatone/cascade/internal/depregistration/notifier"
)

type detectionSummary struct {
	BaseRef      string                            `json:"base_ref"`
	HeadRef      string                            `json:"head_ref"`
	GeneratedAt  time.Time                         `json:"generated_at"`
	Dependencies []depregistration.DependencyDelta `json:"dependencies"`
}

func main() {
	log.SetFlags(0)

	var (
		summaryPath = flag.String("summary", "", "path to dependency-registration.json (required)")
		runURL      = flag.String("run-url", "", "optional workflow run URL for comment")
		prNumber    = flag.String("pr", "", "pull request number to comment on")
		dryRun      = flag.Bool("dry-run", false, "skip GitHub mutations")
	)

	flag.Parse()

	if *summaryPath == "" {
		fatal("missing --summary path")
	}

	summary, err := loadSummary(*summaryPath)
	if err != nil {
		fatal(fmt.Sprintf("load summary: %v", err))
	}

	repo := os.Getenv("GITHUB_REPOSITORY")
	if repo == "" {
		fatal("GITHUB_REPOSITORY not set")
	}

	token := os.Getenv("GITHUB_TOKEN")
	if token == "" && !*dryRun {
		fatal("GITHUB_TOKEN not provided")
	}

	ctx := context.Background()
	mlog := log.New(os.Stdout, "", 0)

	var ghClient *github.Client
	if token != "" {
		ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
		tc := oauth2.NewClient(ctx, ts)
		ghClient = github.NewClient(tc)
	} else {
		ghClient = github.NewClient(nil)
	}

	notifierClient := notifier.NewGoGitHubClient(ghClient)
	notifierSvc := notifier.NewGitHubNotifier(notifierClient)

	commenterClient := commenter.NewGoGitHubClient(ghClient)
	commenterSvc := commenter.New(commenterClient)

	results := make([]notifier.ActionResult, 0, len(summary.Dependencies))

	for _, dep := range summary.Dependencies {
		req := notifier.Request{
			Dependency:      dep,
			ConsumingRepo:   repo,
			ConsumingModule: "",
			DependencyRepo:  deriveRepo(dep.Module),
			WorkflowInputs: map[string]string{
				"base_ref": summary.BaseRef,
				"head_ref": summary.HeadRef,
			},
			DryRun:     *dryRun,
			CommentTag: commenterTag(),
		}

		res, err := notifierSvc.Notify(ctx, req)
		if err != nil {
			mlog.Printf("warn: notify %s: %v", dep.Module, err)
			results = append(results, notifier.ActionResult{
				Dependency: dep,
				Action:     notifier.ActionIssueCreated,
				Notes:      fmt.Sprintf("notification failed: %v", err),
			})
			continue
		}

		results = append(results, *res)
	}

	if len(results) == 0 {
		results = append(results, notifier.ActionResult{
			Dependency: depregistration.DependencyDelta{},
			Action:     notifier.ActionDryRun,
			Notes:      "no new dependencies detected",
		})
	}

	if *prNumber != "" {
		num, err := strconv.Atoi(strings.TrimSpace(*prNumber))
		if err != nil {
			mlog.Printf("warn: invalid PR number %q: %v", *prNumber, err)
		} else if num > 0 {
			_, err = commenterSvc.Upsert(ctx, commenter.Request{
				Repository: repo,
				PRNumber:   num,
				Results:    results,
				RunURL:     *runURL,
				Now:        time.Now,
			})
			if err != nil {
				mlog.Printf("warn: comment update failed: %v", err)
			}
		}
	}
}

func loadSummary(path string) (*detectionSummary, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var summary detectionSummary
	if err := json.NewDecoder(file).Decode(&summary); err != nil {
		return nil, err
	}
	return &summary, nil
}

func deriveRepo(module string) string {
	const prefix = "github.com/"
	module = strings.TrimSpace(module)
	if strings.HasPrefix(module, prefix) {
		module = strings.TrimPrefix(module, prefix)
	}
	parts := strings.Split(module, "/")
	if len(parts) >= 2 {
		return strings.Join(parts[:2], "/")
	}
	return module
}

func commenterTag() string {
	return "dependency-registration"
}

func fatal(msg string) {
	log.Printf("error: %s", msg)
	os.Exit(1)
}
