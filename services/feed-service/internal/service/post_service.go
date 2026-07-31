package service

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"social-bbs/feed-service/internal/repository"
	"social-bbs/feed-service/internal/worker"
)

// PostService handles the post lifecycle RPCs.
type PostService interface {
	CreatePost(ctx context.Context, userID int64, content, mediaURL string) (*repository.Post, error)
	GetPost(ctx context.Context, id, viewerID int64) (*repository.Post, error)
	DeletePost(ctx context.Context, id, userID int64) error
}

type postService struct {
	posts  repository.PostRepo
	users  repository.UserRepo
	likes  repository.LikeRepo
	cache  repository.Cache
	fanout worker.Enqueuer
}

// NewPostService wires the post service.
func NewPostService(posts repository.PostRepo, users repository.UserRepo, likes repository.LikeRepo, cache repository.Cache, fanout worker.Enqueuer) PostService {
	return &postService{posts: posts, users: users, likes: likes, cache: cache, fanout: fanout}
}

// CreatePost validates content (non-empty, ≤280 runes) and the author, inserts
// the post, then enqueues the fanout asynchronously — the response never blocks
// on the fanout (D2).
func (s *postService) CreatePost(ctx context.Context, userID int64, content, mediaURL string) (*repository.Post, error) {
	content = strings.TrimSpace(content)
	if content == "" || utf8.RuneCountInString(content) > repository.MaxContentRunes {
		return nil, repository.ErrInvalidArgument
	}
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		if err == repository.ErrUserNotFound {
			return nil, repository.ErrInvalidArgument
		}
		return nil, err
	}
	post, err := s.posts.Create(ctx, user, content, mediaURL)
	if err != nil {
		return nil, err
	}
	s.fanout.Enqueue(worker.FanoutEvent{PostID: post.ID, AuthorID: post.UserID, CreatedAtMs: post.CreatedAtMs()})
	return post, nil
}

// GetPost reads post:detail:{id} cache first; on miss falls back to MySQL and
// writes the cache for 30min. liked_by_viewer is always computed per-request.
func (s *postService) GetPost(ctx context.Context, id, viewerID int64) (*repository.Post, error) {
	if raw, err := s.cache.Get(ctx, repository.PostDetailKey(id)); err == nil {
		var post repository.Post
		if json.Unmarshal([]byte(raw), &post) == nil {
			post.LikedByViewer = s.isLiked(ctx, viewerID, id)
			return &post, nil
		}
	}
	post, err := s.posts.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if data, err := json.Marshal(post); err == nil {
		_ = s.cache.Set(ctx, repository.PostDetailKey(id), string(data), repository.PostDetailTTL)
	}
	post.LikedByViewer = s.isLiked(ctx, viewerID, id)
	return post, nil
}

// DeletePost soft-deletes a post; only the author may delete. Timelines are NOT
// cleaned (read-time filtering).
func (s *postService) DeletePost(ctx context.Context, id, userID int64) error {
	post, err := s.posts.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if post.UserID != userID {
		return repository.ErrPermissionDenied
	}
	if err := s.posts.SoftDelete(ctx, id); err != nil {
		return err
	}
	_ = s.cache.Del(ctx, repository.PostDetailKey(id))
	return nil
}

func (s *postService) isLiked(ctx context.Context, viewerID, postID int64) bool {
	if viewerID <= 0 {
		return false
	}
	liked, err := s.likes.LikedByUser(ctx, viewerID, []int64{postID})
	return err == nil && liked[postID]
}
