package localupdate

import (
	"reflect"
	"testing"
)

func TestNormalizeRequestDefaultsPrefix(t *testing.T) {
	req := NormalizeRequest(Request{})
	if !reflect.DeepEqual(req.Prefixes, []string{DefaultPrefix}) {
		t.Fatalf("expected default prefix, got %#v", req.Prefixes)
	}
}

func TestNormalizeRequestSplitsCommaSeparatedValues(t *testing.T) {
	req := NormalizeRequest(Request{
		Prefixes: []string{"github.com/goliatone/, github.com/acme/", "github.com/acme/"},
		Only:     []string{"github.com/goliatone/a,github.com/goliatone/b"},
		Exclude:  []string{"github.com/goliatone/c"},
	})

	if !reflect.DeepEqual(req.Prefixes, []string{"github.com/goliatone/", "github.com/acme/"}) {
		t.Fatalf("unexpected prefixes: %#v", req.Prefixes)
	}
	if !reflect.DeepEqual(req.Only, []string{"github.com/goliatone/a", "github.com/goliatone/b"}) {
		t.Fatalf("unexpected only values: %#v", req.Only)
	}
	if !reflect.DeepEqual(req.Exclude, []string{"github.com/goliatone/c"}) {
		t.Fatalf("unexpected exclude values: %#v", req.Exclude)
	}
}

func TestPlanUpdatesReturnsOnlyUpdateItems(t *testing.T) {
	plan := Plan{Items: []Item{
		{Module: "github.com/goliatone/a", Status: StatusCurrent},
		{Module: "github.com/goliatone/b", Status: StatusUpdate, NeedsUpdate: true},
	}}

	updates := plan.Updates()
	if len(updates) != 1 || updates[0].Module != "github.com/goliatone/b" {
		t.Fatalf("unexpected updates: %#v", updates)
	}
}
