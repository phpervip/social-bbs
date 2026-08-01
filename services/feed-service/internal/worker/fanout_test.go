package worker

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"social-bbs/feed-service/internal/repository"
)

// fakeUserClient is a minimal in-memory UserClient for fanout tests: follower
// ids come from a static map, profiles are never resolvable.
type fakeUserClient struct {
	followers map[int64][]int64
}

func (f *fakeUserClient) GetProfile(context.Context, int64) (*repository.User, error) {
	return nil, repository.ErrUserNotFound
}

func (f *fakeUserClient) GetProfiles(context.Context, []int64) (map[int64]*repository.User, error) {
	return nil, nil
}

func (f *fakeUserClient) GetFollowerIDs(_ context.Context, userID int64) ([]int64, error) {
	return f.followers[userID], nil
}

func (f *fakeUserClient) GetFollowingIDs(context.Context, int64) ([]int64, error) {
	return nil, nil
}

func (f *fakeUserClient) Close() error { return nil }

func newFanoutEnv(t *testing.T, followers map[int64][]int64) (repository.Cache, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return repository.NewRedisCache(rdb), mr
}

// TestFanoutPushesToFollowers: a new post is ZADDed into EVERY real follower's
// feed:home ZSet plus the author's own, with the created_at score, a 7d TTL,
// and the 500-member cap (design §5.3 RealFollowersMode).
func TestFanoutPushesToFollowers(t *testing.T) {
	cache, mr := newFanoutEnv(t, map[int64][]int64{2: {1, 3}})
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	h := NewFanoutHandler(cache, &fakeUserClient{followers: map[int64][]int64{2: {1, 3}}})
	if err := h.Handle(ctx, FanoutEvent{PostID: 7, AuthorID: 2, CreatedAtMs: 12345}); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	// Followers 1, 3 and author 2 all see the post.
	for _, uid := range []int64{1, 2, 3} {
		members, err := mr.ZMembers(repository.FeedHomeKey(uid))
		if err != nil || len(members) != 1 || members[0] != "7" {
			t.Fatalf("feed:home:%d members = %v (err %v), want [7]", uid, members, err)
		}
		score, _ := rdb.ZScore(ctx, repository.FeedHomeKey(uid), "7").Result()
		if score != 12345 {
			t.Errorf("feed:home:%d score = %v, want 12345", uid, score)
		}
		ttl, _ := rdb.TTL(ctx, repository.FeedHomeKey(uid)).Result()
		if ttl <= 0 || ttl > repository.FeedHomeTTL {
			t.Errorf("feed:home:%d TTL = %v, want ~%v", uid, ttl, repository.FeedHomeTTL)
		}
	}

	// No one else's feed is touched.
	if mr.Exists(repository.FeedHomeKey(4)) {
		t.Error("feed:home:4 must not exist (4 is not a follower)")
	}
}

// TestFanoutCapsFeedAt500: Handle applies the 500-member cap (ZRemRangeByRank
// 0, -501), keeping the newest 500 posts (design §3.1 TimelineMaxSize).
func TestFanoutCapsFeedAt500(t *testing.T) {
	cache, mr := newFanoutEnv(t, nil)
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	key := repository.FeedHomeKey(1)
	// Pre-seed a full 500-member feed (oldest = 1 … newest = 500).
	for i := 1; i <= 500; i++ {
		_ = cache.ZAdd(ctx, key, float64(i), strconv.Itoa(i))
	}

	h := NewFanoutHandler(cache, &fakeUserClient{followers: map[int64][]int64{1: {}}})
	if err := h.Handle(ctx, FanoutEvent{PostID: 501, AuthorID: 1, CreatedAtMs: 501}); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	if n, err := rdb.ZCard(ctx, key).Result(); err != nil || n != 500 {
		t.Fatalf("feed:home:1 size = %d (err %v), want 500 after cap", n, err)
	}
	if _, err := rdb.ZScore(ctx, key, "501").Result(); err != nil {
		t.Error("newest post 501 must survive the cap")
	}
	if _, err := rdb.ZScore(ctx, key, "1").Result(); err == nil {
		t.Error("oldest post 1 must be trimmed by the cap")
	}
}

// TestFanoutNoFollowersStillPushesAuthor: with zero followers the author's own
// feed still receives the post (self-view + rebuild base).
func TestFanoutNoFollowersStillPushesAuthor(t *testing.T) {
	cache, mr := newFanoutEnv(t, nil)
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	h := NewFanoutHandler(cache, &fakeUserClient{followers: map[int64][]int64{2: {}}})
	if err := h.Handle(ctx, FanoutEvent{PostID: 9, AuthorID: 2, CreatedAtMs: 100}); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	if _, err := rdb.ZScore(ctx, repository.FeedHomeKey(2), "9").Result(); err != nil {
		t.Error("author's own feed must contain the post")
	}
	if mr.Exists(repository.FeedHomeKey(1)) {
		t.Error("feed:home:1 must not exist (no followers)")
	}
}

// TestFanoutPropagatesUserClientError: a User Service failure aborts the fanout
// instead of silently writing an incomplete feed.
func TestFanoutPropagatesUserClientError(t *testing.T) {
	cache, mr := newFanoutEnv(t, nil)
	ctx := context.Background()

	fail := &failFollowersClient{}
	h := NewFanoutHandler(cache, fail)
	if err := h.Handle(ctx, FanoutEvent{PostID: 1, AuthorID: 2, CreatedAtMs: 100}); err == nil {
		t.Fatal("Handle = nil, want error from GetFollowerIDs")
	}
	if mr.Exists(repository.FeedHomeKey(2)) {
		t.Error("nothing must be written when follower fetch fails")
	}
}

type failFollowersClient struct{ fakeUserClient }

func (failFollowersClient) GetFollowerIDs(context.Context, int64) ([]int64, error) {
	return nil, errors.New("user service unavailable")
}
