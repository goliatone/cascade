package cliui

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"
)

func TestProgressPlainOutputHasNoANSI(t *testing.T) {
	var out bytes.Buffer
	interactive := false
	progress := NewProgress(&out, Options{Interactive: &interactive})
	task := progress.Start("Resolving dependencies")
	task.Success("Resolved dependencies")

	output := out.String()
	if strings.Contains(output, "\x1b[") {
		t.Fatalf("plain output contains ANSI escapes: %q", output)
	}
	for _, want := range []string{"→ Resolving dependencies", "✓ Resolved dependencies"} {
		if !strings.Contains(output, want) {
			t.Fatalf("plain output missing %q: %q", want, output)
		}
	}
}

func TestProgressInteractiveOutputUsesSpinnerAndColor(t *testing.T) {
	withoutNoColor(t)
	var out bytes.Buffer
	interactive := true
	progress := NewProgress(&out, Options{Interactive: &interactive, Interval: time.Millisecond})
	task := progress.Start("Applying updates")
	time.Sleep(3 * time.Millisecond)
	task.Success("Applied updates")

	output := out.String()
	if !strings.Contains(output, "\r\x1b[2K") || !strings.Contains(output, "\x1b[") {
		t.Fatalf("interactive output missing spinner controls/color: %q", output)
	}
}

func withoutNoColor(t *testing.T) {
	t.Helper()
	value, existed := os.LookupEnv("NO_COLOR")
	if err := os.Unsetenv("NO_COLOR"); err != nil {
		t.Fatalf("unset NO_COLOR: %v", err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv("NO_COLOR", value)
		} else {
			_ = os.Unsetenv("NO_COLOR")
		}
	})
}

func TestProgressNoColorDisablesANSIColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var out bytes.Buffer
	interactive := true
	progress := NewProgress(&out, Options{Interactive: &interactive})
	task := progress.Start("Applying updates")
	task.Success("Applied updates")

	output := out.String()
	withoutClear := strings.ReplaceAll(output, "\r\x1b[2K", "")
	if strings.Contains(withoutClear, "\x1b[") {
		t.Fatalf("NO_COLOR output contains color escapes: %q", output)
	}
}

func TestProgressQuietSuppressesOutput(t *testing.T) {
	var out bytes.Buffer
	progress := NewProgress(&out, Options{Quiet: true})
	task := progress.Start("Applying updates")
	task.Fail("Apply failed")
	progress.Info("detail")
	progress.Warn("warning")
	if out.Len() != 0 {
		t.Fatalf("quiet progress wrote output: %q", out.String())
	}
}

func TestProgressVerboseDisablesAnimation(t *testing.T) {
	var out bytes.Buffer
	interactive := true
	message := strings.TrimSpace(strings.Repeat("Applying detailed updates ", 4))
	progress := NewProgress(&out, Options{Interactive: &interactive, Verbose: true, Width: 20})
	task := progress.Start(message)
	task.Success(message)
	if strings.Contains(out.String(), "\r\x1b[2K") {
		t.Fatalf("verbose progress used animation: %q", out.String())
	}
	if strings.Count(out.String(), message) != 2 {
		t.Fatalf("verbose progress truncated detail: %q", out.String())
	}
}

func TestProgressInteractiveOutputFitsTerminalWidth(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var out bytes.Buffer
	interactive := true
	const width = 40
	progress := NewProgress(&out, Options{
		Interactive: &interactive,
		Interval:    time.Millisecond,
		Width:       width,
	})
	task := progress.Start(strings.Repeat("long-module-path/", 8) + " updating dependency")
	time.Sleep(3 * time.Millisecond)
	task.Success(strings.Repeat("long-module-path/", 8) + " updated dependency")

	for _, rendered := range strings.Split(out.String(), "\r\x1b[2K") {
		rendered = strings.TrimSuffix(rendered, "\n")
		if rendered == "" {
			continue
		}
		if got := runeCount(rendered); got >= width {
			t.Fatalf("rendered progress width = %d, want less than %d: %q", got, width, rendered)
		}
	}
}

func TestFitProgressPartsDropsDurationBeforeOverTruncatingMessage(t *testing.T) {
	message, suffix := fitProgressParts("Updating dependency", " (12.3s)", 12)
	if suffix != "" {
		t.Fatalf("expected duration to be dropped, got %q", suffix)
	}
	if got := runeCount("✓ " + message); got >= 12 {
		t.Fatalf("fitted line width = %d, want less than 12: %q", got, message)
	}
}
