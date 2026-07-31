package service

import (
	"context"
	"strings"
	"unicode/utf8"

	"social-bbs/feed-service/internal/repository"
)

// CommentService handles comments (plan §3.3).
type CommentService interface {
	AddComment(ctx context.Context, postID, userID int64, content string) (*repository.Comment, error)
	GetComments(ctx context.Context, postID, cursor int64, limit int) ([]*repository.Comment, int64, bool, error)
}

type commentService struct {
	comments repository.CommentRepo
	cache    repository.Cache
}

// NewCommentService wires the comment service.
func NewCommentService(comments repository.CommentRepo, cache repository.Cache) CommentService {
	return &commentService{comments: comments, cache: cache}
}

// AddComment validates content (non-empty, ≤500 runes), inserts the comment and
// bumps comment_count in the same tx, then invalidates post:detail:{id}
// (plan §3.2: cache is DEL'd on update).
func (s *commentService) AddComment(ctx context.Context, postID, userID int64, content string) (*repository.Comment, error) {
	content = strings.TrimSpace(content)
	if content == "" || utf8.RuneCountInString(content) > repository.MaxCommentRunes {
		return nil, repository.ErrInvalidArgument
	}
	comment, err := s.comments.Add(ctx, postID, userID, content)
	if err != nil {
		return nil, err
	}
	_ = s.cache.Del(ctx, repository.PostDetailKey(postID))
	return comment, nil
}

// GetComments paginates comments newest-first by created_at cursor.
func (s *commentService) GetComments(ctx context.Context, postID, cursor int64, limit int) ([]*repository.Comment, int64, bool, error) {
	limit = clampLimit(limit)
	comments, err := s.comments.ListByPost(ctx, postID, cursor, limit+1)
	if err != nil {
		return nil, 0, false, err
	}
	hasMore := len(comments) > limit
	if hasMore {
		comments = comments[:limit]
	}
	var nextCursor int64
	if n := len(comments); n > 0 {
		nextCursor = comments[n-1].CreatedAtMs()
	}
	return comments, nextCursor, hasMore, nil
}
