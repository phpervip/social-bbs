package service

import (
	"context"
	"fmt"

	"social-bbs/video-service/internal/repository"
)

// PlaybackService handles the read/query RPCs: video detail, playback URL,
// transcode status, delete and the user's video list.
type PlaybackService interface {
	GetVideo(ctx context.Context, videoID, viewerID uint64) (*repository.Video, error)
	GetPlaybackURL(ctx context.Context, videoID, userID uint64) (playbackURL, thumbnailURL string, err error)
	GetTranscodeStatus(ctx context.Context, videoID uint64) (status string, tasks []*repository.TranscodeTask, err error)
	DeleteVideo(ctx context.Context, videoID, userID uint64) error
	ListUserVideos(ctx context.Context, userID, viewerID uint64, cursor int64, limit int) ([]*repository.Video, int64, bool, error)
}

type playbackService struct {
	videos repository.VideoRepo
	tasks  repository.TranscodeRepo
	s3     *repository.S3Client
	cache  repository.Cache
	users  repository.UserClient
	bucket string
}

// NewPlaybackService wires the playback service.
func NewPlaybackService(videos repository.VideoRepo, tasks repository.TranscodeRepo, s3 *repository.S3Client, cache repository.Cache, users repository.UserClient, bucket string) PlaybackService {
	return &playbackService{videos: videos, tasks: tasks, s3: s3, cache: cache, users: users, bucket: bucket}
}

// GetVideo returns a video after enforcing its visibility for the viewer.
func (s *playbackService) GetVideo(ctx context.Context, videoID, viewerID uint64) (*repository.Video, error) {
	video, err := s.videos.GetByID(ctx, videoID)
	if err != nil {
		return nil, err
	}
	if !s.canView(ctx, video, viewerID) {
		return nil, repository.ErrPermissionDenied
	}
	return video, nil
}

// GetPlaybackURL checks visibility, then returns presigned HLS + thumbnail URLs
// for the transcoded 720p rendition. Playback auth is cached for 5min.
func (s *playbackService) GetPlaybackURL(ctx context.Context, videoID, userID uint64) (string, string, error) {
	video, err := s.videos.GetByID(ctx, videoID)
	if err != nil {
		return "", "", err
	}
	if !s.canView(ctx, video, userID) {
		return "", "", repository.ErrPermissionDenied
	}
	if video.Status != "completed" {
		return "", "", repository.ErrNotTranscoded
	}

	playbackURL, err := s.s3.Presign(ctx, s.bucket, fmt.Sprintf("hls/%d/720p/index.m3u8", videoID), repository.PresignTTL)
	if err != nil {
		return "", "", err
	}
	thumbURL := ""
	if video.ThumbKey != "" {
		thumbURL, err = s.s3.Presign(ctx, s.bucket, video.ThumbKey, repository.PresignTTL)
		if err != nil {
			return "", "", err
		}
	}

	_ = s.cache.Set(ctx, repository.PlaybackAuthKey(videoID, userID), "1", repository.PlaybackAuthTTL)
	return playbackURL, thumbURL, nil
}

// GetTranscodeStatus returns the video status and its per-quality task states.
func (s *playbackService) GetTranscodeStatus(ctx context.Context, videoID uint64) (string, []*repository.TranscodeTask, error) {
	video, err := s.videos.GetByID(ctx, videoID)
	if err != nil {
		return "", nil, err
	}
	tasks, err := s.tasks.GetByVideoID(ctx, videoID)
	if err != nil {
		return "", nil, err
	}
	return video.Status, tasks, nil
}

// DeleteVideo removes a video owned by the caller.
func (s *playbackService) DeleteVideo(ctx context.Context, videoID, userID uint64) error {
	video, err := s.videos.GetByID(ctx, videoID)
	if err != nil {
		return err
	}
	if video.UploaderID != userID {
		return repository.ErrPermissionDenied
	}
	return s.videos.Delete(ctx, videoID)
}

// ListUserVideos returns a user's videos (visibility-filtered for the viewer),
// newest first with created_at cursor pagination.
func (s *playbackService) ListUserVideos(ctx context.Context, userID, viewerID uint64, cursor int64, limit int) ([]*repository.Video, int64, bool, error) {
	videos, err := s.videos.ListByUploaderID(ctx, userID, cursor, limit+1)
	if err != nil {
		return nil, 0, false, err
	}
	hasMore := len(videos) > limit
	if hasMore {
		videos = videos[:limit]
	}
	nextCursor := int64(0)
	if len(videos) > 0 {
		nextCursor = videos[len(videos)-1].CreatedAt.UnixMilli()
	}
	// Filter out videos the viewer cannot see.
	visible := make([]*repository.Video, 0, len(videos))
	for _, v := range videos {
		if s.canView(ctx, v, viewerID) {
			visible = append(visible, v)
		}
	}
	return visible, nextCursor, hasMore, nil
}

// canView enforces the visibility policy for a viewer.
func (s *playbackService) canView(ctx context.Context, video *repository.Video, viewerID uint64) bool {
	if video == nil {
		return false
	}
	switch video.Visibility {
	case "public":
		return true
	case "private":
		return viewerID == video.UploaderID
	case "followers_only":
		if viewerID == video.UploaderID {
			return true
		}
		if viewerID <= 0 {
			return false
		}
		follows, err := s.users.IsFollowing(ctx, viewerID, video.UploaderID)
		return err == nil && follows
	default:
		return false
	}
}