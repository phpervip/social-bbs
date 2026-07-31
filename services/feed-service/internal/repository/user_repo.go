package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

type gormUserRepo struct {
	db *gorm.DB
}

// NewUserRepo returns a GORM-backed UserRepo.
func NewUserRepo(db *gorm.DB) UserRepo { return &gormUserRepo{db: db} }

func (r *gormUserRepo) GetByID(ctx context.Context, id int64) (*User, error) {
	var u User
	if err := r.db.WithContext(ctx).First(&u, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (r *gormUserRepo) ListIDs(ctx context.Context) ([]int64, error) {
	var ids []int64
	if err := r.db.WithContext(ctx).Model(&User{}).Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}
