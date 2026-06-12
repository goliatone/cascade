package automation

import (
	"context"
	"time"
)

type Runner struct {
	workflow Workflow
	source   EventSource
	executor Executor
}

func NewRunner(workflow Workflow, opts RunnerOptions) (*Runner, error) {
	workflow = workflow.WithDefaults()
	if err := workflow.Validate(); err != nil {
		return nil, err
	}
	source := opts.Source
	if source == nil {
		source = &FilesystemEventSource{Recursive: workflow.Recursive}
	}
	executor := opts.Executor
	if executor == nil {
		executor = NewShellExecutor()
	}

	return &Runner{
		workflow: workflow,
		source:   source,
		executor: executor,
	}, nil
}

func (r *Runner) Run(ctx context.Context) error {
	events, errs, err := r.source.Watch(ctx, r.workflow.Targets)
	if err != nil {
		return err
	}

	scheduler := NewScheduler(r.executor, r.workflow.Exec, SchedulerOptions{
		AllowConcurrent: r.workflow.AllowConcurrent,
	})
	defer scheduler.Wait()

	var (
		timer   *time.Timer
		timerC  <-chan time.Time
		pending Event
	)

	stopTimer := func() {
		if timer == nil {
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timerC = nil
	}
	defer stopTimer()

	submit := func(event Event) {
		if ctx.Err() != nil {
			return
		}
		scheduler.Submit(ctx, event)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case err, ok := <-errs:
			if !ok {
				errs = nil
				continue
			}
			return err
		case event, ok := <-events:
			if !ok {
				if timerC != nil {
					stopTimer()
					submit(pending)
				}
				events = nil
				if errs == nil {
					return nil
				}
				continue
			}
			if r.workflow.Debounce == 0 {
				submit(event)
				continue
			}
			pending = event
			if timer == nil {
				timer = time.NewTimer(r.workflow.Debounce)
			} else {
				stopTimer()
				timer.Reset(r.workflow.Debounce)
			}
			timerC = timer.C
		case <-timerC:
			timerC = nil
			submit(pending)
		}
	}
}
