package repository

import "fmt"

// Redis key builders — keys must match the P1 contract exactly (plan §3.2).
func FeedHomeKey(userID int64) string    { return fmt.Sprintf("feed:home:%d", userID) }
func PostDetailKey(postID int64) string  { return fmt.Sprintf("post:detail:%d", postID) }
func PostLikesKey(postID int64) string   { return fmt.Sprintf("post:likes:%d", postID) }
func FeedLockKey(userID int64) string    { return fmt.Sprintf("feed:lock:%d", userID) }
