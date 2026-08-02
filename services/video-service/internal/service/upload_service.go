package service

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"social-bbs/video-service/internal/kafka"
	"social-bbs/video-service/internal/repository"
)

// UploadService handles the multipart upload lifecycle (Init/Chunk/Complete).
type UploadService interface {
	InitUpload(ctx context.Context, userID uint64, filename, contentType string, totalSize int64) (uploadID string, videoID uint64, err error)
	UploadChunk(ctx context.Context, userID uint64, uploadID string, partNumber int32, data []byte) (int32, int64, error)
	CompleteUpload(ctx context.Context, userID uint64, uploadID, title, description, visibility string) (*repository.Video, error)
}

type uploadService struct {
	videos    repository.VideoRepo
	uploads   repository.UploadRepo
	transcode repository.TranscodeRepo
	s3        *repository.S3Client
	cache     repository.Cache
	kafka     *kafka.Client
	bucket    string
}

// NewUploadService wires the upload service.
func NewUploadService(videos repository.VideoRepo, uploads repository.UploadRepo, transcode repository.TranscodeRepo, s3 *repository.S3Client, cache repository.Cache, kc *kafka.Client, bucket string) UploadService {
	return &uploadService{videos: videos, uploads: uploads, transcode: transcode, s3: s3, cache: cache, kafka: kc, bucket: bucket}
}

// InitUpload creates the video + S3 multipart upload + upload session record.
func (s *uploadService) InitUpload(ctx context.Context, userID uint64, filename, contentType string, totalSize int64) (string, uint64, error) {
	if userID <= 0 {
		return "", 0, repository.ErrInvalidArgument
	}
	if contentType == "" {
		contentType = "video/mp4"
	}
	video, err := s.videos.Create(ctx, &repository.Video{UploaderID: userID, Status: "pending"})
	if err != nil {
		return "", 0, err
	}

	// Distributed lock keyed on the freshly-created video id (TTL 1h).
	lockKey := repository.UploadInitLockKey(video.ID)
	ok, err := s.cache.SetNX(ctx, lockKey, "1", repository.UploadInitLockTTL)
	if err != nil {
		_ = s.videos.Delete(ctx, video.ID)
		return "", 0, err
	}
	if !ok {
		_ = s.videos.Delete(ctx, video.ID)
		return "", 0, repository.ErrLockNotAcquired
	}

	rawKey := "raw/" + uuid.NewString() + ".mp4"
	uploadID, err := s.s3.CreateMultipartUpload(ctx, s.bucket, rawKey, contentType)
	if err != nil {
		_ = s.cache.Del(ctx, lockKey)
		_ = s.videos.Delete(ctx, video.ID)
		return "", 0, err
	}

	chunkSize := uint32(repository.DefaultChunkSize)
	totalChunks := uint32(0)
	if totalSize > 0 {
		totalChunks = uint32(math.Ceil(float64(totalSize) / float64(chunkSize)))
	}
	_, err = s.uploads.Create(ctx, &repository.Upload{
		UploadID:    uploadID,
		VideoID:     video.ID,
		Filename:    filename,
		ContentType: contentType,
		TotalSize:   totalSize,
		ChunkSize:   chunkSize,
		TotalChunks: totalChunks,
		Status:      "uploading",
	})
	if err != nil {
		_ = s.cache.Del(ctx, lockKey)
		_ = s.videos.Delete(ctx, video.ID)
		return "", 0, err
	}
	return uploadID, video.ID, nil
}

// UploadChunk uploads one part to S3 and bumps the received-chunk counter.
func (s *uploadService) UploadChunk(ctx context.Context, userID uint64, uploadID string, partNumber int32, data []byte) (int32, int64, error) {
	if partNumber <= 0 || len(data) == 0 {
		return 0, 0, repository.ErrInvalidArgument
	}
	up, err := s.uploads.GetByID(ctx, uploadID)
	if err != nil {
		return 0, 0, err
	}
	if up.VideoID == 0 {
		return 0, 0, repository.ErrUploadNotFound
	}
	video, err := s.videos.GetByID(ctx, up.VideoID)
	if err != nil {
		return 0, 0, err
	}
	if video.UploaderID != userID {
		return 0, 0, repository.ErrPermissionDenied
	}

	etag, err := s.s3.UploadPart(ctx, s.bucket, video.RawKey, uploadID, partNumber, data)
	if err != nil {
		return 0, 0, err
	}
	_ = etag // ETag is retained by S3 for the Complete call; not persisted here.

	up.ReceivedChunks++
	if err := s.uploads.Update(ctx, up); err != nil {
		return 0, 0, err
	}
	return partNumber, int64(len(data)), nil
}

// CompleteUpload validates all chunks, finalizes the S3 multipart upload,
// updates the video metadata, enqueues the 3 transcode tasks and releases the
// init lock.
func (s *uploadService) CompleteUpload(ctx context.Context, userID uint64, uploadID, title, description, visibility string) (*repository.Video, error) {
	title = strings.TrimSpace(title)
	description = strings.TrimSpace(description)
	if utf8.RuneCountInString(title) > repository.MaxTitleRunes || utf8.RuneCountInString(description) > repository.MaxDescRunes {
		return nil, repository.ErrInvalidArgument
	}
	if visibility == "" {
		visibility = "public"
	}
	if !validVisibility(visibility) {
		return nil, repository.ErrInvalidArgument
	}

	up, err := s.uploads.GetByID(ctx, uploadID)
	if err != nil {
		return nil, err
	}
	video, err := s.videos.GetByID(ctx, up.VideoID)
	if err != nil {
		return nil, err
	}
	if video.UploaderID != userID {
		return nil, repository.ErrPermissionDenied
	}
	if up.TotalChunks > 0 && up.ReceivedChunks < up.TotalChunks {
		return nil, repository.ErrUploadIncomplete
	}

	// S3 merges the uploaded parts into the final raw object.
	if err := s.s3.CompleteMultipartUpload(ctx, s.bucket, video.RawKey, uploadID, nil); err != nil {
		return nil, err
	}

	video.Title = title
	video.Description = description
	video.Visibility = visibility
	video.Status = "pending"
	if err := s.videos.Update(ctx, video); err != nil {
		return nil, err
	}

	up.Status = "completed"
	_ = s.uploads.Update(ctx, up)

	// Create the 3 transcode tasks and enqueue one Kafka message per task.
	if err := s.transcode.CreateBatch(ctx, video.ID, repository.TranscodeQualities); err != nil {
		return nil, err
	}
	tasks, err := s.transcode.GetByVideoID(ctx, video.ID)
	if err != nil {
		return nil, err
	}
	for _, t := range tasks {
		payload, _ := json.Marshal(kafka.TranscodeTaskEvent{TaskID: t.ID, VideoID: video.ID, Quality: t.Quality})
		_ = s.kafka.Publish(ctx, kafka.TopicTranscodeTask, []byte(uploadID), payload)
	}

	// Release the init lock.
	_ = s.cache.Del(ctx, repository.UploadInitLockKey(video.ID))
	return video, nil
}

func validVisibility(v string) bool {
	switch v {
	case "public", "followers_only", "private":
		return true
	default:
		return false
	}
}