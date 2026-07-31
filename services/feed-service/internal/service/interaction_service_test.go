package service

import (
	"context"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"social-bbs/feed-service/internal/repository"
)

// fakeLikeRepo is a minimal in-memory LikeRepo for service-level tests.
type fakeLikeRepo struct {
	likeNew   int64
	unlikeNew int64
	likeErr   error
	unlikeErr error
	liked     map[int64]bool
}

func (f *fakeLikeRepo) Like(context.Context, int64, int64) (int64, error) {
	return f.likeNew, f.likeErr
}

func (f *fakeLikeRepo) Unlike(context.Context, int64, int64) (int64, error) {
	return f.unlikeNew, f.unlikeErr
}

func (f *fakeLikeRepo) LikedByUser(_ context.Context, _ int64, postIDs []int64) (map[int64]bool, error) {
	out := make(map[int64]bool)
	for _, id := range postIDs {
		if f.liked[id] {
			out[id] = true
		}
	}
	return out, nil
}

func newMiniredisCache(t *testing.T) (repository.Cache, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return repository.NewRedisCache(rdb), mr
}

// TestLikeRefreshesCaches: after a successful like, post:likes:{id} mirrors the
// new count and post:detail:{id} is invalidated.
func TestLikeRefreshesCaches(t *testing.T) {
	cache, mr := newMiniredisCache(t)
	_ = mr.Set("post:detail:10", `{"id":10}`)

	svc := NewInteractionService(&fakeLikeRepo{likeNew: 4}, cache)
	if err := svc.Like(context.Background(), 1, 10); err != nil {
		t.Fatalf("Like: %v", err)
	}
	if got, _ := mr.Get("post:likes:10"); got != "4" {
		t.Errorf("post:likes:10 = %q, want %q", got, "4")
	}
	if mr.Exists("post:detail:10") {
		t.Error("post:detail:10 should have been DEL'd after like")
	}
}

// TestUnlikeRefreshesCaches: unlike mirrors the (floored) count and invalidates
// the detail cache.
func TestUnlikeRefreshesCaches(t *testing.T) {
	cache, mr := newMiniredisCache(t)
	_ = mr.Set("post:detail:10", `{"id":10}`)

	svc := NewInteractionService(&fakeLikeRepo{unlikeNew: 0}, cache)
	if err := svc.Unlike(context.Background(), 1, 10); err != nil {
		t.Fatalf("Unlike: %v", err)
	}
	if got, _ := mr.Get("post:likes:10"); got != "0" {
		t.Errorf("post:likes:10 = %q, want %q", got, "0")
	}
	if mr.Exists("post:detail:10") {
		t.Error("post:detail:10 should have been DEL'd after unlike")
	}
}

// TestInteractionErrorPropagation: repo errors surface and skip cache writes.
func TestInteractionErrorPropagation(t *testing.T) {
	cache, mr := newMiniredisCache(t)
	svc := NewInteractionService(&fakeLikeRepo{likeErr: repository.ErrPostNotFound}, cache)

	err := svc.Like(context.Background(), 1, 99)
	if !errors.Is(err, repository.ErrPostNotFound) {
		t.Errorf("Like: err = %v, want ErrPostNotFound", err)
	}
	if mr.Exists("post:likes:99") {
		t.Error("cache must not be written when the repo call fails")
	}
}
