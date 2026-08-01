package repository

import (
	"context"
	"encoding/json"
	"strconv"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	userpb "social-bbs/feed-service/proto/gen/userpb"
)

const (
	userRPCTimeout  = 5 * time.Second // bounds waitForReady + RPC latency
	followPageLimit = 500             // GetFollowers/GetFollowing page size
	maxFollowUsers  = 5000            // hard cap on paged follow lists
)

// userProfileCache mirrors the User Service's user:profile:{id} JSON (design
// §4.4). IDs are json.Number so both the protojson string form ("1") and the
// Jackson number form (1) written by User Service parse correctly.
type userProfileCache struct {
	ID          json.Number `json:"id"`
	Username    string      `json:"username"`
	DisplayName string      `json:"display_name"`
	AvatarURL   string      `json:"avatar_url"`
}

type userClient struct {
	addr   string
	cache  Cache
	mu     sync.Mutex
	conn   *grpc.ClientConn
	client userpb.UserServiceClient
	dial   func(ctx context.Context, addr string) (*grpc.ClientConn, error)
}

// NewUserClient returns a UserClient with a lazy gRPC connection (dialed on
// first use, auto-reconnecting) and waitForReady calls (design §5.5).
func NewUserClient(addr string, cache Cache) UserClient {
	return &userClient{addr: addr, cache: cache, dial: dialUserConn}
}

// Close releases the gRPC connection (graceful shutdown).
func (c *userClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		c.client = nil
		return err
	}
	return nil
}

func dialUserConn(_ context.Context, addr string) (*grpc.ClientConn, error) {
	return grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.WaitForReady(true)),
	)
}

// getClient lazily dials once; the returned ClientConn reconnects internally.
func (c *userClient) getClient(ctx context.Context) (userpb.UserServiceClient, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client != nil {
		return c.client, nil
	}
	conn, err := c.dial(ctx, c.addr)
	if err != nil {
		return nil, err
	}
	c.conn = conn
	c.client = userpb.NewUserServiceClient(conn)
	return c.client, nil
}

func (c *userClient) GetProfile(ctx context.Context, id int64) (*User, error) {
	key := UserProfileKey(id)
	if raw, err := c.cache.Get(ctx, key); err == nil {
		if u := parseProfileCache(raw); u != nil {
			return u, nil
		}
	}
	client, err := c.getClient(ctx)
	if err != nil {
		return nil, err
	}
	rpcCtx, cancel := context.WithTimeout(ctx, userRPCTimeout)
	defer cancel()
	resp, err := client.GetProfile(rpcCtx, &userpb.GetProfileRequest{UserId: id})
	if err != nil {
		return nil, mapGRPCErr(err)
	}
	u := toAuthor(resp.GetUser())
	if u != nil {
		_ = c.cache.Set(ctx, key, marshalProfileCache(u), UserProfileTTL)
	}
	return u, nil
}

// GetProfiles batch-resolves profiles (MGET user:profile → gRPC per miss).
// The map holds every successfully resolved id; err is set when any single
// fetch failed (callers use the partial map and may ignore the error).
func (c *userClient) GetProfiles(ctx context.Context, ids []int64) (map[int64]*User, error) {
	out := make(map[int64]*User, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = UserProfileKey(id)
	}
	vals, err := c.cache.MGet(ctx, keys...)
	if err != nil {
		vals = make([]string, len(ids))
	}
	var miss []int64
	for i, id := range ids {
		if i < len(vals) && vals[i] != "" {
			if u := parseProfileCache(vals[i]); u != nil {
				out[id] = u
				continue
			}
		}
		miss = append(miss, id)
	}
	var firstErr error
	for _, id := range miss {
		u, err := c.GetProfile(ctx, id)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if u != nil {
			out[id] = u
		}
	}
	return out, firstErr
}

func (c *userClient) GetFollowerIDs(ctx context.Context, userID int64) ([]int64, error) {
	return c.getRelationIDs(ctx, UserFollowersKey(userID), func(rpcCtx context.Context, cursor int64, limit int32) ([]*userpb.User, int64, bool, error) {
		client, err := c.getClient(rpcCtx)
		if err != nil {
			return nil, 0, false, err
		}
		resp, err := client.GetFollowers(rpcCtx, &userpb.GetFollowersRequest{UserId: userID, Cursor: cursor, Limit: limit})
		if err != nil {
			return nil, 0, false, mapGRPCErr(err)
		}
		return resp.GetUsers(), resp.GetNextCursor(), resp.GetHasMore(), nil
	})
}

func (c *userClient) GetFollowingIDs(ctx context.Context, userID int64) ([]int64, error) {
	return c.getRelationIDs(ctx, UserFollowingKey(userID), func(rpcCtx context.Context, cursor int64, limit int32) ([]*userpb.User, int64, bool, error) {
		client, err := c.getClient(rpcCtx)
		if err != nil {
			return nil, 0, false, err
		}
		resp, err := client.GetFollowing(rpcCtx, &userpb.GetFollowingRequest{UserId: userID, Cursor: cursor, Limit: limit})
		if err != nil {
			return nil, 0, false, mapGRPCErr(err)
		}
		return resp.GetUsers(), resp.GetNextCursor(), resp.GetHasMore(), nil
	})
}

// fetchFollowPage is one page of a follow list RPC.
type fetchFollowPage func(ctx context.Context, cursor int64, limit int32) ([]*userpb.User, int64, bool, error)

// getRelationIDs reads the ZSet cache first; on miss it pages the gRPC follow
// list and backfills the ZSet (5min TTL).
func (c *userClient) getRelationIDs(ctx context.Context, key string, fetch fetchFollowPage) ([]int64, error) {
	members, err := c.cache.ZRange(ctx, key, 0, -1)
	if err == nil && len(members) > 0 {
		return parseIDs(members), nil
	}
	users, err := c.fetchFollowPages(ctx, fetch)
	if err != nil {
		return nil, err
	}
	if len(users) > 0 {
		for i, u := range users {
			_ = c.cache.ZAdd(ctx, key, float64(i), strconv.FormatInt(u.GetId(), 10))
		}
		_ = c.cache.Expire(ctx, key, UserFollowsTTL)
	}
	return idsOf(users), nil
}

func (c *userClient) fetchFollowPages(ctx context.Context, fetch fetchFollowPage) ([]*userpb.User, error) {
	var (
		users   []*userpb.User
		cursor  int64
		hasMore = true
	)
	for hasMore && len(users) < maxFollowUsers {
		rpcCtx, cancel := context.WithTimeout(ctx, userRPCTimeout)
		page, next, more, err := fetch(rpcCtx, cursor, followPageLimit)
		cancel()
		if err != nil {
			return nil, err
		}
		users = append(users, page...)
		cursor, hasMore = next, more
	}
	return users, nil
}

func toAuthor(u *userpb.User) *User {
	if u == nil {
		return nil
	}
	return &User{ID: u.GetId(), Username: u.GetUsername(), DisplayName: u.GetDisplayName(), AvatarURL: u.GetAvatarUrl()}
}

func parseProfileCache(raw string) *User {
	var c userProfileCache
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return nil
	}
	id, err := c.ID.Int64()
	if err != nil || id <= 0 {
		return nil
	}
	return &User{ID: id, Username: c.Username, DisplayName: c.DisplayName, AvatarURL: c.AvatarURL}
}

func marshalProfileCache(u *User) string {
	b, err := json.Marshal(userProfileCache{
		ID:          json.Number(strconv.FormatInt(u.ID, 10)),
		Username:    u.Username,
		DisplayName: u.DisplayName,
		AvatarURL:   u.AvatarURL,
	})
	if err != nil {
		return ""
	}
	return string(b)
}

func parseIDs(members []string) []int64 {
	out := make([]int64, 0, len(members))
	for _, m := range members {
		if id, err := strconv.ParseInt(m, 10, 64); err == nil {
			out = append(out, id)
		}
	}
	return out
}

func idsOf(users []*userpb.User) []int64 {
	out := make([]int64, 0, len(users))
	for _, u := range users {
		out = append(out, u.GetId())
	}
	return out
}

func mapGRPCErr(err error) error {
	if status.Code(err) == codes.NotFound {
		return ErrUserNotFound
	}
	return err
}
