package cliui

import (
	"io"
	"os"
)

const (
	colorRed    = "31"
	colorGreen  = "32"
	colorYellow = "33"
	colorCyan   = "36"
	colorDim    = "90"
)

type Styler struct {
	enabled bool
}

func NewStyler(out io.Writer) Styler {
	return Styler{enabled: isTerminal(out) && colorEnabled()}
}

func (s Styler) Success(value string) string { return paint(value, colorGreen, s.enabled) }
func (s Styler) Error(value string) string   { return paint(value, colorRed, s.enabled) }
func (s Styler) Warning(value string) string { return paint(value, colorYellow, s.enabled) }
func (s Styler) Info(value string) string    { return paint(value, colorCyan, s.enabled) }
func (s Styler) Muted(value string) string   { return paint(value, colorDim, s.enabled) }

func colorEnabled() bool {
	_, disabled := os.LookupEnv("NO_COLOR")
	return !disabled
}

func paint(value, color string, enabled bool) string {
	if !enabled || value == "" {
		return value
	}
	return "\x1b[" + color + "m" + value + "\x1b[0m"
}
