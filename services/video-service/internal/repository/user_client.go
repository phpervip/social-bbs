package repository

import (
	"context"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	userpb "social-bbs/video-service/proto/gen/userpb"
)

const (
	userRPCTimeout = 5 * time.Second
	followPageSize = 500
	maxFollowUsers = 5000
)

// UserClient talks to User Service (gRPC :9001) for the followers_only
// visibility check (does viewer follow the uploader?).
type UserClient interface {
	// IsFollowing reports whether followerID follows followeeID.
	IsFollowing(ctx context.Context, followerID, followeeID uint64) (bool, error)
	// Close releases the underlying gRPC connection (call once at shutdown).
	Close() error
}

type userClient struct {
	addr   string
	mu     sync.Mutex
	conn   *grpc.ClientConn
	client userpb.UserServiceClient
}

// NewUserClient returns a UserClient with a lazy gRPC connection (dialed on
// first use, auto-reconnecting) and waitForReady calls.
func NewUserClient(addr string) UserClient {
	return &userClient{addr: addr}
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

func (c *userClient) getClient(ctx context.Context) (userpb.UserServiceClient, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client != nil {
		return c.client, nil
	}
	conn, err := grpc.NewClient(c.addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.WaitForReady(true)),
	)
	if err != nil {
		return nil, err
	}
	c.conn = conn
	c.client = userpb.NewUserServiceClient(conn)
	return c.client, nil
}

// IsFollowing pages the viewer's following list and reports whether the
// uploader is among them.
func (c *userClient) IsFollowing(ctx context.Context, followerID, followeeID uint64) (bool, error) {
	client, err := c.getClient(ctx)
	if err != nil {
		return false, err
	}
	var (
		cursor  int64
		hasMore = true
	)
	for hasMore {
		rpcCtx, cancel := context.WithTimeout(ctx, userRPCTimeout)
		resp, err := client.GetFollowing(rpcCtx, &userpb.GetFollowingRequest{
			UserId: int64(followerID),
			Cursor: cursor,
			Limit:  followPageSize,
		})
		cancel()
		if err != nil {
			return false, err
		}
		for _, u := range resp.GetUsers() {
			if u.GetId() == int64(followeeID) {
				return true, nil
			}
		}
		cursor = resp.GetNextCursor()
		hasMore = resp.GetHasMore() && len(resp.GetUsers()) > 0
		if cursor <= 0 {
			hasMore = false
		}
	}
	return false, nil
}