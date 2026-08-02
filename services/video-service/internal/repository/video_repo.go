package repository

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// VideoRepo accesses the videos table.
type VideoRepo interface {
	Create(ctx context.Context, v *Video) (*Video, error)
	GetByID(ctx context.Context, id uint64) (*Video, error)
	Update(ctx context.Context, v *Video) error
	Delete(ctx context.Context, id uint64) error
	// ListByUploaderID returns uploaderID's videos newest-first with created_at
	// cursor pagination (cursor = last created_at unix ms, 0 = first page).
	ListByUploaderID(ctx context.Context, uploaderID uint64, cursor int64, limit int) ([]*Video, error)
}

type gormVideoRepo struct {
	db *gorm.DB
}

// NewVideoRepo returns a GORM-backed VideoRepo.
func NewVideoRepo(db *gorm.DB) VideoRepo { return &gormVideoRepo{db: db} }

func (r *gormVideoRepo) Create(ctx context.Context, v *Video) (*Video, error) {
	if err := r.db.WithContext(ctx).Create(v).Error; err != nil {
		return nil, err
	}
	return v, nil
}

func (r *gormVideoRepo) GetByID(ctx context.Context, id uint64) (*Video, error) {
	var v Video
	err := r.db.WithContext(ctx).First(&v, id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrVideoNotFound
		}
		return nil, err
	}
	return &v, nil
}

func (r *gormVideoRepo) Update(ctx context.Context, v *Video) error {
	return r.db.WithContext(ctx).Save(v).Error
}

func (r *gormVideoRepo) Delete(ctx context.Context, id uint64) error {
	res := r.db.WithContext(ctx).Delete(&Video{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrVideoNotFound
	}
	return nil
}

func (r *gormVideoRepo) ListByUploaderID(ctx context.Context, uploaderID uint64, cursor int64, limit int) ([]*Video, error) {
	q := r.db.WithContext(ctx).Where("uploader_id = ?", uploaderID)
	if cursor > 0 {
		q = q.Where("created_at < ?", time.UnixMilli(cursor))
	}
	var rows []Video
	if err := q.Order("created_at DESC, id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*Video, len(rows))
	for i := range rows {
		out[i] = &rows[i]
	}
	return out, nil
}