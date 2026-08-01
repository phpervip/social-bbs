package worker

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"social-bbs/feed-service/internal/repository"
)

// Publisher publishes one message to a Kafka topic (implemented by
// kafka.Client; faked in tests).
type Publisher interface {
	Publish(ctx context.Context, topic string, key, value []byte) error
}

// Dispatcher is the resident outbox worker: it polls pending outbox rows and
// publishes them to Kafka, marking delivered on ack or retrying on failure
// (design §5.2, R1 — outbox → Kafka, never direct sends).
type Dispatcher struct {
	outbox    repository.OutboxRepo
	publisher Publisher
	logger    *slog.Logger
}

// NewDispatcher wires the outbox dispatcher.
func NewDispatcher(outbox repository.OutboxRepo, publisher Publisher, logger *slog.Logger) *Dispatcher {
	return &Dispatcher{outbox: outbox, publisher: publisher, logger: logger}
}

// Run polls pending rows until ctx is cancelled. The loop stays hot while rows
// exist; on empty or error it backs off OutboxPollInterval.
func (d *Dispatcher) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		events, err := d.outbox.ClaimPending(ctx, repository.OutboxBatchSize)
		if err != nil {
			d.logger.Error("outbox claim failed", "error", err)
			sleepCtx(ctx, repository.OutboxPollInterval)
			continue
		}
		for _, ev := range events {
			if ctx.Err() != nil {
				return
			}
			if err := d.Dispatch(ctx, ev); err != nil {
				d.logger.Error("outbox dispatch failed", "id", ev.ID, "topic", ev.Topic, "error", err)
			}
		}
		if len(events) == 0 {
			sleepCtx(ctx, repository.OutboxPollInterval)
		}
	}
}

// RunCompensation re-dispatches stuck pending rows (created before
// OutboxStalePending) and failed rows that still have retry budget, on an
// OutboxCompInterval ticker (design §5.2 — covers a crashed/blocked
// dispatcher). Rows past the retry budget stay failed and are logged.
func (d *Dispatcher) RunCompensation(ctx context.Context) {
	ticker := time.NewTicker(repository.OutboxCompInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			stale, err := d.outbox.ClaimStale(ctx, time.Now().Add(-repository.OutboxStalePending), repository.OutboxBatchSize)
			if err != nil {
				d.logger.Error("outbox compensation stale claim failed", "error", err)
			}
			for _, ev := range stale {
				d.redispatch(ctx, ev)
			}
			retryable, err := d.outbox.ClaimFailedRetryable(ctx, repository.OutboxBatchSize)
			if err != nil {
				d.logger.Error("outbox compensation failed claim failed", "error", err)
			}
			for _, ev := range retryable {
				d.redispatch(ctx, ev)
			}
		}
	}
}

// redispatch publishes a compensation row with the same semantics as Dispatch.
func (d *Dispatcher) redispatch(ctx context.Context, ev repository.OutboxEvent) {
	if err := d.Dispatch(ctx, ev); err != nil {
		d.logger.Warn("outbox compensation redispatch failed", "id", ev.ID, "topic", ev.Topic, "error", err)
	}
}

// Dispatch publishes one event: on success marks delivered; on failure
// increments the retry counter (which flips the row to failed at >= 3).
func (d *Dispatcher) Dispatch(ctx context.Context, ev repository.OutboxEvent) error {
	key := []byte(strconv.FormatInt(ev.ID, 10))
	if err := d.publisher.Publish(ctx, ev.Topic, key, ev.Payload); err != nil {
		_ = d.outbox.IncrementRetry(ctx, ev.ID)
		return err
	}
	return d.outbox.MarkDelivered(ctx, ev.ID)
}

// sleepCtx sleeps for d unless ctx is cancelled first.
func sleepCtx(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}
