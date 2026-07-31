package repository

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// postSelectCols selects the joined posts+users columns into the Post read model.
const postSelectCols = "posts.id, posts.user_id, users.username, users.display_name, users.avatar_url, " +
	"posts.content, posts.media_url, posts.like_count, posts.comment_count, posts.created_at"

type gormPostRepo struct {
	db *gorm.DB
}

// NewPostRepo returns a GORM-backed PostRepo.
func NewPostRepo(db *gorm.DB) PostRepo { return &gormPostRepo{db: db} }

func (r *gormPostRepo) Create(ctx context.Context, user *User, content, mediaURL string) (*Post, error) {
	row := PostRow{UserID: user.ID, Content: content, MediaURL: mediaURL}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return nil, err
	}
	return &Post{
		ID:           row.ID,
		UserID:       row.UserID,
		Username:     user.Username,
		DisplayName:  user.DisplayName,
		AvatarURL:    user.AvatarURL,
		Content:      row.Content,
		MediaURL:     row.MediaURL,
		LikeCount:    0,
		CommentCount: 0,
		CreatedAt:    row.CreatedAt,
	}, nil
}

func (r *gormPostRepo) GetByID(ctx context.Context, id int64) (*Post, error) {
	var p Post
	err := r.db.WithContext(ctx).Table("posts").
		Select(postSelectCols).
		Joins("INNER JOIN users ON users.id = posts.user_id").
		Where("posts.id = ?", id).
		Where("posts.deleted_at IS NULL").
		Scan(&p).Error
	if err != nil {
		return nil, err
	}
	if p.ID == 0 {
		return nil, ErrPostNotFound
	}
	return &p, nil
}

func (r *gormPostRepo) GetByIDs(ctx context.Context, ids []int64) ([]*Post, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var rows []Post
	err := r.db.WithContext(ctx).Table("posts").
		Select(postSelectCols).
		Joins("INNER JOIN users ON users.id = posts.user_id").
		Where("posts.id IN ?", ids).
		Where("posts.deleted_at IS NULL").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]*Post, len(rows))
	for i := range rows {
		out[i] = &rows[i]
	}
	return out, nil
}

func (r *gormPostRepo) Latest(ctx context.Context, cursor int64, limit int) ([]*Post, error) {
	q := r.db.WithContext(ctx).Table("posts").
		Select(postSelectCols).
		Joins("INNER JOIN users ON users.id = posts.user_id").
		Where("posts.deleted_at IS NULL")
	if cursor > 0 {
		q = q.Where("posts.created_at < ?", time.UnixMilli(cursor))
	}
	var rows []Post
	err := q.Order("posts.created_at DESC, posts.id DESC").Limit(limit).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]*Post, len(rows))
	for i := range rows {
		out[i] = &rows[i]
	}
	return out, nil
}

func (r *gormPostRepo) Search(ctx context.Context, query string, cursor int64, limit int) ([]*Post, error) {
	q := r.db.WithContext(ctx).Table("posts").
		Select(postSelectCols).
		Joins("INNER JOIN users ON users.id = posts.user_id").
		Where("posts.content LIKE ?", "%"+query+"%").
		Where("posts.deleted_at IS NULL")
	if cursor > 0 {
		q = q.Where("posts.created_at < ?", time.UnixMilli(cursor))
	}
	var rows []Post
	err := q.Order("posts.created_at DESC, posts.id DESC").Limit(limit).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]*Post, len(rows))
	for i := range rows {
		out[i] = &rows[i]
	}
	return out, nil
}

func (r *gormPostRepo) SoftDelete(ctx context.Context, id int64) error {
	res := r.db.WithContext(ctx).Delete(&PostRow{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrPostNotFound
	}
	return nil
}
