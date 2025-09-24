package broker

import "testing"

func TestRenderTitle(t *testing.T) {
	tests := []struct {
		name     string
		template string
		module   string
		version  string
		expected string
	}{
		{"default", "", "github.com/org/mod", "v1.0.0", "Update dependencies"},
		{"with placeholders", "chore: bump {{ module }} to {{ version }}", "github.com/org/mod", "v1.0.0", "chore: bump github.com/org/mod to v1.0.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RenderTitle(tt.template, tt.module, tt.version); got != tt.expected {
				t.Fatalf("RenderTitle() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestRenderBody(t *testing.T) {
	ctx := map[string]string{"module": "github.com/org/mod", "version": "v1.0.0"}
	got := RenderBody("Update {{ module }} to {{ version }}", ctx)
	want := "Update github.com/org/mod to v1.0.0"
	if got != want {
		t.Fatalf("RenderBody() = %q, want %q", got, want)
	}
}
