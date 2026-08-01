// Package kafka owns the P2 Kafka contract: topic/group names and the JSON
// event payloads exchanged with User Service (plan T4 §6, design §3.2).
package kafka

// Topic and consumer group names (design §3.2, R6 — dot-separated snake_case).
const (
	TopicPostCreated   = "post.created"
	TopicFollowChanged = "user.follow-changed"
	GroupFeedFanout    = "feed-fanout"
	GroupFeedTimeline  = "feed-timeline"
)

// PostCreatedEvent is the payload of post.created (produced by Feed's outbox
// dispatcher, consumed by the feed-fanout group).
type PostCreatedEvent struct {
	PostID    int64  `json:"post_id"`
	UserID    int64  `json:"user_id"`
	Content   string `json:"content"`
	CreatedAt int64  `json:"created_at"` // unix ms
}

// FollowChangedEvent is the payload of user.follow-changed (produced by User
// Service, consumed by the feed-timeline group).
type FollowChangedEvent struct {
	FollowerID int64  `json:"follower_id"`
	FolloweeID int64  `json:"followee_id"`
	Action     string `json:"action"`     // "follow" | "unfollow"
	CreatedAt  int64  `json:"created_at"` // unix ms
}
