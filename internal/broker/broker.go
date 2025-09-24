package broker

import (
	"context"

	"github.com/goliatone/cascade/internal/executor"
	"github.com/goliatone/cascade/internal/planner"
)

// New returns a stub broker implementation.
func New() Broker {
	return &broker{}
}

type broker struct{}

func (b *broker) EnsurePR(ctx context.Context, item planner.WorkItem, result *executor.Result) (*PullRequest, error) {
	return nil, &NotImplementedError{Operation: "broker.EnsurePR"}
}

func (b *broker) Comment(ctx context.Context, pr *PullRequest, body string) error {
	return &NotImplementedError{Operation: "broker.Comment"}
}

func (b *broker) Notify(ctx context.Context, item planner.WorkItem, result *executor.Result) error {
	return &NotImplementedError{Operation: "broker.Notify"}
}
