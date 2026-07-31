package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

type gormCommentRepo struct {
	db *gorm.DB
}

// NewCommentRepo returns a GORM-backed CommentRepo.
func NewCommentRepo(db *gorm.DB) CommentRepo { return &gormCommentRepo{db: db} }

func (r *gormCommentRepo) Add(ctx context.Context, postID, userID int64, content string) (*Comment, error) {
	created := Comment{}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var post PostRow
		if err := tx.Where("id = ?", postID).First(&post).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPostNotFound
			}
			return err
		}
		c := PostComment{PostID: postID, UserID: userID, Content: content}
		if err := tx.Create(&c).Error; err != nil {
			return err
		}
		if err := tx.Model(&PostRow{}).Where("id = ?", postID).
			Update("comment_count", post.CommentCount+1).Error; err != nil {
			return err
		}
		created.ID = c.ID
		created.PostID = postID
		created.UserID = userID
		created.Content = content
		created.CreatedAt = c.CreatedAt
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Join author info (plan §3.3 AddComment).
	var u User
	if err := r.db.WithContext(ctx).First(&u, userID).Error; err != nil {
		return nil, err
	}
	created.Username = u.Username
	created.DisplayName = u.DisplayName
	created.AvatarURL = u.AvatarURL
	return &created, nil
}

func (r *gormCommentRepo) ListByPost(ctx context.Context, postID, cursor int64, limit int) ([]*Comment, error) {
	q := r.db.WithContext(ctx).Where("post_id = ?", postID)
	if cursor > 0 {
		q = q.Where("created_at < ?", time.UnixMilli(cursor))
	}
	var rows []PostComment
	if err := q.Order("created_at DESC, id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}

	// Batch-join authors.
	userIDs := make(map[int64]struct{}, len(rows))
	for i := range rows {
		userIDs[rows[i].UserID] = struct{}{}
	}
	users := make(map[int64]*User, len(userIDs))
	if len(userIDs) > 0 {
		ids := make([]int64, 0, len(userIDs))
		for id := range userIDs {
			ids = append(ids, id)
		}
		var us []User
		if err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&us).Error; err != nil {
			return nil, err
		}
		for i := range us {
			users[us[i].ID] = &us[i]
		}
	}

	out := make([]*Comment, len(rows))
	for i := range rows {
		c := &Comment{
			ID:        rows[i].ID,
			PostID:    rows[i].PostID,
			UserID:    rows[i].UserID,
			Content:   rows[i].Content,
			CreatedAt: rows[i].CreatedAt,
		}
		if u, ok := users[rows[i].UserID]; ok {
			c.Username = u.Username
			c.DisplayName = u.DisplayName
			c.AvatarURL = u.AvatarURL
		}
		out[i] = c
	}
	return out, nil
}
