package repository

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// newMockDB builds a GORM handle over sqlmock so repository logic can be unit
// tested without a live MySQL (brief §5).
func newMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	gdb, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	return gdb, mock
}

const lockPostSQL = "SELECT id, like_count FROM posts WHERE id = ? AND deleted_at IS NULL FOR UPDATE"

// TestLikePostIncrementsCount: a fresh like inserts the row and bumps the count.
func TestLikePostIncrementsCount(t *testing.T) {
	gdb, mock := newMockDB(t)
	repo := NewLikeRepo(gdb)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(lockPostSQL)).WithArgs(10).
		WillReturnRows(sqlmock.NewRows([]string{"id", "like_count"}).AddRow(10, 3))
	mock.ExpectExec(regexp.QuoteMeta("INSERT IGNORE INTO post_likes (post_id, user_id, created_at) VALUES (?, ?, ?)")).
		WithArgs(10, 1, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE posts SET like_count = like_count + 1 WHERE id = ? AND deleted_at IS NULL")).
		WithArgs(10).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	newCount, err := repo.Like(context.Background(), 10, 1)
	if err != nil {
		t.Fatalf("Like: %v", err)
	}
	if newCount != 4 {
		t.Errorf("newCount = %d, want 4", newCount)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestLikeIdempotency: INSERT IGNORE affecting 0 rows (already liked) must NOT
// increment the count and must still succeed.
func TestLikeIdempotency(t *testing.T) {
	gdb, mock := newMockDB(t)
	repo := NewLikeRepo(gdb)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(lockPostSQL)).WithArgs(10).
		WillReturnRows(sqlmock.NewRows([]string{"id", "like_count"}).AddRow(10, 5))
	mock.ExpectExec(regexp.QuoteMeta("INSERT IGNORE INTO post_likes (post_id, user_id, created_at) VALUES (?, ?, ?)")).
		WithArgs(10, 1, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	newCount, err := repo.Like(context.Background(), 10, 1)
	if err != nil {
		t.Fatalf("Like on already-liked post: %v", err)
	}
	if newCount != 5 {
		t.Errorf("newCount = %d, want 5 (unchanged for idempotent like)", newCount)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestUnlikeFloorNeverNegative: unliking a post with like_count == 0 must issue
// the GREATEST(like_count - 1, 0) guard and stay at 0.
func TestUnlikeFloorNeverNegative(t *testing.T) {
	gdb, mock := newMockDB(t)
	repo := NewLikeRepo(gdb)

	const floorUpdate = "UPDATE posts SET like_count = GREATEST(like_count - 1, 0) WHERE id = ?"

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(lockPostSQL)).WithArgs(10).
		WillReturnRows(sqlmock.NewRows([]string{"id", "like_count"}).AddRow(10, 0))
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM post_likes WHERE post_id = ? AND user_id = ?")).
		WithArgs(10, 1).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(floorUpdate)).
		WithArgs(10).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	newCount, err := repo.Unlike(context.Background(), 10, 1)
	if err != nil {
		t.Fatalf("Unlike: %v", err)
	}
	if newCount != 0 {
		t.Errorf("newCount = %d, want 0 (like_count must never go below 0)", newCount)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestUnlikeIdempotency: DELETE affecting 0 rows (not liked) must not change
// the count and must still succeed.
func TestUnlikeIdempotency(t *testing.T) {
	gdb, mock := newMockDB(t)
	repo := NewLikeRepo(gdb)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(lockPostSQL)).WithArgs(10).
		WillReturnRows(sqlmock.NewRows([]string{"id", "like_count"}).AddRow(10, 2))
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM post_likes WHERE post_id = ? AND user_id = ?")).
		WithArgs(10, 1).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	newCount, err := repo.Unlike(context.Background(), 10, 1)
	if err != nil {
		t.Fatalf("Unlike on not-liked post: %v", err)
	}
	if newCount != 2 {
		t.Errorf("newCount = %d, want 2 (unchanged for idempotent unlike)", newCount)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestLikeMissingPost: no posts row → ErrPostNotFound (maps to NOT_FOUND).
func TestLikeMissingPost(t *testing.T) {
	gdb, mock := newMockDB(t)
	repo := NewLikeRepo(gdb)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(lockPostSQL)).WithArgs(99).
		WillReturnRows(sqlmock.NewRows([]string{"id", "like_count"}))
	mock.ExpectRollback()

	_, err := repo.Like(context.Background(), 99, 1)
	if !errors.Is(err, ErrPostNotFound) {
		t.Errorf("Like on missing post: err = %v, want ErrPostNotFound", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestClampLikeDelta pins the pure floor helper used for the returned count.
func TestClampLikeDelta(t *testing.T) {
	cases := []struct {
		current, delta, want int64
	}{
		{current: 0, delta: -1, want: 0},
		{current: 5, delta: -1, want: 4},
		{current: 0, delta: 0, want: 0},
		{current: 1, delta: 1, want: 2},
	}
	for _, c := range cases {
		if got := clampLikeDelta(c.current, c.delta); got != c.want {
			t.Errorf("clampLikeDelta(%d, %d) = %d, want %d", c.current, c.delta, got, c.want)
		}
	}
}
