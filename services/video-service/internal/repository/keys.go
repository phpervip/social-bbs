package repository

import "fmt"

// Redis key builders — keys must match the P3 contract exactly.
func UploadInitLockKey(videoID uint64) string { return fmt.Sprintf("upload:init:lock:%d", videoID) }
func TranscodeLockKey(id uint64) string       { return fmt.Sprintf("video:transcode:lock:%d", id) }
func PlaybackAuthKey(videoID, userID uint64) string {
	return fmt.Sprintf("playback:%d:%d", videoID, userID)
}
func VideoDetailKey(videoID uint64) string { return fmt.Sprintf("video:detail:%d", videoID) }