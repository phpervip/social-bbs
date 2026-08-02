package service

import (
	"context"
	"encoding/json"

	"social-bbs/video-service/internal/kafka"
	"social-bbs/video-service/internal/repository"
)

// TranscodeService is called by the FFmpeg worker when a transcode task
// completes or fails.
type TranscodeService interface {
	CompleteTranscode(ctx context.Context, videoID uint64, quality string) error
	FailTranscode(ctx context.Context, videoID uint64, quality, errorMsg string) error
}

type transcodeService struct {
	videos    repository.VideoRepo
	transcode repository.TranscodeRepo
	kafka     *kafka.Client
}

// NewTranscodeService wires the transcode service.
func NewTranscodeService(videos repository.VideoRepo, transcode repository.TranscodeRepo, kc *kafka.Client) TranscodeService {
	return &transcodeService{videos: videos, transcode: transcode, kafka: kc}
}

// CompleteTranscode marks the task completed; when every task for the video is
// done it flips the video to completed and emits video:transcoded.
func (s *transcodeService) CompleteTranscode(ctx context.Context, videoID uint64, quality string) error {
	tasks, err := s.transcode.GetByVideoID(ctx, videoID)
	if err != nil {
		return err
	}
	var target *repository.TranscodeTask
	for _, t := range tasks {
		if t.Quality == quality {
			target = t
			break
		}
	}
	if target == nil {
		return repository.ErrTaskNotFound
	}
	if err := s.transcode.UpdateStatus(ctx, target.ID, "completed", ""); err != nil {
		return err
	}

	// Re-read to check whether all tasks are now completed.
	tasks, err = s.transcode.GetByVideoID(ctx, videoID)
	if err != nil {
		return err
	}
	for _, t := range tasks {
		if t.Status != "completed" {
			return nil
		}
	}
	video, err := s.videos.GetByID(ctx, videoID)
	if err != nil {
		return err
	}
	video.Status = "completed"
	if err := s.videos.Update(ctx, video); err != nil {
		return err
	}
	payload, _ := json.Marshal(kafka.TranscodedEvent{VideoID: videoID, Status: "completed"})
	_ = s.kafka.Publish(ctx, kafka.TopicTranscoded, []byte(itoa(videoID)), payload)
	return nil
}

// FailTranscode increments retry_count; if the budget remains it requeues the
// task, otherwise it marks the task (and video) failed.
func (s *transcodeService) FailTranscode(ctx context.Context, videoID uint64, quality, errorMsg string) error {
	tasks, err := s.transcode.GetByVideoID(ctx, videoID)
	if err != nil {
		return err
	}
	var target *repository.TranscodeTask
	for _, t := range tasks {
		if t.Quality == quality {
			target = t
			break
		}
	}
	if target == nil {
		return repository.ErrTaskNotFound
	}

	nextRetry := target.RetryCount + 1
	if nextRetry < target.MaxRetries {
		if err := s.transcode.UpdateStatus(ctx, target.ID, "pending", errorMsg); err != nil {
			return err
		}
		payload, _ := json.Marshal(kafka.TranscodeTaskEvent{TaskID: target.ID, VideoID: videoID, Quality: quality})
		return s.kafka.Publish(ctx, kafka.TopicTranscodeTask, []byte(itoa(target.ID)), payload)
	}

	if err := s.transcode.UpdateStatus(ctx, target.ID, "failed", errorMsg); err != nil {
		return err
	}
	video, err := s.videos.GetByID(ctx, videoID)
	if err != nil {
		return err
	}
	video.Status = "failed"
	if err := s.videos.Update(ctx, video); err != nil {
		return err
	}
	payload, _ := json.Marshal(kafka.TranscodedEvent{VideoID: videoID, Status: "failed"})
	_ = s.kafka.Publish(ctx, kafka.TopicTranscoded, []byte(itoa(videoID)), payload)
	return nil
}

func itoa(v uint64) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}