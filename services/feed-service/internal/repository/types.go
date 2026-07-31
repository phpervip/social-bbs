package repository

import "time"

// Post is the joined read model (posts row + author) returned by the repository.
// LikedByViewer is per-request context and is NEVER persisted to the cache.
type Post struct {
	ID            int64     `json:"id"`
	UserID        int64     `json:"user_id"`
	Username      string    `json:"username"`
	DisplayName   string    `json:"display_name"`
	AvatarURL     string    `json:"avatar_url"`
	Content       string    `json:"content"`
	MediaURL      string    `json:"media_url"`
	LikeCount     int64     `json:"like_count"`
	CommentCount  int64     `json:"comment_count"`
	LikedByViewer bool      `json:"liked_by_viewer"`
	CreatedAt     time.Time `json:"created_at"`
}

// CreatedAtMs returns the API-facing unix millisecond timestamp.
func (p *Post) CreatedAtMs() int64 { return p.CreatedAt.UnixMilli() }

// Comment is the joined read model (comment row + author).
type Comment struct {
	ID          int64     `json:"id"`
	PostID      int64     `json:"post_id"`
	UserID      int64     `json:"user_id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	AvatarURL   string    `json:"avatar_url"`
	Content     string    `json:"content"`
	CreatedAt   time.Time `json:"created_at"`
}

// CreatedAtMs returns the API-facing unix millisecond timestamp.
func (c *Comment) CreatedAtMs() int64 { return c.CreatedAt.UnixMilli() }
