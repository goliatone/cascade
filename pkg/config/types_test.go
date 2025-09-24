package config_test

import (
	"testing"

	"github.com/goliatone/cascade/pkg/config"
)

func TestNewReturnsConfig(t *testing.T) {
	cfg := config.New()
	if cfg == nil {
		t.Fatal("expected config.New to return non-nil")
	}
}
