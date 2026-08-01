package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// TestOutboxCreateInTx: the INSERT runs inside the caller's transaction and
// always lands in the 'pending' state with retry_count 0 (design §5.1).
func TestOutboxCreateInTx(t *testing.T) {
	gdb, mock := newMockDB(t)
	repo := NewOutboxRepo(gdb)

	mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO outbox_events (topic, payload, status, retry_count, created_at) VALUES (?, ?, 'pending', 0, ?)")).
		WithArgs("post.created", `{"post_id":7}`, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.CreateInTx(context.Background(), gdb, "post.created", []byte(`{"post_id":7}`))
	if err != nil {
		t.Fatalf("CreateInTx: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestOutboxClaimPending: only status='pending' rows come back, oldest first.
func TestOutboxClaimPending(t *testing.T) {
	gdb, mock := newMockDB(t)
	repo := NewOutboxRepo(gdb)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `outbox_events` WHERE status = ? ORDER BY id LIMIT ?")).
		WithArgs("pending", 50).
		WillReturnRows(sqlmock.NewRows([]string{"id", "topic", "payload", "status", "retry_count", "created_at"}).
			AddRow(1, "post.created", `{"post_id":7}`, "pending", 0, time.Now()))

	rows, err := repo.ClaimPending(context.Background(), 50)
	if err != nil {
		t.Fatalf("ClaimPending: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != 1 || rows[0].Topic != "post.created" || rows[0].Status != "pending" {
		t.Fatalf("rows = %+v, want one pending row id=1", rows)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestOutboxClaimStale: compensation claims pending rows older than the cutoff.
func TestOutboxClaimStale(t *testing.T) {
	gdb, mock := newMockDB(t)
	repo := NewOutboxRepo(gdb)

	cutoff := time.Now().Add(-30 * time.Second)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `outbox_events` WHERE status = ? AND created_at < ? ORDER BY id LIMIT ?")).
		WithArgs("pending", cutoff, 50).
		WillReturnRows(sqlmock.NewRows([]string{"id", "topic", "payload", "status", "retry_count", "created_at"}))

	rows, err := repo.ClaimStale(context.Background(), cutoff, 50)
	if err != nil {
		t.Fatalf("ClaimStale: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("rows = %+v, want none", rows)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestOutboxClaimFailedRetryable: failed rows with retry budget left.
func TestOutboxClaimFailedRetryable(t *testing.T) {
	gdb, mock := newMockDB(t)
	repo := NewOutboxRepo(gdb)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `outbox_events` WHERE status = ? AND retry_count < ? ORDER BY id LIMIT ?")).
		WithArgs("failed", OutboxMaxRetries, 50).
		WillReturnRows(sqlmock.NewRows([]string{"id", "topic", "payload", "status", "retry_count", "created_at"}).
			AddRow(2, "post.created", `{"post_id":8}`, "failed", 1, time.Now()))

	rows, err := repo.ClaimFailedRetryable(context.Background(), 50)
	if err != nil {
		t.Fatalf("ClaimFailedRetryable: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != 2 || rows[0].RetryCount != 1 {
		t.Fatalf("rows = %+v, want failed row id=2 retry=1", rows)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestOutboxMarkDelivered: success path flips the row to delivered.
func TestOutboxMarkDelivered(t *testing.T) {
	gdb, mock := newMockDB(t)
	repo := NewOutboxRepo(gdb)

	mock.ExpectExec(regexp.QuoteMeta("UPDATE outbox_events SET status = 'delivered' WHERE id = ?")).
		WithArgs(1).WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.MarkDelivered(context.Background(), 1); err != nil {
		t.Fatalf("MarkDelivered: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestOutboxIncrementRetryBelowBudget: retry 1/2 keeps the row pending.
func TestOutboxIncrementRetryBelowBudget(t *testing.T) {
	gdb, mock := newMockDB(t)
	repo := NewOutboxRepo(gdb)

	mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE outbox_events SET retry_count = retry_count + 1, status = IF(retry_count + 1 >= ?, 'failed', 'pending') WHERE id = ?")).
		WithArgs(OutboxMaxRetries, 1).WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.IncrementRetry(context.Background(), 1); err != nil {
		t.Fatalf("IncrementRetry: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestOutboxMarkFailed: terminal failure path.
func TestOutboxMarkFailed(t *testing.T) {
	gdb, mock := newMockDB(t)
	repo := NewOutboxRepo(gdb)

	mock.ExpectExec(regexp.QuoteMeta("UPDATE outbox_events SET status = 'failed' WHERE id = ?")).
		WithArgs(2).WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.MarkFailed(context.Background(), 2); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
