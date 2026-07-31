package repository

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// User is the P1 seed users table (plan §3.1, D3 — replaced by User Service in P2).
type User struct {
	ID          int64     `gorm:"primaryKey"`
	Username    string    `gorm:"size:64;uniqueIndex"`
	DisplayName string    `gorm:"size:64"`
	AvatarURL   string    `gorm:"size:255"`
	CreatedAt   time.Time
}

// PostRow is the GORM model for the posts table (plan §3.1).
type PostRow struct {
	ID           int64          `gorm:"primaryKey"`
	UserID       int64          `gorm:"index:idx_posts_user"`
	Content      string         `gorm:"type:text"`
	MediaURL     string         `gorm:"size:500;default:''"`
	LikeCount    int64          `gorm:"not null;default:0"`
	CommentCount int64          `gorm:"not null;default:0"`
	CreatedAt    time.Time      `gorm:"index:idx_posts_created"`
	DeletedAt    gorm.DeletedAt `gorm:"index"`
}

// PostLike is a like row; the composite PK makes INSERT IGNORE idempotent.
type PostLike struct {
	PostID    int64     `gorm:"primaryKey"`
	UserID    int64     `gorm:"primaryKey"`
	CreatedAt time.Time
	Post      PostRow   `gorm:"foreignKey:PostID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
}

// PostComment is a comment on a post.
type PostComment struct {
	ID        int64     `gorm:"primaryKey"`
	PostID    int64     `gorm:"index:idx_comments_post"`
	UserID    int64     `gorm:"index:idx_comments_user"`
	Content   string    `gorm:"size:500"`
	CreatedAt time.Time `gorm:"index:idx_comments_created"`
	Post      PostRow   `gorm:"foreignKey:PostID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
}

// OutboxEvent is the P2 outbox table. P1 only creates the model, never writes it
// (plan §3.1 / brief MUST NOT DO).
type OutboxEvent struct {
	ID         int64          `gorm:"primaryKey"`
	Topic      string         `gorm:"size:64"`
	Payload    datatypes.JSON
	Status     string         `gorm:"size:16;index;default:pending"`
	RetryCount int            `gorm:"not null;default:0"`
	CreatedAt  time.Time
}
