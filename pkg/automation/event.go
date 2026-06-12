package automation

import "time"

func NewEvent(path string, op Operation, watchRoot string) Event {
	return Event{
		Path:      path,
		Op:        op,
		WatchRoot: watchRoot,
		Time:      time.Now(),
	}
}
