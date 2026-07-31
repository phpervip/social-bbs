package repository

import "context"

// UserRepo accesses the users table.
type UserRepo interface {
	// GetByID returns the user or ErrUserNotFound.
	GetByID(ctx context.Context, id int64) (*User, error)
	// ListIDs returns all user ids (used by the fanout worker for global push).
	ListIDs(ctx context.Context) ([]int64, error)
}

// PostRepo accesses the posts table (joined with users).
type PostRepo interface {
	// Create inserts a post and returns the joined read model.
	Create(ctx context.Context, user *User, content, mediaURL string) (*Post, error)
	// GetByID returns a live post or ErrPostNotFound.
	GetByID(ctx context.Context, id int64) (*Post, error)
	// GetByIDs returns live posts for the given ids; order is not guaranteed.
	GetByIDs(ctx context.Context, ids []int64) ([]*Post, error)
	// Latest returns the newest posts across all users, optionally strictly
	// older than cursor (unix ms, 0 = no bound). Includes soft-deleted filtering.
	Latest(ctx context.Context, cursor int64, limit int) ([]*Post, error)
	// Search returns posts whose content LIKE %query%, newest first.
	Search(ctx context.Context, query string, cursor int64, limit int) ([]*Post, error)
	// SoftDelete marks a post deleted (GORM deleted_at). Returns ErrPostNotFound
	// when the post does not exist or is already deleted.
	SoftDelete(ctx context.Context, id int64) error
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
