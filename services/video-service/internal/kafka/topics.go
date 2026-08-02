// Package kafka owns the P3 Kafka contract: topic/group names and the JSON
// event payloads exchanged with the FFmpeg worker.
package kafka

// Topic and consumer group names (dot-separated snake_case).
const (
	TopicTranscodeTask = "video:transcode-task" // produced on CompleteUpload, consumed by the FFmpeg worker
	TopicTranscoded    = "video:transcoded"     // produced when all transcode tasks complete
	GroupTranscode     = "video-transcode"      // FFmpeg worker consumer group
)

// TranscodeTaskEvent is the payload of video:transcode-task.
type TranscodeTaskEvent struct {
	TaskID  uint64 `json:"task_id"`
	VideoID uint64 `json:"video_id"`
	Quality string `json:"quality"`
}

// TranscodedEvent is the payload of video:transcoded.
type TranscodedEvent struct {
	VideoID uint64 `json:"video_id"`
	Status  string `json:"status"` // completed/failed
}