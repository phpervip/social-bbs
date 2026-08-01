// Package worker implements the P2 Kafka-driven fanout pipeline: consuming
// post.created (real-follower fanout) and user.follow-changed (backfill /
// cleanup), plus the outbox dispatcher (design §5.2/§5.3/§5.4).
package worker

import (
	"context"
	"strconv"

	"social-bbs/feed-service/internal/repository"
)

// FanoutEvent is the unit of fanout work decoded from post.created.
type FanoutEvent struct {
	PostID      int64
	AuthorID    int64
	CreatedAtMs int64
}

// BigVThreshold is the P2 stub for the big-V Pull branch (design D-A4): authors
// with more followers than this would switch to a pull model in P4. P2 stays
// Push-only, so the constant is retained but never consulted.
const BigVThreshold = 1000 // TODO(P4): fanout > threshold via feed:inbox read-time merge

// FanoutHandler pushes a created post into the author's real followers' home
// timelines plus the author's own (design §5.3 RealFollowersMode).
type FanoutHandler struct {
	cache repository.Cache
	users repository.UserClient
}

// NewFanoutHandler wires the fanout handler.
func NewFanoutHandler(cache repository.Cache, users repository.UserClient) *FanoutHandler {
	return &FanoutHandler{cache: cache, users: users}
}

// Handle fans ev out: follower ids come from user:followers:{author} (ZSet,
// gRPC backfill on miss); every follower AND the author get the post ZADDed
// into feed:home:{uid} with the created_at score, a slid 7d TTL, and the
// 500-member cap.
func (h *FanoutHandler) Handle(ctx context.Context, ev FanoutEvent) error {
	followerIDs, err := h.users.GetFollowerIDs(ctx, ev.AuthorID)
	if err != nil {
		return err
	}
	// The author's own feed always receives the post (self-view + rebuild base).
	targets := appendUnique(followerIDs, ev.AuthorID)
	for _, uid := range targets {
		key := repository.FeedHomeKey(uid)
		_ = h.cache.ZAdd(ctx, key, float64(ev.CreatedAtMs), strconv.FormatInt(ev.PostID, 10))
		_ = h.cache.Expire(ctx, key, repository.FeedHomeTTL)
		_ = h.cache.ZRemRangeByRank(ctx, key, 0, -(repository.TimelineMaxSize + 1))
	}
	return nil
}

// appendUnique returns ids with extra appended unless already present.
func appendUnique(ids []int64, extra int64) []int64 {
	for _, id := range ids {
		if id == extra {
			return ids
		}
	}
	return append(ids, extra)
}
