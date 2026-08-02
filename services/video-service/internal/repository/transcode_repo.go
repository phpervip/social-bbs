package repository

import (
	"context"

	"gorm.io/gorm"
)

// TranscodeRepo accesses the transcode_tasks table.
type TranscodeRepo interface {
	Create(ctx context.Context, t *TranscodeTask) (*TranscodeTask, error)
	// CreateBatch inserts one task per quality for a video (720p/480p/360p).
	CreateBatch(ctx context.Context, videoID uint64, qualities []string) error
	GetByVideoID(ctx context.Context, videoID uint64) ([]*TranscodeTask, error)
	GetPending(ctx context.Context, limit int) ([]*TranscodeTask, error)
	// UpdateStatus sets status (and error_msg) and increments retry_count when
	// the new status is failed.
	UpdateStatus(ctx context.Context, id uint64, status, errorMsg string) error
}

type gormTranscodeRepo struct {
	db *gorm.DB
}

// NewTranscodeRepo returns a GORM-backed TranscodeRepo.
func NewTranscodeRepo(db *gorm.DB) TranscodeRepo { return &gormTranscodeRepo{db: db} }

func (r *gormTranscodeRepo) Create(ctx context.Context, t *TranscodeTask) (*TranscodeTask, error) {
	if err := r.db.WithContext(ctx).Create(t).Error; err != nil {
		return nil, err
	}
	return t, nil
}

func (r *gormTranscodeRepo) CreateBatch(ctx context.Context, videoID uint64, qualities []string) error {
	if len(qualities) == 0 {
		return nil
	}
	tasks := make([]TranscodeTask, 0, len(qualities))
	for _, q := range qualities {
		tasks = append(tasks, TranscodeTask{VideoID: videoID, Quality: q, Status: "pending"})
	}
	return r.db.WithContext(ctx).Create(&tasks).Error
}

func (r *gormTranscodeRepo) GetByVideoID(ctx context.Context, videoID uint64) ([]*TranscodeTask, error) {
	var rows []TranscodeTask
	if err := r.db.WithContext(ctx).Where("video_id = ?", videoID).Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*TranscodeTask, len(rows))
	for i := range rows {
		out[i] = &rows[i]
	}
	return out, nil
}

func (r *gormTranscodeRepo) GetPending(ctx context.Context, limit int) ([]*TranscodeTask, error) {
	var rows []TranscodeTask
	if err := r.db.WithContext(ctx).Where("status = ?", "pending").Order("id ASC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*TranscodeTask, len(rows))
	for i := range rows {
		out[i] = &rows[i]
	}
	return out, nil
}

func (r *gormTranscodeRepo) UpdateStatus(ctx context.Context, id uint64, status, errorMsg string) error {
	updates := map[string]any{"status": status, "error_msg": errorMsg}
	if status == "failed" {
		updates["retry_count"] = gorm.Expr("retry_count + 1")
	}
	return r.db.WithContext(ctx).Model(&TranscodeTask{}).Where("id = ?", id).Updates(updates).Error
}