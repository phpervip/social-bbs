package repository

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// postCols selects the posts columns into the Post read model. Author info is
// NOT joined anymore (P2 D-A8): the service layer enriches via UserClient.
const postCols = "posts.id, posts.user_id, posts.content, posts.media_url, " +
	"posts.like_count, posts.comment_count, posts.created_at"

type gormPostRepo struct {
	db *gorm.DB
}

// NewPostRepo returns a GORM-backed PostRepo.
func NewPostRepo(db *gorm.DB) PostRepo { return &gormPostRepo{db: db} }

// WithTx runs fn inside a transaction, committing on success. Errors from fn
// roll the transaction back.
func (r *gormPostRepo) WithTx(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return r.db.WithContext(ctx).Transaction(fn)
}

// Create inserts a post row inside tx (nil = plain insert) and returns the
// read model. The service coordinates the same tx with the outbox write so
// the post and its post.created event commit atomically (design §5.1).
func (r *gormPostRepo) Create(ctx context.Context, tx *gorm.DB, userID int64, content, mediaURL string) (*Post, error) {
	db := r.db
	if tx != nil {
		db = tx
	}
	row := PostRow{UserID: userID, Content: content, MediaURL: mediaURL}
	if err := db.WithContext(ctx).Create(&row).Error; err != nil {
		return nil, err
	}
	return &Post{
		ID:           row.ID,
		UserID:       row.UserID,
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
		Select(postCols).
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
		Select(postCols).
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

// scanPage runs the shared newest-first query shape and scans into []*Post.
func (r *gormPostRepo) scanPage(q *gorm.DB) ([]*Post, error) {
	var rows []Post
	if err := q.Scan(&rows).Error; err != nil {
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
		Select(postCols).
		Where("posts.deleted_at IS NULL")
	if cursor > 0 {
		q = q.Where("posts.created_at < ?", time.UnixMilli(cursor))
	}
	return r.scanPage(q.Order("posts.created_at DESC, posts.id DESC").Limit(limit))
}

func (r *gormPostRepo) LatestByAuthor(ctx context.Context, authorID, cursor int64, limit int) ([]*Post, error) {
	q := r.db.WithContext(ctx).Table("posts").
		Select(postCols).
		Where("posts.user_id = ?", authorID).
		Where("posts.deleted_at IS NULL")
	if cursor > 0 {
		q = q.Where("posts.created_at < ?", time.UnixMilli(cursor))
	}
	return r.scanPage(q.Order("posts.created_at DESC, posts.id DESC").Limit(limit))
}

func (r *gormPostRepo) LatestByAuthors(ctx context.Context, authorIDs []int64, limit int) ([]*Post, error) {
	if len(authorIDs) == 0 {
		return nil, nil
	}
	q := r.db.WithContext(ctx).Table("posts").
		Select(postCols).
		Where("posts.user_id IN ?", authorIDs).
		Where("posts.deleted_at IS NULL")
	return r.scanPage(q.Order("posts.created_at DESC, posts.id DESC").Limit(limit))
}

func (r *gormPostRepo) Search(ctx context.Context, query string, cursor int64, limit int) ([]*Post, error) {
	q := r.db.WithContext(ctx).Table("posts").
		Select(postCols).
		Where("posts.content LIKE ?", "%"+query+"%").
		Where("posts.deleted_at IS NULL")
	if cursor > 0 {
		q = q.Where("posts.created_at < ?", time.UnixMilli(cursor))
	}
	return r.scanPage(q.Order("posts.created_at DESC, posts.id DESC").Limit(limit))
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
