package repository

import (
	"context"

	"gorm.io/gorm"
)

// UploadRepo accesses the uploads table (multipart upload session tracking).
type UploadRepo interface {
	Create(ctx context.Context, u *Upload) (*Upload, error)
	GetByID(ctx context.Context, uploadID string) (*Upload, error)
	// Update persists the whole upload row (received_chunks, status, ...).
	Update(ctx context.Context, u *Upload) error
	Delete(ctx context.Context, uploadID string) error
}

type gormUploadRepo struct {
	db *gorm.DB
}

// NewUploadRepo returns a GORM-backed UploadRepo.
func NewUploadRepo(db *gorm.DB) UploadRepo { return &gormUploadRepo{db: db} }

func (r *gormUploadRepo) Create(ctx context.Context, u *Upload) (*Upload, error) {
	if err := r.db.WithContext(ctx).Create(u).Error; err != nil {
		return nil, err
	}
	return u, nil
}

func (r *gormUploadRepo) GetByID(ctx context.Context, uploadID string) (*Upload, error) {
	var u Upload
	err := r.db.WithContext(ctx).First(&u, "upload_id = ?", uploadID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrUploadNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (r *gormUploadRepo) Update(ctx context.Context, u *Upload) error {
	return r.db.WithContext(ctx).Save(u).Error
}

func (r *gormUploadRepo) Delete(ctx context.Context, uploadID string) error {
	res := r.db.WithContext(ctx).Delete(&Upload{}, "upload_id = ?", uploadID)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrUploadNotFound
	}
	return nil
}