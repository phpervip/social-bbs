package repository

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// PostRepo accesses the posts table. Author info is NO LONGER joined from
// feed_db.users (P2 D-A8): posts come back without author fields and are
// enriched by the service layer via UserClient.
type PostRepo interface {
	// Create inserts a post inside the caller-provided transaction tx (nil =
	// no explicit tx) and returns the row. The service coordinates the same tx
	// across PostRepo.Create and OutboxRepo.CreateInTx so post + outbox event
	// commit atomically (design §5.1).
	Create(ctx context.Context, tx *gorm.DB, userID int64, content, mediaURL string) (*Post, error)
	// WithTx runs fn inside a transaction, committing on success. It is the
	// service-side seam for cross-repo atomic writes.
	WithTx(ctx context.Context, fn func(tx *gorm.DB) error) error
	// GetByID returns a live post or ErrPostNotFound.
	GetByID(ctx context.Context, id int64) (*Post, error)
	// GetByIDs returns live posts for the given ids; order is not guaranteed.
	GetByIDs(ctx context.Context, ids []int64) ([]*Post, error)
	// Latest returns the newest posts across all users, optionally strictly
	// older than cursor (unix ms, 0 = no bound). Includes soft-deleted filtering.
	Latest(ctx context.Context, cursor int64, limit int) ([]*Post, error)
	// LatestByAuthor returns authorID's newest posts (follow backfill, §5.4).
	LatestByAuthor(ctx context.Context, authorID, cursor int64, limit int) ([]*Post, error)
	// LatestByAuthors returns the newest posts by any of authorIDs (timeline
	// rebuild from the following ZSet, §5.5).
	LatestByAuthors(ctx context.Context, authorIDs []int64, limit int) ([]*Post, error)
	// Search returns posts whose content LIKE %query%, newest first.
	Search(ctx context.Context, query string, cursor int64, limit int) ([]*Post, error)
	// SoftDelete marks a post deleted (GORM deleted_at). Returns ErrPostNotFound
	// when the post does not exist or is already deleted.
	SoftDelete(ctx context.Context, id int64) error
}

// OutboxRepo is the transactional outbox (design §5.2 / §5.6, R1).
type OutboxRepo interface {
	// CreateInTx inserts a pending outbox row inside the caller's transaction.
	CreateInTx(ctx context.Context, tx *gorm.DB, topic string, payload []byte) error
	// ClaimPending returns up to limit pending rows, oldest first.
	ClaimPending(ctx context.Context, limit int) ([]OutboxEvent, error)
	// ClaimStale returns pending rows created before the cutoff (compensation
	// re-dispatch after a crashed/blocked dispatcher).
	ClaimStale(ctx context.Context, before time.Time, limit int) ([]OutboxEvent, error)
	// ClaimFailedRetryable returns failed rows that have not exhausted their
	// retry budget.
	ClaimFailedRetryable(ctx context.Context, limit int) ([]OutboxEvent, error)
	// MarkDelivered sets status=delivered.
	MarkDelivered(ctx context.Context, id int64) error
	// IncrementRetry bumps retry_count and flips status to failed at >=3.
	IncrementRetry(ctx context.Context, id int64) error
	// MarkFailed sets status=failed.
	MarkFailed(ctx context.Context, id int64) error
}

// UserClient talks to User Service (gRPC :9001) with Redis caching, replacing
// the P1 feed_db.users table (design §5.5, D-A8).
type UserClient interface {
	// GetProfile returns the user or ErrUserNotFound. Reads user:profile:{id}
	// first (10min TTL), on miss calls gRPC GetProfile and backfills the cache.
	GetProfile(ctx context.Context, id int64) (*User, error)
	// GetProfiles batch-fetches profiles, returning the resolvable subset; the
	// error is non-nil when any individual fetch failed.
	GetProfiles(ctx context.Context, ids []int64) (map[int64]*User, error)
	// GetFollowerIDs returns userID's follower ids. Reads user:followers:{id}
	// ZSet first (5min TTL); on miss calls gRPC GetFollowers and backfills.
	GetFollowerIDs(ctx context.Context, userID int64) ([]int64, error)
	// GetFollowingIDs returns userID's following ids (user:following:{id} ZSet,
	// gRPC GetFollowing fallback).
	GetFollowingIDs(ctx context.Context, userID int64) ([]int64, error)
	// Close releases the underlying gRPC connection (call once at shutdown).
	Close() error
}

// LikeRepo implements like/unlike with exact like_count maintenance.
type LikeRepo interface {
	// Like is idempotent (INSERT IGNORE): returns the new like_count.
	// ErrPostNotFound when the post row is missing or soft-deleted.
	Like(ctx context.Context, postID, userID int64) (int64, error)
	// Unlike is idempotent: returns the new like_count (never below 0).
	// ErrPostNotFound when the post row is missing or soft-deleted.
	Unlike(ctx context.Context, postID, userID int64) (int64, error)
	// LikedByUser returns the set of post ids liked by userID (for viewer context).
	LikedByUser(ctx context.Context, userID int64, postIDs []int64) (map[int64]bool, error)
}

// CommentRepo accesses the post_comments table (joined with users).
type CommentRepo interface {
	// Add inserts a comment and increments comment_count in the same tx.
	// Returns the joined read model or ErrPostNotFound.
	Add(ctx context.Context, postID, userID int64, content string) (*Comment, error)
	// ListByPost returns comments newest-first with created_at cursor pagination.
	ListByPost(ctx context.Context, postID, cursor int64, limit int) ([]*Comment, error)
}
