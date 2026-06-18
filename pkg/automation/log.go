package automation

import (
	"fmt"
	"io"
	"sync"
)

type textLogger struct {
	mu sync.Mutex
	w  io.Writer
}

func NewTextLogger(w io.Writer) Logger {
	if w == nil {
		return nil
	}
	return &textLogger{w: w}
}

func (l *textLogger) Printf(format string, args ...any) {
	if l == nil || l.w == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Fprintf(l.w, "automation: "+format+"\n", args...)
}

func logf(logger Logger, format string, args ...any) {
	if logger == nil {
		return
	}
	logger.Printf(format, args...)
}
