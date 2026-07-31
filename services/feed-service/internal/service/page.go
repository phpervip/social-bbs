package service

import "social-bbs/feed-service/internal/repository"

// clampLimit applies the P1 cursor-pagination contract (plan §3.3):
// 0/absent → 20, >50 → 50, <0 → 20, otherwise unchanged.
func clampLimit(limit int) int {
	if limit <= 0 {
		return repository.DefaultPageLimit
	}
	if limit > repository.MaxPageLimit {
		return repository.MaxPageLimit
	}
	return limit
}
