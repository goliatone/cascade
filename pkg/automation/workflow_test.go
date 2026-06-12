package automation

import (
	"errors"
	"testing"
	"time"
)

func TestWorkflowWithDefaults(t *testing.T) {
	w := Workflow{
		Targets: []WatchTarget{{Path: ".version"}},
		Exec:    ExecSpec{Command: "echo changed"},
	}

	got := w.WithDefaults()
	if got.Debounce != DefaultDebounce {
		t.Fatalf("default debounce = %v, want %v", got.Debounce, DefaultDebounce)
	}
}

func TestWorkflowValidate(t *testing.T) {
	tests := []struct {
		name     string
		workflow Workflow
		wantErr  bool
	}{
		{
			name: "valid minimal workflow",
			workflow: Workflow{
				Targets:  []WatchTarget{{Path: ".version"}},
				Exec:     ExecSpec{Command: "echo changed"},
				Debounce: time.Millisecond,
			},
		},
		{
			name: "missing targets",
			workflow: Workflow{
				Exec: ExecSpec{Command: "echo changed"},
			},
			wantErr: true,
		},
		{
			name: "empty target path",
			workflow: Workflow{
				Targets: []WatchTarget{{Path: " "}},
				Exec:    ExecSpec{Command: "echo changed"},
			},
			wantErr: true,
		},
		{
			name: "empty command",
			workflow: Workflow{
				Targets: []WatchTarget{{Path: ".version"}},
			},
			wantErr: true,
		},
		{
			name: "negative debounce",
			workflow: Workflow{
				Targets:  []WatchTarget{{Path: ".version"}},
				Exec:     ExecSpec{Command: "echo changed"},
				Debounce: -time.Millisecond,
			},
			wantErr: true,
		},
		{
			name: "negative timeout",
			workflow: Workflow{
				Targets: []WatchTarget{{Path: ".version"}},
				Exec: ExecSpec{
					Command: "echo changed",
					Timeout: -time.Millisecond,
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.workflow.Validate()
			if tt.wantErr {
				if err == nil {
					t.Fatal("Validate() error = nil, want error")
				}
				if !errors.Is(err, ErrInvalidWorkflow) {
					t.Fatalf("Validate() error = %v, want ErrInvalidWorkflow", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}
