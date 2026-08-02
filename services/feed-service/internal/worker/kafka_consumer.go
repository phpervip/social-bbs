package worker

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"

	kafkago "github.com/segmentio/kafka-go"

	"social-bbs/feed-service/internal/kafka"
	"social-bbs/feed-service/internal/repository"
)

// Consumer runs the two P2 Kafka consumer loops (design §3.2):
//   - feed-fanout group: post.created -> FanoutHandler
//   - feed-timeline group: user.follow-changed -> follow backfill / unfollow cleanup
type Consumer struct {
	fanout *FanoutHandler
	posts  repository.PostRepo
	cache  repository.Cache
	logger *slog.Logger
}

// NewConsumer wires the Kafka consumers.
func NewConsumer(fanout *FanoutHandler, posts repository.PostRepo, cache repository.Cache, logger *slog.Logger) *Consumer {
	return &Consumer{fanout: fanout, posts: posts, cache: cache, logger: logger}
}

// RunFanout consumes post.created until ctx is cancelled.
func (c *Consumer) RunFanout(ctx context.Context, reader *kafkago.Reader) error {
	defer reader.Close()
	return c.run(ctx, reader, c.handlePostCreated)
}

// RunTimeline consumes user.follow-changed until ctx is cancelled.
func (c *Consumer) RunTimeline(ctx context.Context, reader *kafkago.Reader) error {
	defer reader.Close()
	return c.run(ctx, reader, c.handleFollowChanged)
}

type messageHandler func(ctx context.Context, value []byte) error

func (c *Consumer) run(ctx context.Context, reader *kafkago.Reader, handle messageHandler) error {
	for {
		msg, err := reader.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			c.logger.Error("kafka read failed", "error", err)
			sleepCtx(ctx, repository.OutboxPollInterval)
			continue
		}
		if err := handle(ctx, msg.Value); err != nil {
			c.logger.Error("kafka message handling failed", "topic", msg.Topic, "error", err)
		}
	}
}

func (c *Consumer) handlePostCreated(ctx context.Context, value []byte) error {
	var ev kafka.PostCreatedEvent
	if err := json.Unmarshal(value, &ev); err != nil {
		return err
	}
	return c.fanout.Handle(ctx, FanoutEvent{PostID: ev.PostID, AuthorID: ev.UserID, CreatedAtMs: ev.CreatedAt})
}

func (c *Consumer) handleFollowChanged(ctx context.Context, value []byte) error {
	var ev kafka.FollowChangedEvent
	if err := json.Unmarshal(value, &ev); err != nil {
		return err
	}
	if ev.FollowerID <= 0 || ev.FolloweeID <= 0 || ev.FollowerID == ev.FolloweeID {
		return nil
	}
	switch ev.Action {
	case "follow":
		return c.backfill(ctx, ev.FollowerID, ev.FolloweeID)
	case "unfollow":
		return c.cleanup(ctx, ev.FollowerID, ev.FolloweeID)
	default:
		return nil // unknown action: ignore (forward compatible)
	}
}

// backfill pushes the followee's recent posts into the follower's feed:home
// (design §5.4 follow: LatestByAuthor 50 -> ZADD + TTL + cap).
func (c *Consumer) backfill(ctx context.Context, followerID, followeeID int64) error {
	posts, err := c.posts.LatestByAuthor(ctx, followeeID, 0, repository.RebuildBatchSize)
	if err != nil {
		return err
	}
	if len(posts) == 0 {
		return nil
	}
	key := repository.FeedHomeKey(followerID)
	for _, p := range posts {
		_ = c.cache.ZAdd(ctx, key, float64(p.CreatedAtMs()), strconv.FormatInt(p.ID, 10))
	}
	_ = c.cache.Expire(ctx, key, repository.FeedHomeTTL)
	_ = c.cache.ZRemRangeByRank(ctx, key, 0, -(repository.TimelineMaxSize + 1))
	return nil
}

// cleanup removes the followee's posts from the follower's feed:home
// (design §5.4 unfollow: ZREM by post id).
func (c *Consumer) cleanup(ctx context.Context, followerID, followeeID int64) error {
	posts, err := c.posts.LatestByAuthor(ctx, followeeID, 0, repository.TimelineMaxSize)
	if err != nil {
		return err
	}
	key := repository.FeedHomeKey(followerID)
	for _, p := range posts {
		_ = c.cache.ZRem(ctx, key, strconv.FormatInt(p.ID, 10))
	}
	return nil
}
