package repository

import "fmt"

// Redis key builders — keys must match the P1 contract exactly (plan §3.2).
func FeedHomeKey(userID int64) string   { return fmt.Sprintf("feed:home:%d", userID) }
func PostDetailKey(postID int64) string { return fmt.Sprintf("post:detail:%d", postID) }
func PostLikesKey(postID int64) string  { return fmt.Sprintf("post:likes:%d", postID) }
func FeedLockKey(userID int64) string   { return fmt.Sprintf("feed:lock:%d", userID) }

// P2 keys maintained by User Service (design §4.4 — names must match exactly).
func UserProfileKey(id int64) string   { return fmt.Sprintf("user:profile:%d", id) }
func UserFollowersKey(id int64) string { return fmt.Sprintf("user:followers:%d", id) }
func UserFollowingKey(id int64) string { return fmt.Sprintf("user:following:%d", id) }
