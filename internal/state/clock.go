package state

import "time"

type systemClock struct{}

func (systemClock) Now() time.Time {
	return time.Now().UTC()
}

type nopLogger struct{}

func (nopLogger) Debug(string, ...any) {}

func (nopLogger) Info(string, ...any) {}

func (nopLogger) Error(string, ...any) {}
