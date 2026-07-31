package repository

import "time"

// Exact P1 limits and TTLs (plan §3.2 / §3.3 — reproduce verbatim).
const (
	TimelineMaxSize     = 500  // per-user feed:home ZSet cap
	RebuildBatchSize    = 50   // timeline rebuild fetch size
	DefaultPageLimit    = 20   // cursor pagination default
	MaxPageLimit        = 50   // cursor pagination max
	MaxContentRunes     = 280  // CreatePost content limit (UTF-8 runes)
	MaxCommentRunes     = 500  // AddComment content limit (UTF-8 runes)
	FanoutQueueCapacity = 1024 // fanout worker channel cap
)

var (
	FeedHomeTTL   = 7 * 24 * time.Hour // feed:home:{uid} sliding TTL
	PostDetailTTL = 30 * time.Minute   // post:detail:{id} TTL
	PostLikesTTL  = 30 * time.Minute   // post:likes:{id} TTL
	FeedLockTTL   = 5 * time.Second    // feed:lock:{uid} rebuild lock TTL
)
