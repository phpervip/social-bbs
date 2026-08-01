package service

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"

	"social-bbs/feed-service/internal/kafka"
	"social-bbs/feed-service/internal/repository"
)

// PostService handles the post lifecycle RPCs.
type PostService interface {
	CreatePost(ctx context.Context, userID int64, content, mediaURL string) (*repository.Post, error)
	GetPost(ctx context.Context, id, viewerID int64) (*repository.Post, error)
	DeletePost(ctx context.Context, id, userID int64) error
}

type postService struct {
	posts  repository.PostRepo
	users  repository.UserClient
	likes  repository.LikeRepo
	cache  repository.Cache
	outbox repository.OutboxRepo
}

// NewPostService wires the post service.
func NewPostService(posts repository.PostRepo, outbox repository.OutboxRepo, users repository.UserClient, likes repository.LikeRepo, cache repository.Cache) PostService {
	return &postService{posts: posts, outbox: outbox, users: users, likes: likes, cache: cache}
}

// CreatePost validates content (non-empty, ≤280 runes) and the author, inserts
// the post and a pending post.created outbox event in ONE transaction (design
// §5.1), then enriches the response with the author profile. The response
// never blocks on the outbox dispatch — a Kafka consumer fans out later.
func (s *postService) CreatePost(ctx context.Context, userID int64, content, mediaURL string) (*repository.Post, error) {
	content = strings.TrimSpace(content)
	if content == "" || utf8.RuneCountInString(content) > repository.MaxContentRunes {
		return nil, repository.ErrInvalidArgument
	}
	// Author existence check now goes through User Service (P2 D-A8); the old
	// feed_db.users lookup is gone.
	author, err := s.users.GetProfile(ctx, userID)
	if err != nil {
		if err == repository.ErrUserNotFound {
			return nil, repository.ErrInvalidArgument
		}
		return nil, err
	}

	var post *repository.Post
	err = s.posts.WithTx(ctx, func(tx *gorm.DB) error {
		p, err := s.posts.Create(ctx, tx, userID, content, mediaURL)
		if err != nil {
			return err
		}
		payload, err := json.Marshal(kafka.PostCreatedEvent{
			PostID:    p.ID,
			UserID:    userID,
			Content:   content,
			CreatedAt: p.CreatedAtMs(),
		})
		if err != nil {
			return err
		}
		if err := s.outbox.CreateInTx(ctx, tx, kafka.TopicPostCreated, payload); err != nil {
			return err
		}
		post = p
		return nil
	})
	if err != nil {
		return nil, err
	}

	post.Username = author.Username
	post.DisplayName = author.DisplayName
	post.AvatarURL = author.AvatarURL
	return post, nil
}

// GetPost reads post:detail:{id} cache first; on miss falls back to MySQL and
// writes the cache for 30min. Author info is enriched via User Service.
// liked_by_viewer is always computed per-request.
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
	s.enrichAuthor(ctx, post)
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

// enrichAuthor fills Username/DisplayName/AvatarURL from User Service; a
// failure leaves the fields empty (best effort).
func (s *postService) enrichAuthor(ctx context.Context, post *repository.Post) {
	if post == nil || post.UserID <= 0 {
		return
	}
	if author, err := s.users.GetProfile(ctx, post.UserID); err == nil {
		post.Username = author.Username
		post.DisplayName = author.DisplayName
		post.AvatarURL = author.AvatarURL
	}
}

func (s *postService) isLiked(ctx context.Context, viewerID, postID int64) bool {
	if viewerID <= 0 {
		return false
	}
	liked, err := s.likes.LikedByUser(ctx, viewerID, []int64{postID})
	return err == nil && liked[postID]
}
