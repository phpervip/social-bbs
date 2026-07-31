// Package worker implements the in-process async fanout pipeline (plan D2).
// P2 replaces this with outbox + Kafka; P1 keeps a channel-backed single
// consumer goroutine inside the Feed Service process.
package worker

import (
	"context"
	"strconv"
	"sync"

	"social-bbs/feed-service/internal/repository"
)

// FanoutEvent is the unit of work consumed by the fanout worker.
type FanoutEvent struct {
	PostID      int64
	AuthorID    int64
	CreatedAtMs int64
}

// FanoutMode decides whether a post is pushed to followers' home timelines
// (plan §1.1 D5 — big-V pull threshold stub, default all-Push).
type FanoutMode interface {
	// ShouldPush reports whether authorID's posts must be PUSH-fanned out.
	ShouldPush(ctx context.Context, authorID int64) (bool, error)
}

// StubFanoutMode always returns PUSH (global fanout). P2 replaces it with a
// real follower-count query.
type StubFanoutMode struct{}

// ShouldPush implements FanoutMode: P1 fans out to all users, including the author.
func (StubFanoutMode) ShouldPush(context.Context, int64) (bool, error) { return true, nil }

// Enqueuer abstracts the async fanout queue for the post service.
type Enqueuer interface {
	Enqueue(FanoutEvent)
}

// Worker asynchronously pushes new posts into every user's feed:home ZSet.
// Channel capacity is 1024; when the queue is full, Enqueue falls back to a
// synchronous fanout so posts are never lost.
type Worker struct {
	ch       chan FanoutEvent
	mode     FanoutMode
	cache    repository.Cache
	users    repository.UserRepo
	wg       sync.WaitGroup
	mu       sync.Mutex
	stopOnce sync.Once
}

// NewWorker starts the single-consumer fanout goroutine.
func NewWorker(cache repository.Cache, users repository.UserRepo, mode FanoutMode) *Worker {
	w := &Worker{
		ch:    make(chan FanoutEvent, repository.FanoutQueueCapacity),
		mode:  mode,
		cache: cache,
		users: users,
	}
	w.wg.Add(1)
	go w.run()
	return w
}

// Enqueue queues an event; when the queue is full it fans out synchronously as
// a fallback.
func (w *Worker) Enqueue(ev FanoutEvent) {
	select {
	case w.ch <- ev:
	default:
		w.fanout(context.Background(), ev)
	}
}

func (w *Worker) run() {
	defer w.wg.Done()
	for ev := range w.ch {
		w.fanout(context.Background(), ev)
	}
}

// Stop drains the channel and waits for the consumer to finish. Safe to call
// multiple times. Callers must ensure no Enqueue happens after Stop (main shuts
// down the gRPC server first, draining in-flight RPCs).
func (w *Worker) Stop() {
	w.stopOnce.Do(func() {
		close(w.ch)
		w.wg.Wait()
	})
}

// fanout performs the global push: for every user, ZADD the post into
// feed:home:{uid}, slide the 7d TTL, and cap the ZSet at 500 members
// (ZREMRANGEBYRANK 0 -501 drops the oldest beyond 500).
func (w *Worker) fanout(ctx context.Context, ev FanoutEvent) {
	push, err := w.mode.ShouldPush(ctx, ev.AuthorID)
	if err != nil || !push {
		return
	}
	ids, err := w.users.ListIDs(ctx)
	if err != nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, uid := range ids {
		key := repository.FeedHomeKey(uid)
		_ = w.cache.ZAdd(ctx, key, float64(ev.CreatedAtMs), strconv.FormatInt(ev.PostID, 10))
		_ = w.cache.Expire(ctx, key, repository.FeedHomeTTL)
		_ = w.cache.ZRemRangeByRank(ctx, key, 0, -501)
	}
}
