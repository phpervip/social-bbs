package repository

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type gormLikeRepo struct {
	db *gorm.DB
}

// NewLikeRepo returns a GORM-backed LikeRepo.
func NewLikeRepo(db *gorm.DB) LikeRepo { return &gormLikeRepo{db: db} }

// clampLikeDelta keeps the like_count from ever dropping below 0.
// It mirrors the GREATEST(like_count - 1, 0) guard in the Unlike UPDATE.
func clampLikeDelta(current, delta int64) int64 {
	if current+delta < 0 {
		return 0
	}
	return current + delta
}

type postLockRow struct {
	ID        int64
	LikeCount int64
}

// lockPost locks and reads a live post row (deleted_at IS NULL), returning
// ErrPostNotFound when the post is missing or soft-deleted.
func lockPost(tx *gorm.DB, postID int64) (*postLockRow, error) {
	var row postLockRow
	if err := tx.Raw("SELECT id, like_count FROM posts WHERE id = ? AND deleted_at IS NULL FOR UPDATE", postID).Scan(&row).Error; err != nil {
		return nil, err
	}
	if row.ID == 0 {
		return nil, ErrPostNotFound
	}
	return &row, nil
}

// Like is idempotent: INSERT IGNORE; the count is incremented only when the
// like row was actually inserted (exact counting). Already-liked → success with
// the count unchanged.
func (r *gormLikeRepo) Like(ctx context.Context, postID, userID int64) (int64, error) {
	var newCount int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := lockPost(tx, postID)
		if err != nil {
			return err
		}
		res := tx.Exec("INSERT IGNORE INTO post_likes (post_id, user_id, created_at) VALUES (?, ?, ?)",
			postID, userID, time.Now())
		if res.Error != nil {
			return res.Error
		}
		newCount = row.LikeCount
		if res.RowsAffected == 1 {
			if err := tx.Exec("UPDATE posts SET like_count = like_count + 1 WHERE id = ? AND deleted_at IS NULL", postID).Error; err != nil {
				return err
			}
			newCount = row.LikeCount + 1
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return newCount, nil
}

// Unlike is idempotent: DELETE; the count is decremented only when a like row
// was actually removed, guarded by GREATEST(...) so it never goes below 0.
func (r *gormLikeRepo) Unlike(ctx context.Context, postID, userID int64) (int64, error) {
	var newCount int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := lockPost(tx, postID)
		if err != nil {
			return err
		}
		res := tx.Exec("DELETE FROM post_likes WHERE post_id = ? AND user_id = ?", postID, userID)
		if res.Error != nil {
			return res.Error
		}
		newCount = row.LikeCount
		if res.RowsAffected == 1 {
			if err := tx.Exec("UPDATE posts SET like_count = GREATEST(like_count - 1, 0) WHERE id = ?", postID).Error; err != nil {
				return err
			}
			newCount = clampLikeDelta(row.LikeCount, -1)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return newCount, nil
}

// LikedByUser returns the subset of postIDs liked by userID (viewer context).
func (r *gormLikeRepo) LikedByUser(ctx context.Context, userID int64, postIDs []int64) (map[int64]bool, error) {
	out := make(map[int64]bool)
	if userID == 0 || len(postIDs) == 0 {
		return out, nil
	}
	var ids []int64
	if err := r.db.WithContext(ctx).Model(&PostLike{}).
		Where("user_id = ? AND post_id IN ?", userID, postIDs).
		Pluck("post_id", &ids).Error; err != nil {
		return nil, err
	}
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}
