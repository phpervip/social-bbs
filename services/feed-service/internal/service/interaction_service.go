package service

import (
	"context"
	"strconv"

	"social-bbs/feed-service/internal/repository"
)

// InteractionService handles like/unlike (plan §3.3).
type InteractionService interface {
	Like(ctx context.Context, userID, postID int64) error
	Unlike(ctx context.Context, userID, postID int64) error
}

type interactionService struct {
	likes repository.LikeRepo
	cache repository.Cache
}

// NewInteractionService wires the interaction service.
func NewInteractionService(likes repository.LikeRepo, cache repository.Cache) InteractionService {
	return &interactionService{likes: likes, cache: cache}
}

// Like is idempotent (INSERT IGNORE); liking twice is still success (Empty).
func (s *interactionService) Like(ctx context.Context, userID, postID int64) error {
	newCount, err := s.likes.Like(ctx, postID, userID)
	if err != nil {
		return err
	}
	s.refreshLikeCache(ctx, postID, newCount)
	return nil
}

// Unlike is idempotent; unliking a post that was not liked is still success.
func (s *interactionService) Unlike(ctx context.Context, userID, postID int64) error {
	newCount, err := s.likes.Unlike(ctx, postID, userID)
	if err != nil {
		return err
	}
	s.refreshLikeCache(ctx, postID, newCount)
	return nil
}

// refreshLikeCache mirrors the new count into post:likes:{id} (30min TTL) and
// invalidates post:detail:{id}.
func (s *interactionService) refreshLikeCache(ctx context.Context, postID, newCount int64) {
	_ = s.cache.Set(ctx, repository.PostLikesKey(postID), strconv.FormatInt(newCount, 10), repository.PostLikesTTL)
	_ = s.cache.Del(ctx, repository.PostDetailKey(postID))
}
