package repository

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type gormOutboxRepo struct {
	db *gorm.DB
}

// NewOutboxRepo returns a GORM-backed OutboxRepo.
func NewOutboxRepo(db *gorm.DB) OutboxRepo { return &gormOutboxRepo{db: db} }

// CreateInTx inserts a pending event inside the caller's transaction (design
// §5.1: post insert + outbox insert commit atomically).
func (r *gormOutboxRepo) CreateInTx(ctx context.Context, tx *gorm.DB, topic string, payload []byte) error {
	return tx.WithContext(ctx).Exec(
		"INSERT INTO outbox_events (topic, payload, status, retry_count, created_at) VALUES (?, ?, 'pending', 0, ?)",
		topic, string(payload), time.Now(),
	).Error
}

func (r *gormOutboxRepo) ClaimPending(ctx context.Context, limit int) ([]OutboxEvent, error) {
	var rows []OutboxEvent
	err := r.db.WithContext(ctx).
		Where("status = ?", "pending").
		Order("id").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}

func (r *gormOutboxRepo) ClaimStale(ctx context.Context, before time.Time, limit int) ([]OutboxEvent, error) {
	var rows []OutboxEvent
	err := r.db.WithContext(ctx).
		Where("status = ? AND created_at < ?", "pending", before).
		Order("id").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}

func (r *gormOutboxRepo) ClaimFailedRetryable(ctx context.Context, limit int) ([]OutboxEvent, error) {
	var rows []OutboxEvent
	err := r.db.WithContext(ctx).
		Where("status = ? AND retry_count < ?", "failed", OutboxMaxRetries).
		Order("id").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}

func (r *gormOutboxRepo) MarkDelivered(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Exec("UPDATE outbox_events SET status = 'delivered' WHERE id = ?", id).Error
}

// IncrementRetry bumps retry_count and atomically flips the status to failed
// once the retry budget (3) is exhausted (design §5.2).
func (r *gormOutboxRepo) IncrementRetry(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Exec(
		"UPDATE outbox_events SET retry_count = retry_count + 1, status = IF(retry_count + 1 >= ?, 'failed', 'pending') WHERE id = ?",
		OutboxMaxRetries, id,
	).Error
}

func (r *gormOutboxRepo) MarkFailed(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Exec("UPDATE outbox_events SET status = 'failed' WHERE id = ?", id).Error
}
