package worker

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"social-bbs/feed-service/internal/kafka"
	"social-bbs/feed-service/internal/repository"
)

// fakeConsumerPostRepo is an in-memory PostRepo for consumer tests: only
// LatestByAuthor matters (backfill/cleanup source); everything else is a no-op.
type fakeConsumerPostRepo struct {
	byAuthor map[int64][]*repository.Post
}

func (f *fakeConsumerPostRepo) Create(context.Context, *gorm.DB, int64, string, string) (*repository.Post, error) {
	return nil, nil
}
func (f *fakeConsumerPostRepo) WithTx(_ context.Context, fn func(tx *gorm.DB) error) error {
	return fn(nil)
}
func (f *fakeConsumerPostRepo) GetByID(context.Context, int64) (*repository.Post, error) {
	return nil, repository.ErrPostNotFound
}
func (f *fakeConsumerPostRepo) GetByIDs(context.Context, []int64) ([]*repository.Post, error) {
	return nil, nil
}
func (f *fakeConsumerPostRepo) Latest(context.Context, int64, int) ([]*repository.Post, error) {
	return nil, nil
}
func (f *fakeConsumerPostRepo) LatestByAuthor(_ context.Context, authorID, _ int64, _ int) ([]*repository.Post, error) {
	return f.byAuthor[authorID], nil
}
func (f *fakeConsumerPostRepo) LatestByAuthors(context.Context, []int64, int) ([]*repository.Post, error) {
	return nil, nil
}
func (f *fakeConsumerPostRepo) Search(context.Context, string, int64, int) ([]*repository.Post, error) {
	return nil, nil
}
func (f *fakeConsumerPostRepo) SoftDelete(context.Context, int64) error { return nil }

func newConsumerEnv(t *testing.T, byAuthor map[int64][]*repository.Post) (*Consumer, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	c := NewConsumer(
		NewFanoutHandler(repository.NewRedisCache(rdb), &fakeUserClient{}),
		&fakeConsumerPostRepo{byAuthor: byAuthor},
		repository.NewRedisCache(rdb),
		logger,
	)
	return c, mr
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// zMember is the canonical ZSet member encoding for a post id (strconv of the
// numeric id — matches the production fanout writes).
func zMember(id int64) string {
	return strconv.FormatInt(id, 10)
}

// TestFollowBackfillZAddsFolloweePosts: a follow event pushes the followee's
// recent posts (LatestByAuthor) into the follower's feed:home with scores,
// TTL, and the cap (design §5.4 follow branch).
func TestFollowBackfillZAddsFolloweePosts(t *testing.T) {
	p1 := &repository.Post{ID: 11, UserID: 2, Content: "a", CreatedAt: time.UnixMilli(5000)}
	p2 := &repository.Post{ID: 12, UserID: 2, Content: "b", CreatedAt: time.UnixMilli(6000)}
	c, mr := newConsumerEnv(t, map[int64][]*repository.Post{2: {p1, p2}})
	ctx := context.Background()

	err := c.handleFollowChanged(ctx, mustJSON(t, kafka.FollowChangedEvent{
		FollowerID: 1, FolloweeID: 2, Action: "follow", CreatedAt: time.Now().UnixMilli(),
	}))
	if err != nil {
		t.Fatalf("handleFollowChanged: %v", err)
	}

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()
	for _, p := range []*repository.Post{p1, p2} {
		score, err := rdb.ZScore(ctx, repository.FeedHomeKey(1), zMember(p.ID)).Result()
		if err != nil {
			t.Fatalf("feed:home:1 missing post %d: %v", p.ID, err)
		}
		if int64(score) != p.CreatedAtMs() {
			t.Errorf("post %d score = %v, want %d", p.ID, score, p.CreatedAtMs())
		}
	}
	ttl, _ := rdb.TTL(ctx, repository.FeedHomeKey(1)).Result()
	if ttl <= 0 || ttl > repository.FeedHomeTTL {
		t.Errorf("feed:home:1 TTL = %v, want ~%v", ttl, repository.FeedHomeTTL)
	}
}

// TestFollowBackfillWithNoPosts: a follow event for an author with no posts
// writes nothing.
func TestFollowBackfillWithNoPosts(t *testing.T) {
	c, mr := newConsumerEnv(t, nil)
	ctx := context.Background()

	if err := c.handleFollowChanged(ctx, mustJSON(t, kafka.FollowChangedEvent{
		FollowerID: 1, FolloweeID: 99, Action: "follow",
	})); err != nil {
		t.Fatalf("handleFollowChanged: %v", err)
	}
	if mr.Exists(repository.FeedHomeKey(1)) {
		t.Error("feed:home:1 must not exist when followee has no posts")
	}
}

// TestUnfollowZRemovesFolloweePosts: an unfollow event ZREMs the followee's
// posts from the follower's feed:home (design §5.4 unfollow branch).
func TestUnfollowZRemovesFolloweePosts(t *testing.T) {
	p1 := &repository.Post{ID: 11, UserID: 2, Content: "a", CreatedAt: time.UnixMilli(5000)}
	p2 := &repository.Post{ID: 12, UserID: 2, Content: "b", CreatedAt: time.UnixMilli(6000)}
	other := &repository.Post{ID: 21, UserID: 3, Content: "keep", CreatedAt: time.UnixMilli(7000)}
	c, mr := newConsumerEnv(t, map[int64][]*repository.Post{2: {p1, p2}})
	ctx := context.Background()

	// Pre-seed the follower feed with the followee's posts + another author's.
	cache := repository.NewRedisCache(redis.NewClient(&redis.Options{Addr: mr.Addr()}))
	for _, p := range []*repository.Post{p1, p2, other} {
		_ = cache.ZAdd(ctx, repository.FeedHomeKey(1), float64(p.CreatedAtMs()), zMember(p.ID))
	}

	if err := c.handleFollowChanged(ctx, mustJSON(t, kafka.FollowChangedEvent{
		FollowerID: 1, FolloweeID: 2, Action: "unfollow",
	})); err != nil {
		t.Fatalf("handleFollowChanged: %v", err)
	}

	members, err := mr.ZMembers(repository.FeedHomeKey(1))
	if err != nil {
		t.Fatalf("ZMembers: %v", err)
	}
	if len(members) != 1 || members[0] != zMember(other.ID) {
		t.Errorf("feed:home:1 members = %v, want only [%s] (followee posts removed)", members, zMember(other.ID))
	}
}

// TestFollowChangedIgnoresSelfFollow: follower == followee is a no-op
// (self-follows are rejected by User Service anyway; belt and braces).
func TestFollowChangedIgnoresSelfFollow(t *testing.T) {
	c, mr := newConsumerEnv(t, nil)
	ctx := context.Background()

	if err := c.handleFollowChanged(ctx, mustJSON(t, kafka.FollowChangedEvent{
		FollowerID: 5, FolloweeID: 5, Action: "follow",
	})); err != nil {
		t.Fatalf("handleFollowChanged: %v", err)
	}
	if mr.Exists(repository.FeedHomeKey(5)) {
		t.Error("self-follow must not write any feed")
	}
}

// TestFollowChangedIgnoresUnknownAction: future actions are skipped, not fatal.
func TestFollowChangedIgnoresUnknownAction(t *testing.T) {
	c, mr := newConsumerEnv(t, nil)
	ctx := context.Background()

	if err := c.handleFollowChanged(ctx, mustJSON(t, kafka.FollowChangedEvent{
		FollowerID: 1, FolloweeID: 2, Action: "block",
	})); err != nil {
		t.Fatalf("handleFollowChanged: %v", err)
	}
	if mr.Exists(repository.FeedHomeKey(1)) {
		t.Error("unknown action must not write any feed")
	}
}

// TestPostCreatedRoutesToFanout: a post.created message fans the post out to
// the author's real followers + the author (integration of the two consumers).
func TestPostCreatedRoutesToFanout(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	c := NewConsumer(
		NewFanoutHandler(repository.NewRedisCache(rdb), &fakeUserClient{followers: map[int64][]int64{2: {1, 3}}}),
		&fakeConsumerPostRepo{},
		repository.NewRedisCache(rdb),
		logger,
	)
	ctx := context.Background()

	if err := c.handlePostCreated(ctx, mustJSON(t, kafka.PostCreatedEvent{
		PostID: 7, UserID: 2, Content: "hi", CreatedAt: 12345,
	})); err != nil {
		t.Fatalf("handlePostCreated: %v", err)
	}

	for _, uid := range []int64{1, 2, 3} {
		members, err := mr.ZMembers(repository.FeedHomeKey(uid))
		if err != nil || len(members) != 1 || members[0] != "7" {
			t.Fatalf("feed:home:%d members = %v (err %v), want [7]", uid, members, err)
		}
	}
}
