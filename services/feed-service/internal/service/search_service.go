package service

import (
	"context"
	"strings"

	"social-bbs/feed-service/internal/repository"
)

// SearchService implements the P1 MySQL LIKE search (plan §3.3, D7).
type SearchService interface {
	Search(ctx context.Context, query string, userID, cursor int64, limit int) ([]*repository.Post, int64, bool, error)
}

type searchService struct {
	posts repository.PostRepo
	likes repository.LikeRepo
	users repository.UserClient
}

// NewSearchService wires the search service.
func NewSearchService(posts repository.PostRepo, likes repository.LikeRepo, users repository.UserClient) SearchService {
	return &searchService{posts: posts, likes: likes, users: users}
}

// Search runs SELECT ... LIKE '%q%' ... ORDER BY created_at DESC with a
// created_at cursor (cursor > 0 → strictly older), LIMIT limit+1. Author info
// is enriched via User Service (P2 D-A8).
func (s *searchService) Search(ctx context.Context, query string, userID, cursor int64, limit int) ([]*repository.Post, int64, bool, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, 0, false, repository.ErrInvalidArgument
	}
	limit = clampLimit(limit)
	posts, err := s.posts.Search(ctx, query, cursor, limit+1)
	if err != nil {
		return nil, 0, false, err
	}
	enrichAuthors(ctx, s.users, posts)
	hasMore := len(posts) > limit
	if hasMore {
		posts = posts[:limit]
	}
	var nextCursor int64
	if n := len(posts); n > 0 {
		nextCursor = posts[n-1].CreatedAtMs()
	}
	if userID > 0 && len(posts) > 0 {
		ids := make([]int64, len(posts))
		for i, p := range posts {
			ids[i] = p.ID
		}
		liked, err := s.likes.LikedByUser(ctx, userID, ids)
		if err == nil {
			for _, p := range posts {
				p.LikedByViewer = liked[p.ID]
			}
		}
	}
	return posts, nextCursor, hasMore, nil
}
