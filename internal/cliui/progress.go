package cliui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type Options struct {
	Interactive *bool
	Quiet       bool
	Verbose     bool
	Interval    time.Duration
	Width       int
}

type Progress struct {
	out           io.Writer
	interactive   bool
	color         bool
	quiet         bool
	interval      time.Duration
	width         int
	fallbackWidth int
	terminal      *os.File
	mu            sync.Mutex
}

func NewProgress(out io.Writer, opts Options) *Progress {
	if out == nil {
		out = io.Discard
	}
	terminalFile, detectedTerminal := terminalWriter(out)
	interactive := detectedTerminal
	forcedInteractive := opts.Interactive != nil
	if opts.Interactive != nil {
		interactive = *opts.Interactive
	}
	if opts.Verbose {
		interactive = false
	}
	interval := opts.Interval
	if interval <= 0 {
		interval = 80 * time.Millisecond
	}
	fallbackWidth := 0
	if interactive && opts.Width <= 0 && terminalFile != nil {
		width, _, err := term.GetSize(int(terminalFile.Fd()))
		if err != nil || width <= 0 {
			if !forcedInteractive {
				interactive = false
			}
		} else {
			fallbackWidth = width
		}
	}
	return &Progress{
		out:           out,
		interactive:   interactive,
		color:         interactive && colorEnabled(),
		quiet:         opts.Quiet,
		interval:      interval,
		width:         opts.Width,
		fallbackWidth: fallbackWidth,
		terminal:      terminalFile,
	}
}

func (p *Progress) Start(message string) *Task {
	task := &Task{
		progress: p,
		message:  strings.TrimSpace(message),
		started:  time.Now(),
		done:     make(chan struct{}),
	}
	if p == nil || p.quiet {
		task.silent = true
		return task
	}
	if !p.interactive {
		p.writeLine("→", task.message, colorCyan, 0)
		return task
	}
	task.wg.Add(1)
	p.renderFrame(0, task.message)
	go task.animate()
	return task
}

func (p *Progress) Info(message string) {
	if p == nil || p.quiet {
		return
	}
	p.writeLine("•", strings.TrimSpace(message), colorCyan, 0)
}

func (p *Progress) Warn(message string) {
	if p == nil || p.quiet {
		return
	}
	p.writeLine("!", strings.TrimSpace(message), colorYellow, 0)
}

func (p *Progress) renderFrame(index int, message string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	frame := paint(spinnerFrames[index%len(spinnerFrames)], colorCyan, p.color)
	message, _ = fitProgressParts(message, "", p.renderWidth())
	fmt.Fprintf(p.out, "\r\x1b[2K%s %s", frame, message)
}

func (p *Progress) writeLine(icon, message, color string, duration time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.interactive {
		fmt.Fprint(p.out, "\r\x1b[2K")
	}
	durationText := ""
	if duration > 0 {
		durationText = " (" + formatDuration(duration) + ")"
	}
	renderWidth := 0
	if p.interactive {
		renderWidth = p.renderWidth()
	}
	message, durationText = fitProgressParts(message, durationText, renderWidth)
	fmt.Fprintf(p.out, "%s %s", paint(icon, color, p.color), message)
	if durationText != "" {
		fmt.Fprint(p.out, paint(durationText, colorDim, p.color))
	}
	fmt.Fprintln(p.out)
}

func (p *Progress) renderWidth() int {
	if p == nil {
		return 0
	}
	if p.width > 0 {
		return p.width
	}
	if p.terminal != nil {
		if width, _, err := term.GetSize(int(p.terminal.Fd())); err == nil && width > 0 {
			return width
		}
	}
	return p.fallbackWidth
}

func fitProgressParts(message, suffix string, terminalWidth int) (string, string) {
	if terminalWidth <= 0 {
		return message, suffix
	}
	lineBudget := terminalWidth - 1
	const prefixWidth = 2
	if lineBudget <= prefixWidth {
		return "", ""
	}
	if prefixWidth+runeCount(message)+runeCount(suffix) <= lineBudget {
		return message, suffix
	}
	messageBudget := lineBudget - prefixWidth - runeCount(suffix)
	if messageBudget < 4 {
		suffix = ""
		messageBudget = lineBudget - prefixWidth
	}
	return truncateRunes(message, messageBudget), suffix
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	if limit == 1 {
		return "…"
	}
	return string(runes[:limit-1]) + "…"
}

func runeCount(value string) int {
	return len([]rune(value))
}

type Task struct {
	progress *Progress
	message  string
	started  time.Time
	done     chan struct{}
	wg       sync.WaitGroup
	once     sync.Once
	silent   bool
}

func (t *Task) animate() {
	defer t.wg.Done()
	ticker := time.NewTicker(t.progress.interval)
	defer ticker.Stop()
	frame := 1
	for {
		select {
		case <-ticker.C:
			t.progress.renderFrame(frame, t.message)
			frame++
		case <-t.done:
			return
		}
	}
}

func (t *Task) Success(message string) {
	t.finish("✓", message, colorGreen)
}

func (t *Task) Fail(message string) {
	t.finish("✗", message, colorRed)
}

func (t *Task) Warn(message string) {
	t.finish("!", message, colorYellow)
}

func (t *Task) finish(icon, message, color string) {
	if t == nil {
		return
	}
	t.once.Do(func() {
		if !t.silent && t.progress.interactive {
			close(t.done)
			t.wg.Wait()
		}
		if t.silent {
			return
		}
		message = strings.TrimSpace(message)
		if message == "" {
			message = t.message
		}
		t.progress.writeLine(icon, message, color, time.Since(t.started))
	})
}

func formatDuration(duration time.Duration) string {
	if duration < time.Millisecond {
		return "<1ms"
	}
	if duration < time.Second {
		return duration.Round(time.Millisecond).String()
	}
	return duration.Round(100 * time.Millisecond).String()
}

func isTerminal(out io.Writer) bool {
	_, ok := terminalWriter(out)
	return ok
}

func terminalWriter(out io.Writer) (*os.File, bool) {
	file, ok := out.(*os.File)
	if !ok {
		return nil, false
	}
	if !term.IsTerminal(int(file.Fd())) {
		return nil, false
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("TERM")), "dumb") {
		return nil, false
	}
	return file, true
}
