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
	users repository.UserClient
}

// NewTimelineService wires the timeline service.
func NewTimelineService(posts repository.PostRepo, likes repository.LikeRepo, cache repository.Cache, users repository.UserClient) TimelineService {
	return &timelineService{posts: posts, likes: likes, cache: cache, users: users}
}

// GetHomeTimeline implements the cache-first, rebuild-on-empty timeline contract:
//   - read feed:home:{uid} (ZREVRANGEBYSCORE, LIMIT limit+1) with cursor as the
//     exclusive max bound (0 → +inf)
//   - on hit: MGET post:detail:{id}, batch-fetch misses from MySQL, drop
//     missing/soft-deleted posts (server-side filtering), re-sort by ZSet order
//   - on miss (ZSet missing/empty): rebuild from the following list
//     (user:following:{uid} ZSet + LatestByAuthors, including self — design
//     §5.5) under feed:lock:{uid} (SET NX EX 5); if the lock is held elsewhere,
//     fall back to a direct MySQL query without rebuilding
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
			enrichAuthors(ctx, s.users, posts)
			return s.finish(ctx, posts, userID, limit)
		}
		defer s.cache.Del(ctx, repository.FeedLockKey(userID))

		recent, rerr := s.rebuildSource(ctx, userID)
		if rerr != nil {
			return nil, 0, false, rerr
		}
		for _, p := range recent {
			_ = s.cache.ZAdd(ctx, key, float64(p.CreatedAtMs()), strconv.FormatInt(p.ID, 10))
		}
		if len(recent) > 0 {
			_ = s.cache.Expire(ctx, key, repository.FeedHomeTTL)
		}

		ids, err = s.readTimeline(ctx, key, cursor, int64(limit+1))
		if err != nil {
			return nil, 0, false, err
		}
	}

	posts := s.fetchByIDs(ctx, ids, userID)
	enrichAuthors(ctx, s.users, posts)
	return s.finish(ctx, posts, userID, limit)
}

// rebuildSource returns the posts a timeline is rebuilt from: the newest posts
// by the user's following list (user:following:{uid} ZSet via UserClient) plus
// the user themself. When User Service is unreachable it degrades to the P1
// global latest query.
func (s *timelineService) rebuildSource(ctx context.Context, userID int64) ([]*repository.Post, error) {
	following, err := s.users.GetFollowingIDs(ctx, userID)
	if err != nil {
		return s.posts.Latest(ctx, 0, repository.RebuildBatchSize)
	}
	return s.posts.LatestByAuthors(ctx, appendUnique(following, userID), repository.RebuildBatchSize)
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

// enrichAuthors fills Username/DisplayName/AvatarURL from User Service
// (user:profile:{id} MGET + gRPC misses). Best effort: enrichment failures
// leave the fields empty rather than failing the timeline.
func enrichAuthors(ctx context.Context, users repository.UserClient, posts []*repository.Post) {
	ids := make(map[int64]struct{}, len(posts))
	for _, p := range posts {
		if p != nil && p.UserID > 0 {
			ids[p.UserID] = struct{}{}
		}
	}
	if len(ids) == 0 {
		return
	}
	idList := make([]int64, 0, len(ids))
	for id := range ids {
		idList = append(idList, id)
	}
	resolved, _ := users.GetProfiles(ctx, idList)
	for _, p := range posts {
		if p == nil {
			continue
		}
		if u, ok := resolved[p.UserID]; ok {
			p.Username = u.Username
			p.DisplayName = u.DisplayName
			p.AvatarURL = u.AvatarURL
		}
	}
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

// appendUnique returns ids with extra appended unless already present.
func appendUnique(ids []int64, extra int64) []int64 {
	for _, id := range ids {
		if id == extra {
			return ids
		}
	}
	return append(ids, extra)
}
