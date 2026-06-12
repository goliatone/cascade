package automation

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
)

type SchedulerOptions struct {
	AllowConcurrent bool
}

type Scheduler struct {
	executor        Executor
	exec            ExecSpec
	allowConcurrent bool

	seq uint64

	mu      sync.Mutex
	running bool
	pending *Event
	wg      sync.WaitGroup
}

func NewScheduler(executor Executor, exec ExecSpec, opts SchedulerOptions) *Scheduler {
	return &Scheduler{
		executor:        executor,
		exec:            exec,
		allowConcurrent: opts.AllowConcurrent,
	}
}

func (s *Scheduler) Submit(ctx context.Context, event Event) {
	if ctx.Err() != nil {
		return
	}
	if s.allowConcurrent {
		s.start(ctx, event, false)
		return
	}

	s.mu.Lock()
	if s.running {
		s.pending = cloneEvent(event)
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	s.start(ctx, event, true)
}

func (s *Scheduler) Wait() {
	s.wg.Wait()
}

func (s *Scheduler) start(ctx context.Context, event Event, serial bool) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if serial {
			s.runSerial(ctx, event)
			return
		}
		s.execute(ctx, event)
	}()
}

func (s *Scheduler) runSerial(ctx context.Context, event Event) {
	current := event
	for {
		if ctx.Err() == nil {
			s.execute(ctx, current)
		}

		s.mu.Lock()
		if ctx.Err() != nil || s.pending == nil {
			s.pending = nil
			s.running = false
			s.mu.Unlock()
			return
		}
		current = *s.pending
		s.pending = nil
		s.mu.Unlock()
	}
}

func (s *Scheduler) execute(ctx context.Context, event Event) {
	if s.executor == nil {
		return
	}
	req := RunRequest{
		ID:    s.nextRunID(),
		Event: event,
		Exec:  s.exec,
	}
	if _, err := s.executor.Execute(ctx, req); err != nil && req.Exec.Stderr != nil {
		fmt.Fprintf(req.Exec.Stderr, "automation: %v\n", err)
	}
}

func (s *Scheduler) nextRunID() string {
	id := atomic.AddUint64(&s.seq, 1)
	return fmt.Sprintf("run-%d", id)
}

func cloneEvent(event Event) *Event {
	return &Event{
		Path:      event.Path,
		Op:        event.Op,
		WatchRoot: event.WatchRoot,
		Time:      event.Time,
	}
}
