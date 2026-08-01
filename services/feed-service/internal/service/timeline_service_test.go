package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"social-bbs/feed-service/internal/repository"
)

// fakePostRepo is an in-memory PostRepo for timeline tests.
type fakePostRepo struct {
	posts  map[int64]*repository.Post
	latest []*repository.Post
}

func (f *fakePostRepo) Create(context.Context, *gorm.DB, int64, string, string) (*repository.Post, error) {
	return nil, nil
}
func (f *fakePostRepo) WithTx(_ context.Context, fn func(tx *gorm.DB) error) error {
	return fn(nil) // in-memory: no real tx
}
func (f *fakePostRepo) GetByID(_ context.Context, id int64) (*repository.Post, error) {
	if p, ok := f.posts[id]; ok {
		return p, nil
	}
	return nil, repository.ErrPostNotFound
}
func (f *fakePostRepo) GetByIDs(_ context.Context, ids []int64) ([]*repository.Post, error) {
	var out []*repository.Post
	for _, id := range ids {
		if p, ok := f.posts[id]; ok {
			out = append(out, p)
		}
	}
	return out, nil
}
func (f *fakePostRepo) Latest(context.Context, int64, int) ([]*repository.Post, error) {
	return f.latest, nil
}
func (f *fakePostRepo) LatestByAuthor(context.Context, int64, int64, int) ([]*repository.Post, error) {
	return nil, nil
}
func (f *fakePostRepo) LatestByAuthors(_ context.Context, authorIDs []int64, limit int) ([]*repository.Post, error) {
	// In-memory simplification: author filtering is covered by repo tests.
	_ = authorIDs
	if limit > 0 && len(f.latest) > limit {
		return f.latest[:limit], nil
	}
	return f.latest, nil
}
func (f *fakePostRepo) Search(context.Context, string, int64, int) ([]*repository.Post, error) {
	return nil, nil
}
func (f *fakePostRepo) SoftDelete(context.Context, int64) error { return nil }

func newTimelineEnv(t *testing.T) (repository.Cache, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return repository.NewRedisCache(rdb), mr
}

func mustMarshal(t *testing.T, p *repository.Post) string {
	t.Helper()
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal post: %v", err)
	}
	return string(b)
}

func seedCachePosts(t *testing.T, cache repository.Cache, posts []*repository.Post, userID int64) {
	t.Helper()
	ctx := context.Background()
	for _, p := range posts {
		_ = cache.ZAdd(ctx, repository.FeedHomeKey(userID), float64(p.CreatedAtMs()), jsonNum(p.ID))
		_ = cache.Set(ctx, repository.PostDetailKey(p.ID), mustMarshal(t, p), time.Minute)
	}
}

func jsonNum(id int64) string {
	b, _ := json.Marshal(id)
	return string(b)
}

// TestTimelineCacheHit: ZSet + post:detail cache present → served from cache,
// ordered newest-first, with next_cursor = last returned created_at.
func TestTimelineCacheHit(t *testing.T) {
	cache, mr := newTimelineEnv(t)
	ctx := context.Background()

	p1 := &repository.Post{ID: 1, UserID: 2, Username: "bob", Content: "a", CreatedAt: time.UnixMilli(2000)}
	p2 := &repository.Post{ID: 2, UserID: 1, Username: "alice", Content: "b", CreatedAt: time.UnixMilli(1000)}
	seedCachePosts(t, cache, []*repository.Post{p1, p2}, 7)

	svc := NewTimelineService(&fakePostRepo{posts: map[int64]*repository.Post{}}, &fakeLikeRepo{}, cache, &fakeUserClient{})
	posts, next, hasMore, err := svc.GetHomeTimeline(ctx, 7, 0, 0)
	if err != nil {
		t.Fatalf("GetHomeTimeline: %v", err)
	}
	if len(posts) != 2 || posts[0].ID != 1 || posts[1].ID != 2 {
		t.Fatalf("posts = %+v, want [1 2] newest-first", postIDs(posts))
	}
	if next != 1000 {
		t.Errorf("next_cursor = %d, want 1000", next)
	}
	if hasMore {
		t.Error("has_more = true, want false")
	}
	// Cache path must NOT have rebuilt (no feed:lock key ever created).
	if mr.Exists(repository.FeedLockKey(7)) {
		t.Error("feed:lock should not exist on cache hit")
	}
}

// TestTimelineDropsMissingAndDeleted: ids without a live post are filtered out.
func TestTimelineDropsMissingAndDeleted(t *testing.T) {
	cache, _ := newTimelineEnv(t)
	ctx := context.Background()

	p1 := &repository.Post{ID: 1, UserID: 1, Content: "x", CreatedAt: time.UnixMilli(3000)}
	p3 := &repository.Post{ID: 3, UserID: 1, Content: "z", CreatedAt: time.UnixMilli(1000)}
	// ZSet has 1, 2 (no cache, no DB row), 3.
	_ = cache.ZAdd(ctx, "feed:home:7", 3000, "1")
	_ = cache.ZAdd(ctx, "feed:home:7", 2000, "2")
	_ = cache.ZAdd(ctx, "feed:home:7", 1000, "3")
	_ = cache.Set(ctx, "post:detail:1", mustMarshal(t, p1), time.Minute)
	_ = cache.Set(ctx, "post:detail:3", mustMarshal(t, p3), time.Minute)

	svc := NewTimelineService(&fakePostRepo{posts: map[int64]*repository.Post{1: p1, 3: p3}}, &fakeLikeRepo{}, cache, &fakeUserClient{})
	posts, _, _, err := svc.GetHomeTimeline(ctx, 7, 0, 0)
	if err != nil {
		t.Fatalf("GetHomeTimeline: %v", err)
	}
	if len(posts) != 2 || posts[0].ID != 1 || posts[1].ID != 3 {
		t.Fatalf("posts = %+v, want [1 3] (id 2 dropped)", postIDs(posts))
	}
}

// TestTimelineRebuildOnEmptyZSet: missing ZSet → rebuild from MySQL latest 50,
// serve from the rebuilt set, set the 7d TTL, and release the lock.
func TestTimelineRebuildOnEmptyZSet(t *testing.T) {
	cache, mr := newTimelineEnv(t)
	ctx := context.Background()

	p5 := &repository.Post{ID: 5, UserID: 1, Content: "5", CreatedAt: time.UnixMilli(5000)}
	p4 := &repository.Post{ID: 4, UserID: 1, Content: "4", CreatedAt: time.UnixMilli(4000)}
	p3 := &repository.Post{ID: 3, UserID: 1, Content: "3", CreatedAt: time.UnixMilli(3000)}
	repo := &fakePostRepo{
		posts:  map[int64]*repository.Post{5: p5, 4: p4, 3: p3},
		latest: []*repository.Post{p5, p4, p3},
	}

	svc := NewTimelineService(repo, &fakeLikeRepo{}, cache, &fakeUserClient{})
	posts, next, hasMore, err := svc.GetHomeTimeline(ctx, 9, 0, 0)
	if err != nil {
		t.Fatalf("GetHomeTimeline: %v", err)
	}
	if len(posts) != 3 || posts[0].ID != 5 || posts[2].ID != 3 {
		t.Fatalf("posts = %+v, want [5 4 3]", postIDs(posts))
	}
	if next != 3000 {
		t.Errorf("next_cursor = %d, want 3000", next)
	}
	if hasMore {
		t.Error("has_more = true, want false")
	}
	if got, err := mr.ZMembers(repository.FeedHomeKey(9)); err != nil || len(got) != 3 {
		t.Errorf("feed:home:9 members = %v (err %v), want 3 (rebuilt)", got, err)
	}
	if mr.Exists(repository.FeedLockKey(9)) {
		t.Error("feed:lock:9 must be released after rebuild")
	}
	ttlClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer ttlClient.Close()
	ttl, _ := ttlClient.TTL(ctx, repository.FeedHomeKey(9)).Result()
	if ttl <= 0 || ttl > repository.FeedHomeTTL {
		t.Errorf("feed:home:9 TTL = %v, want ~%v", ttl, repository.FeedHomeTTL)
	}
}

// TestTimelineLockContendedFallsBackToMySQL: when feed:lock is held, serve from
// MySQL directly WITHOUT rebuilding.
func TestTimelineLockContendedFallsBackToMySQL(t *testing.T) {
	cache, mr := newTimelineEnv(t)
	ctx := context.Background()

	_ = mr.Set(repository.FeedLockKey(5), "1")

	p1 := &repository.Post{ID: 1, UserID: 1, Content: "x", CreatedAt: time.UnixMilli(1000)}
	repo := &fakePostRepo{
		posts:  map[int64]*repository.Post{1: p1},
		latest: []*repository.Post{p1},
	}

	svc := NewTimelineService(repo, &fakeLikeRepo{}, cache, &fakeUserClient{})
	posts, _, _, err := svc.GetHomeTimeline(ctx, 5, 0, 0)
	if err != nil {
		t.Fatalf("GetHomeTimeline: %v", err)
	}
	if len(posts) != 1 || posts[0].ID != 1 {
		t.Fatalf("posts = %+v, want [1]", postIDs(posts))
	}
	if mr.Exists(repository.FeedHomeKey(5)) {
		t.Error("feed:home:5 must NOT be rebuilt when the lock is held")
	}
}

// TestTimelineHasMore: a ZSet larger than the limit returns has_more with a
// next_cursor equal to the last returned post's created_at.
func TestTimelineHasMore(t *testing.T) {
	cache, _ := newTimelineEnv(t)
	ctx := context.Background()

	var posts []*repository.Post
	byID := make(map[int64]*repository.Post)
	for i := 1; i <= 21; i++ {
		p := &repository.Post{ID: int64(i), UserID: 1, Content: "c", CreatedAt: time.UnixMilli(int64(i) * 1000)}
		posts = append(posts, p)
		byID[p.ID] = p
	}
	seedCachePosts(t, cache, posts, 3)

	svc := NewTimelineService(&fakePostRepo{posts: byID}, &fakeLikeRepo{}, cache, &fakeUserClient{})
	got, next, hasMore, err := svc.GetHomeTimeline(ctx, 3, 0, 0)
	if err != nil {
		t.Fatalf("GetHomeTimeline: %v", err)
	}
	if len(got) != 20 {
		t.Fatalf("len(posts) = %d, want 20 (limit clamped from 0 to default 20)", len(got))
	}
	if !hasMore {
		t.Error("has_more = false, want true")
	}
	if next != 2000 {
		t.Errorf("next_cursor = %d, want 2000 (20th post, newest-first)", next)
	}
}

func postIDs(posts []*repository.Post) []int64 {
	ids := make([]int64, len(posts))
	for i, p := range posts {
		ids[i] = p.ID
	}
	return ids
}
