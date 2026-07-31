package service

import (
	"context"
	"encoding/json"
	"strconv"

	"social-bbs/feed-service/internal/repository"
)

// TimelineService serves the home timeline (plan §3.3 GetHomeTimeline).
type TimelineService interface {
	GetHomeTimeline(ctx context.Context, userID, cursor int64, limit int) ([]*repository.Post, int64, bool, error)
}

type timelineService struct {
	posts repository.PostRepo
	likes repository.LikeRepo
	cache repository.Cache
}

// NewTimelineService wires the timeline service.
func NewTimelineService(posts repository.PostRepo, likes repository.LikeRepo, cache repository.Cache) TimelineService {
	return &timelineService{posts: posts, likes: likes, cache: cache}
}

// GetHomeTimeline implements the cache-first, rebuild-on-empty timeline contract:
//   - read feed:home:{uid} (ZREVRANGEBYSCORE, LIMIT limit+1) with cursor as the
//     exclusive max bound (0 → +inf)
//   - on hit: MGET post:detail:{id}, batch-fetch misses from MySQL, drop
//     missing/soft-deleted posts (server-side filtering), re-sort by ZSet order
//   - on miss (ZSet missing/empty): rebuild from the latest 50 posts under
//     feed:lock:{uid} (SET NX EX 5); if the lock is held elsewhere, fall back to
//     a direct MySQL query without rebuilding
func (s *timelineService) GetHomeTimeline(ctx context.Context, userID, cursor int64, limit int) ([]*repository.Post, int64, bool, error) {
	limit = clampLimit(limit)
	key := repository.FeedHomeKey(userID)

	ids, err := s.readTimeline(ctx, key, cursor, int64(limit+1))
	if err != nil {
		return nil, 0, false, err
	}
	if len(ids) == 0 {
		acquired, lerr := s.cache.SetNX(ctx, repository.FeedLockKey(userID), "1", repository.FeedLockTTL)
		if lerr != nil {
			return nil, 0, false, lerr
		}
		if !acquired {
			// Lock held elsewhere → serve from MySQL directly, no rebuild.
			posts, derr := s.posts.Latest(ctx, cursor, limit+1)
			if derr != nil {
				return nil, 0, false, derr
			}
			return s.finish(ctx, posts, userID, limit)
		}
		defer s.cache.Del(ctx, repository.FeedLockKey(userID))

		recent, rerr := s.posts.Latest(ctx, 0, repository.RebuildBatchSize)
		if rerr != nil {
			return nil, 0, false, rerr
		}
		for _, p := range recent {
			_ = s.cache.ZAdd(ctx, key, float64(p.CreatedAtMs()), strconv.FormatInt(p.ID, 10))
		}
		_ = s.cache.Expire(ctx, key, repository.FeedHomeTTL)

		ids, err = s.readTimeline(ctx, key, cursor, int64(limit+1))
		if err != nil {
			return nil, 0, false, err
		}
	}

	posts := s.fetchByIDs(ctx, ids, userID)
	return s.finish(ctx, posts, userID, limit)
}

// readTimeline returns ZSet member ids newest-first. cursor is exclusive
// (Redis "(" syntax) so the boundary post is not re-returned on the next page;
// cursor == 0 reads from +inf.
func (s *timelineService) readTimeline(ctx context.Context, key string, cursor int64, count int64) ([]string, error) {
	max := "+inf"
	if cursor > 0 {
		max = "(" + strconv.FormatInt(cursor, 10)
	}
	return s.cache.ZRevRangeByScore(ctx, key, max, "-inf", 0, count)
}

// fetchByIDs materialises posts preserving the ZSet order, dropping posts that
// are missing or soft-deleted (server-side filtering, never returned).
func (s *timelineService) fetchByIDs(ctx context.Context, ids []string, viewerID int64) []*repository.Post {
	n := len(ids)
	keys := make([]string, n)
	idNums := make([]int64, n)
	pos := make(map[int64]int, n)
	for i, idStr := range ids {
		id, _ := strconv.ParseInt(idStr, 10, 64)
		idNums[i] = id
		keys[i] = repository.PostDetailKey(id)
		if _, ok := pos[id]; !ok {
			pos[id] = i
		}
	}
	vals, _ := s.cache.MGet(ctx, keys...)

	ordered := make([]*repository.Post, n)
	var miss []int64
	for i, v := range vals {
		if v == "" {
			miss = append(miss, idNums[i])
			continue
		}
		var p repository.Post
		if err := json.Unmarshal([]byte(v), &p); err == nil {
			ordered[i] = &p
		} else {
			miss = append(miss, idNums[i])
		}
	}
	if len(miss) > 0 {
		dbPosts, err := s.posts.GetByIDs(ctx, miss)
		if err == nil {
			for _, p := range dbPosts {
				if i, ok := pos[p.ID]; ok && ordered[i] == nil {
					ordered[i] = p
				}
			}
		}
	}

	out := make([]*repository.Post, 0, n)
	for _, p := range ordered {
		if p != nil {
			out = append(out, p)
		}
	}
	return out
}

// finish trims to limit, computes next_cursor/has_more and attaches viewer context.
func (s *timelineService) finish(ctx context.Context, posts []*repository.Post, viewerID int64, limit int) ([]*repository.Post, int64, bool, error) {
	hasMore := len(posts) > limit
	if hasMore {
		posts = posts[:limit]
	}
	var nextCursor int64
	if n := len(posts); n > 0 {
		nextCursor = posts[n-1].CreatedAtMs()
	}
	s.attachLikedByViewer(ctx, posts, viewerID)
	return posts, nextCursor, hasMore, nil
}

func (s *timelineService) attachLikedByViewer(ctx context.Context, posts []*repository.Post, viewerID int64) {
	if viewerID <= 0 || len(posts) == 0 {
		return
	}
	ids := make([]int64, len(posts))
	for i, p := range posts {
		ids[i] = p.ID
	}
	liked, err := s.likes.LikedByUser(ctx, viewerID, ids)
	if err != nil {
		return
	}
	for _, p := range posts {
		p.LikedByViewer = liked[p.ID]
	}
}
