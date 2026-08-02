package repository

import "time"

// Exact P3 limits and TTLs.
const (
	DefaultPageLimit = 20 // cursor pagination default
	MaxPageLimit     = 50 // cursor pagination max
	DefaultChunkSize = 5 * 1024 * 1024 // 5MB default chunk size
	MaxTitleRunes    = 255
	MaxDescRunes     = 1000
)

var (
	UploadInitLockTTL = time.Hour        // upload:init:lock:{video_id} TTL
	TranscodeLockTTL  = 30 * time.Minute // video:transcode:lock:{id} TTL
	PlaybackAuthTTL   = 5 * time.Minute  // playback:{video_id}:{user_id} TTL
	VideoDetailTTL    = 30 * time.Minute // video:detail:{id} cache TTL
	PresignTTL        = 5 * time.Minute  // S3 presigned URL TTL
)

// Transcode qualities produced on CompleteUpload (design §P3).
var TranscodeQualities = []string{"720p", "480p", "360p"}