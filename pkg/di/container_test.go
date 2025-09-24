package di_test

import (
	"testing"

	"github.com/goliatone/cascade/pkg/config"
	"github.com/goliatone/cascade/pkg/di"
)

func TestNew_ReturnsErrorUntilImplemented(t *testing.T) {
	_, err := di.New(di.WithConfig(&config.Config{}))
	if err == nil {
		t.Fatal("expected not implemented error")
	}
}
