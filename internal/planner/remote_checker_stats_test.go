package planner

import (
	"bytes"
	"testing"
)

func TestRemoteDependencyChecker_GetCacheStats(t *testing.T) {
	// Create a checker with a cache
	opts := CheckOptions{
		CacheEnabled:   true,
		CacheTTL:       300000000000, // 5 minutes
		ParallelChecks: 1,
		Timeout:        30000000000, // 30 seconds
	}

	checker := NewRemoteDependencyChecker(opts, nil).(*remoteDependencyChecker)

	// Initially, stats should be zero
	stats := checker.GetCacheStats()
	if stats.Hits != 0 || stats.Misses != 0 || stats.Size != 0 {
		t.Errorf("initial stats should be zero, got hits=%d, misses=%d, size=%d",
			stats.Hits, stats.Misses, stats.Size)
	}

	// Add some cache entries
	deps1 := map[string]string{
		"github.com/example/module": "v1.2.3",
	}
	checker.cache.Set("https://github.com/user/repo1.git", "main", deps1)

	deps2 := map[string]string{
		"github.com/example/module": "v2.0.0",
	}
	checker.cache.Set("https://github.com/user/repo2.git", "main", deps2)

	// Check that size increased
	stats = checker.GetCacheStats()
	if stats.Size != 2 {
		t.Errorf("expected size=2 after adding 2 entries, got %d", stats.Size)
	}

	// Perform some cache operations to generate hits and misses
	checker.cache.Get("https://github.com/user/repo1.git", "main", "github.com/example/module") // hit
	checker.cache.Get("https://github.com/user/repo2.git", "main", "github.com/example/module") // hit
	checker.cache.Get("https://github.com/user/repo3.git", "main", "github.com/example/module") // miss

	// Check stats reflect the operations
	stats = checker.GetCacheStats()
	if stats.Hits != 2 {
		t.Errorf("expected hits=2, got %d", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("expected misses=1, got %d", stats.Misses)
	}
	if stats.Size != 2 {
		t.Errorf("expected size=2, got %d", stats.Size)
	}
}

func TestRemoteDependencyChecker_LogCacheStats(t *testing.T) {
	// Create a spy logger to capture log output
	var logBuf bytes.Buffer
	logger := &spyLogger{buf: &logBuf}

	opts := CheckOptions{
		CacheEnabled:   true,
		CacheTTL:       300000000000, // 5 minutes
		ParallelChecks: 1,
		Timeout:        30000000000, // 30 seconds
	}

	checker := NewRemoteDependencyChecker(opts, logger).(*remoteDependencyChecker)

	// Add cache entries and operations
	deps := map[string]string{"github.com/example/module": "v1.2.3"}
	checker.cache.Set("https://github.com/user/repo.git", "main", deps)
	checker.cache.Get("https://github.com/user/repo.git", "main", "github.com/example/module") // hit
	checker.cache.Get("https://github.com/user/repo.git", "main", "missing")                   // miss

	// Log the stats
	checker.LogCacheStats()

	output := logBuf.String()

	// Verify the log contains expected information
	expectedFields := []string{
		"cache statistics",
		"hits",
		"misses",
		"size",
		"hit_rate",
	}

	for _, field := range expectedFields {
		if !bytes.Contains([]byte(output), []byte(field)) {
			t.Errorf("expected log to contain %q, got: %s", field, output)
		}
	}
}

func TestRemoteDependencyChecker_LogCacheStats_WithoutLogger(t *testing.T) {
	// Create checker without logger - should not panic
	opts := CheckOptions{
		CacheEnabled:   true,
		CacheTTL:       300000000000,
		ParallelChecks: 1,
		Timeout:        30000000000,
	}

	checker := NewRemoteDependencyChecker(opts, nil).(*remoteDependencyChecker)

	// This should not panic
	checker.LogCacheStats()
}

func TestRemoteDependencyChecker_LogCacheStats_HitRate(t *testing.T) {
	tests := []struct {
		name      string
		hits      int
		misses    int
		wantRate  string
	}{
		{
			name:     "100% hit rate",
			hits:     10,
			misses:   0,
			wantRate: "1", // 1.0
		},
		{
			name:     "0% hit rate",
			hits:     0,
			misses:   10,
			wantRate: "0", // 0.0
		},
		{
			name:     "50% hit rate",
			hits:     5,
			misses:   5,
			wantRate: "0.5",
		},
		{
			name:     "empty cache",
			hits:     0,
			misses:   0,
			wantRate: "0", // Division by zero handled
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logBuf bytes.Buffer
			logger := &spyLogger{buf: &logBuf}

			opts := CheckOptions{
				CacheEnabled:   true,
				CacheTTL:       300000000000,
				ParallelChecks: 1,
				Timeout:        30000000000,
			}

			checker := NewRemoteDependencyChecker(opts, logger).(*remoteDependencyChecker)

			// Simulate hits and misses by manually setting cache stats
			for i := 0; i < tt.hits; i++ {
				checker.cache.Get("url", "ref", "exists-"+string(rune(i))) // will miss
			}
			// Add an entry to get hits
			if tt.hits > 0 {
				deps := map[string]string{}
				for i := 0; i < tt.hits; i++ {
					deps["module"+string(rune(i))] = "v1.0.0"
				}
				checker.cache.Set("url", "ref", deps)
				// Reset and perform actual hits
				checker.cache.Clear()
				checker.cache.Set("url", "ref", deps)
				for i := 0; i < tt.hits; i++ {
					checker.cache.Get("url", "ref", "module"+string(rune(i)))
				}
			}
			for i := 0; i < tt.misses; i++ {
				checker.cache.Get("other-url", "ref", "missing")
			}

			checker.LogCacheStats()
			output := logBuf.String()

			if !bytes.Contains([]byte(output), []byte(tt.wantRate)) {
				t.Errorf("expected hit_rate to contain %q, got: %s", tt.wantRate, output)
			}
		})
	}
}

// spyLogger is a test logger that captures log output
type spyLogger struct {
	buf *bytes.Buffer
}

func (s *spyLogger) Info(msg string, keysAndValues ...interface{}) {
	s.buf.WriteString(msg)
	for i := 0; i < len(keysAndValues); i += 2 {
		if i+1 < len(keysAndValues) {
			s.buf.WriteString(" ")
			s.buf.WriteString(keysAndValues[i].(string))
			s.buf.WriteString("=")
			switch v := keysAndValues[i+1].(type) {
			case string:
				s.buf.WriteString(v)
			case int:
				s.buf.WriteString(string(rune(v + '0')))
			case int64:
				s.buf.WriteString(string(rune(v + '0')))
			case float64:
				// Simple float formatting
				s.buf.WriteString(string(rune(int(v) + '0')))
				if v != float64(int(v)) {
					s.buf.WriteString(".")
					frac := v - float64(int(v))
					s.buf.WriteString(string(rune(int(frac*10) + '0')))
				}
			}
		}
	}
	s.buf.WriteString("\n")
}

func (s *spyLogger) Debug(msg string, keysAndValues ...interface{}) {
	s.Info(msg, keysAndValues...)
}

func (s *spyLogger) Warn(msg string, keysAndValues ...interface{}) {
	s.Info(msg, keysAndValues...)
}

func (s *spyLogger) Error(msg string, keysAndValues ...interface{}) {
	s.Info(msg, keysAndValues...)
}