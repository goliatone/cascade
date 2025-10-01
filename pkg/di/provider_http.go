package di

import (
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/goliatone/cascade/pkg/config"
)

const (
	defaultUserAgent = "cascade-cli (+https://github.com/goliatone/cascade)"
	defaultAccept    = "application/json"
)

// provideHTTPClient creates a default HTTP client implementation.
// Configured with reasonable defaults for API calls and timeouts.
func provideHTTPClient() *http.Client {
	return &http.Client{
		Transport: newHeaderRoundTripper(nil, defaultHTTPHeaders(nil)),
	}
}

// provideHTTPClientWithConfig creates an HTTP client with configuration-driven timeouts.
// Respects executor timeout settings and sets appropriate user agent.
func provideHTTPClientWithConfig(cfg *config.Config) *http.Client {
	if cfg == nil {
		return provideHTTPClient()
	}

	// Use executor timeout as base for HTTP timeout, with reasonable default
	timeout := 30 * time.Second // Default timeout
	if cfg.Executor.Timeout > 0 {
		// Use 80% of executor timeout to leave buffer for retries
		timeout = time.Duration(float64(cfg.Executor.Timeout) * 0.8)
		if timeout < 10*time.Second {
			timeout = 10 * time.Second // Minimum timeout
		}
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: newHeaderRoundTripper(nil, defaultHTTPHeaders(cfg)),
	}
}

func defaultHTTPHeaders(cfg *config.Config) http.Header {
	headers := make(http.Header)
	userAgent := buildUserAgent(cfg)
	headers.Set("User-Agent", userAgent)
	headers.Set("Accept", defaultAccept)
	return headers
}

func buildUserAgent(cfg *config.Config) string {
	userAgent := defaultUserAgent
	if cfg != nil {
		if org := strings.TrimSpace(cfg.Integration.GitHub.Organization); org != "" {
			userAgent = fmt.Sprintf("%s org/%s", defaultUserAgent, org)
		}
	}
	return fmt.Sprintf("%s go/%s %s/%s", userAgent, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}

type headerRoundTripper struct {
	base    http.RoundTripper
	headers http.Header
}

func newHeaderRoundTripper(base http.RoundTripper, headers http.Header) http.RoundTripper {
	if headers == nil {
		headers = make(http.Header)
	}
	var underlying http.RoundTripper = http.DefaultTransport
	if base != nil {
		underlying = base
	} else if transport, ok := http.DefaultTransport.(*http.Transport); ok {
		underlying = transport.Clone()
	}
	return &headerRoundTripper{base: underlying, headers: headers}
}

func (h *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if h == nil {
		return http.DefaultTransport.RoundTrip(req)
	}
	clone := req.Clone(req.Context())
	for key, values := range h.headers {
		if clone.Header.Get(key) != "" {
			continue
		}
		for _, value := range values {
			clone.Header.Add(key, value)
		}
	}
	return h.base.RoundTrip(clone)
}
