package worker

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"social-bbs/feed-service/internal/repository"
)

// fakeUserRepo is a minimal in-memory UserRepo for fanout tests.
type fakeUserRepo struct {
	ids []int64
}

func (f *fakeUserRepo) GetByID(context.Context, int64) (*repository.User, error) {
	return nil, repository.ErrUserNotFound
}

func (f *fakeUserRepo) ListIDs(context.Context) ([]int64, error) {
	return f.ids, nil
}

func newFanoutEnv(t *testing.T) (*miniredis.Miniredis, repository.Cache, *fakeUserRepo) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return mr, repository.NewRedisCache(rdb), &fakeUserRepo{ids: []int64{1, 2, 3}}
}

// TestStubFanoutModeReturnsPush: the P1 fanout mode stub always returns PUSH.
func TestStubFanoutModeReturnsPush(t *testing.T) {
	var m StubFanoutMode
	push, err := m.ShouldPush(context.Background(), 42)
	if err != nil {
		t.Fatalf("ShouldPush: %v", err)
	}
	if !push {
		t.Error("StubFanoutMode.ShouldPush = false, want true (always PUSH in P1)")
	}
}

// TestFanoutPushesToAllUsers: a new post is ZADDed into every user's
// feed:home ZSet (including the author), with the created_at score and a 7d TTL.
func TestFanoutPushesToAllUsers(t *testing.T) {
	mr, cache, users := newFanoutEnv(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	w := NewWorker(cache, users, StubFanoutMode{})
	defer w.Stop()

	w.Enqueue(FanoutEvent{PostID: 7, AuthorID: 2, CreatedAtMs: 12345})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		allDone := true
		for _, uid := range []int64{1, 2, 3} {
			members, err := mr.ZMembers(repository.FeedHomeKey(uid))
			// key may not exist yet while the consumer is still draining
			if err != nil || len(members) != 1 || members[0] != "7" {
				allDone = false
				break
			}
		}
		if allDone {
			score, _ := rdb.ZScore(context.Background(), "feed:home:1", "7").Result()
			if score != 12345 {
				t.Errorf("feed:home:1 score = %v, want 12345", score)
			}
			ttl, _ := rdb.TTL(context.Background(), "feed:home:1").Result()
			if ttl <= 0 || ttl > repository.FeedHomeTTL {
				t.Errorf("feed:home:1 TTL = %v, want ~%v", ttl, repository.FeedHomeTTL)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Error("fanout did not reach all users within timeout")
}

// TestFanoutSyncFallbackWhenChannelFull: when the queue is full, Enqueue fans
// out synchronously instead of dropping the event.
func TestFanoutSyncFallbackWhenChannelFull(t *testing.T) {
	mr, cache, users := newFanoutEnv(t)

	// Build a worker with a full channel and NO consumer goroutine so the
	// buffered send must fail and the synchronous fallback runs.
	w := &Worker{
		ch:    make(chan FanoutEvent, 1),
		mode:  StubFanoutMode{},
		cache: cache,
		users: users,
	}

	w.Enqueue(FanoutEvent{PostID: 1, AuthorID: 1, CreatedAtMs: 100}) // buffered
	w.Enqueue(FanoutEvent{PostID: 2, AuthorID: 1, CreatedAtMs: 200}) // channel full → sync fallback

	members, err := mr.ZMembers(repository.FeedHomeKey(1))
	if err != nil {
		t.Fatalf("ZMembers: %v", err)
	}
	if len(members) != 1 || members[0] != "2" {
		t.Errorf("sync fallback did not fanout post 2; feed:home:1 members = %v", members)
	}
}

// TestFanoutRespectsModeDecision: when the mode says "no push", nothing is written.
func TestFanoutRespectsModeDecision(t *testing.T) {
	mr, cache, users := newFanoutEnv(t)
	w := NewWorker(cache, users, noPushMode{})
	defer w.Stop()

	w.Enqueue(FanoutEvent{PostID: 9, AuthorID: 1, CreatedAtMs: 100})

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if mr.Exists(repository.FeedHomeKey(1)) {
			t.Error("feed:home:1 should not exist when mode declines to push")
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// noPushMode is a FanoutMode that always declines (exercises the mode branch).
type noPushMode struct{}

func (noPushMode) ShouldPush(context.Context, int64) (bool, error) { return false, nil }
