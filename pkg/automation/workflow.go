package automation

import (
	"strings"
)

func (w Workflow) WithDefaults() Workflow {
	if w.Debounce == 0 {
		w.Debounce = DefaultDebounce
	}
	return w
}

func (w Workflow) Validate() error {
	var problems []string

	if len(w.Targets) == 0 {
		problems = append(problems, "at least one watch target is required")
	}
	for i, target := range w.Targets {
		if strings.TrimSpace(target.Path) == "" {
			problems = append(problems, "watch target path at index "+itoa(i)+" is empty")
		}
	}
	if strings.TrimSpace(w.Exec.Command) == "" {
		problems = append(problems, "exec command is required")
	}
	if w.Debounce < 0 {
		problems = append(problems, "debounce must be greater than or equal to zero")
	}
	if w.Exec.Timeout < 0 {
		problems = append(problems, "timeout must be greater than or equal to zero")
	}

	return newValidationError(problems)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
