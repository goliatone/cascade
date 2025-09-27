package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/goliatone/cascade/pkg/config"
	gh "github.com/google/go-github/v66/github"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newMockGitHubClient(t *testing.T, handlerMap map[string]func(*http.Request) *http.Response) *gh.Client {
	t.Helper()

	transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		key := req.Method + " " + req.URL.Path
		handler, ok := handlerMap[key]
		if !ok {
			t.Fatalf("unexpected request %s", key)
		}
		return handler(req), nil
	})

	httpClient := &http.Client{Transport: transport}
	client := gh.NewClient(httpClient)

	base, err := url.Parse("https://example.com/")
	if err != nil {
		t.Fatalf("failed to parse base URL: %v", err)
	}
	client.BaseURL = base
	client.UploadURL = base

	return client
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestDiscoverGitHubDependents_ReturnsDependents(t *testing.T) {
	handlerMap := map[string]func(*http.Request) *http.Response{
		"GET /search/code": func(r *http.Request) *http.Response {
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse form: %v", err)
			}
			if !strings.Contains(r.Form.Get("q"), "github.com/target/module") {
				t.Fatalf("expected query to contain module path")
			}
			return jsonResponse(`{"total_count":1,"incomplete_results":false,"items":[{"path":"go.mod","name":"go.mod","repository":{"full_name":"testorg/repo","owner":{"login":"testorg"},"name":"repo"}}]}`)
		},
		"GET /repos/testorg/repo/contents/go.mod": func(r *http.Request) *http.Response {
			content := base64.StdEncoding.EncodeToString([]byte("module github.com/testorg/repo\n"))
			return jsonResponse(fmt.Sprintf(`{"type":"file","encoding":"base64","content":"%s"}`, content))
		},
	}

	client := newMockGitHubClient(t, handlerMap)

	deps, err := discoverGitHubDependentsWithClient(context.Background(), client, "github.com/target/module", "testorg", nil, nil, nil)
	if err != nil {
		t.Fatalf("discoverGitHubDependentsWithClient returned error: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("expected 1 dependent, got %d", len(deps))
	}
	dep := deps[0]
	if dep.Repository != "testorg/repo" {
		t.Errorf("expected repository testorg/repo, got %s", dep.Repository)
	}
	if dep.ModulePath != "github.com/testorg/repo" {
		t.Errorf("expected module path github.com/testorg/repo, got %s", dep.ModulePath)
	}
	if dep.LocalModulePath != "." {
		t.Errorf("expected local module path '.', got %s", dep.LocalModulePath)
	}
}

func TestDiscoverGitHubDependents_MissingToken(t *testing.T) {
	cfg := &config.Config{}
	if _, err := discoverGitHubDependents(context.Background(), "github.com/test/module", "testorg", nil, nil, cfg, nil); err == nil {
		t.Fatalf("expected error when token missing")
	}
}

func TestMatchesRepoPatterns(t *testing.T) {
	include := []string{"*service*"}
	exclude := []string{"*-internal"}

	if matchesRepoPatterns("example/service-api", include, exclude) == false {
		t.Fatalf("expected repo to match include pattern")
	}
	if matchesRepoPatterns("example/service-internal", include, exclude) == true {
		t.Fatalf("expected repo to be excluded")
	}
	if matchesRepoPatterns("example/another", include, nil) == true {
		t.Fatalf("expected repo to be excluded when include patterns set")
	}
}
