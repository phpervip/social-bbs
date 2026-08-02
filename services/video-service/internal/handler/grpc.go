// Package handler implements the video.v1.VideoService gRPC server.
// It is intentionally thin: input conversion + gRPC error mapping only.
// Semantic validation lives in the service layer.
package handler

import (
	"context"
	"errors"
	"strconv"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"social-bbs/video-service/internal/repository"
	"social-bbs/video-service/internal/service"
	videopb "social-bbs/video-service/proto/gen"
)

// VideoHandler implements videopb.VideoServiceServer.
type VideoHandler struct {
	videopb.UnimplementedVideoServiceServer
	uploads   service.UploadService
	playback  service.PlaybackService
}

// NewVideoHandler wires the gRPC handler to its services.
func NewVideoHandler(uploads service.UploadService, playback service.PlaybackService) *VideoHandler {
	return &VideoHandler{uploads: uploads, playback: playback}
}

// InitUpload starts a multipart upload session.
func (h *VideoHandler) InitUpload(ctx context.Context, req *videopb.InitUploadRequest) (*videopb.InitUploadResponse, error) {
	uid, err := currentUserID(ctx)
	if err != nil {
		return nil, toGRPCError(err)
	}
	uploadID, videoID, err := h.uploads.InitUpload(ctx, uint64(uid), req.GetFilename(), req.GetContentType(), req.GetTotalSize())
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &videopb.InitUploadResponse{UploadId: uploadID, VideoId: int64(videoID)}, nil
}

// UploadChunk uploads one part of the multipart upload.
func (h *VideoHandler) UploadChunk(ctx context.Context, req *videopb.UploadChunkRequest) (*videopb.UploadChunkResponse, error) {
	uid, err := currentUserID(ctx)
	if err != nil {
		return nil, toGRPCError(err)
	}
	part, size, err := h.uploads.UploadChunk(ctx, uint64(uid), req.GetUploadId(), req.GetPartNumber(), req.GetData())
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &videopb.UploadChunkResponse{PartNumber: part, EtagBytes: size}, nil
}

// CompleteUpload finalizes the upload and enqueues transcoding.
func (h *VideoHandler) CompleteUpload(ctx context.Context, req *videopb.CompleteUploadRequest) (*videopb.CompleteUploadResponse, error) {
	uid, err := currentUserID(ctx)
	if err != nil {
		return nil, toGRPCError(err)
	}
	video, err := h.uploads.CompleteUpload(ctx, uint64(uid), req.GetUploadId(), req.GetTitle(), req.GetDescription(), visibilityString(req.GetVisibility()))
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &videopb.CompleteUploadResponse{Video: toProtoVideo(video)}, nil
}

// GetVideo returns a single video (visibility-checked).
func (h *VideoHandler) GetVideo(ctx context.Context, req *videopb.GetVideoRequest) (*videopb.Video, error) {
	viewer := viewerID(ctx, req.GetViewerId())
	video, err := h.playback.GetVideo(ctx, uint64(req.GetVideoId()), uint64(viewer))
	if err != nil {
		return nil, toGRPCError(err)
	}
	return toProtoVideo(video), nil
}

// GetPlaybackURL returns presigned HLS + thumbnail URLs.
func (h *VideoHandler) GetPlaybackURL(ctx context.Context, req *videopb.GetPlaybackURLRequest) (*videopb.GetPlaybackURLResponse, error) {
	viewer := viewerID(ctx, req.GetUserId())
	playbackURL, thumbURL, err := h.playback.GetPlaybackURL(ctx, uint64(req.GetVideoId()), uint64(viewer))
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &videopb.GetPlaybackURLResponse{PlaybackUrl: playbackURL, ThumbnailUrl: thumbURL}, nil
}

// GetTranscodeStatus returns the video + per-quality task statuses.
func (h *VideoHandler) GetTranscodeStatus(ctx context.Context, req *videopb.GetTranscodeStatusRequest) (*videopb.GetTranscodeStatusResponse, error) {
	statusStr, tasks, err := h.playback.GetTranscodeStatus(ctx, uint64(req.GetVideoId()))
	if err != nil {
		return nil, toGRPCError(err)
	}
	resp := &videopb.GetTranscodeStatusResponse{Status: videoStatus(statusStr)}
	for _, t := range tasks {
		resp.Tasks = append(resp.Tasks, &videopb.TranscodeTaskInfo{
			Quality:    t.Quality,
			Status:     videoStatus(t.Status),
			RetryCount: int32(t.RetryCount),
		})
	}
	return resp, nil
}

// DeleteVideo removes a video owned by the caller.
func (h *VideoHandler) DeleteVideo(ctx context.Context, req *videopb.DeleteVideoRequest) (*videopb.Empty, error) {
	uid, err := currentUserID(ctx)
	if err != nil {
		return nil, toGRPCError(err)
	}
	if err := h.playback.DeleteVideo(ctx, uint64(req.GetVideoId()), uint64(uid)); err != nil {
		return nil, toGRPCError(err)
	}
	return &videopb.Empty{}, nil
}

// ListUserVideos paginates a user's videos (visibility-filtered).
func (h *VideoHandler) ListUserVideos(ctx context.Context, req *videopb.ListUserVideosRequest) (*videopb.ListUserVideosResponse, error) {
	viewer := viewerID(ctx, req.GetViewerId())
	cursor, limit := pageFrom(req.GetPage())
	videos, nextCursor, hasMore, err := h.playback.ListUserVideos(ctx, uint64(req.GetUserId()), uint64(viewer), cursor, limit)
	if err != nil {
		return nil, toGRPCError(err)
	}
	resp := &videopb.ListUserVideosResponse{NextCursor: nextCursor, HasMore: hasMore}
	for _, v := range videos {
		resp.Videos = append(resp.Videos, toProtoVideo(v))
	}
	return resp, nil
}

// userIDFromContext returns the current user id injected by the Gateway as the
// X-User-ID gRPC metadata header. 0 = header absent/unparsable.
func userIDFromContext(ctx context.Context) int64 {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return 0
	}
	vals := md.Get("x-user-id")
	if len(vals) == 0 {
		return 0
	}
	id, err := strconv.ParseInt(vals[0], 10, 64)
	if err != nil {
		return 0
	}
	return id
}

// currentUserID is the authoritative variant: RPCs that act as the user reject
// requests without a valid X-User-ID.
func currentUserID(ctx context.Context) (int64, error) {
	uid := userIDFromContext(ctx)
	if uid <= 0 {
		return 0, repository.ErrInvalidArgument
	}
	return uid, nil
}

// viewerID prefers the request's explicit viewer id, falling back to the
// authenticated X-User-ID header.
func viewerID(ctx context.Context, reqViewer int64) int64 {
	if reqViewer > 0 {
		return reqViewer
	}
	return userIDFromContext(ctx)
}

// pageFrom extracts (cursor, limit) from a possibly-nil CursorPage.
func pageFrom(p *videopb.CursorPage) (int64, int) {
	if p == nil {
		return 0, repository.DefaultPageLimit
	}
	limit := int(p.GetLimit())
	if limit <= 0 {
		limit = repository.DefaultPageLimit
	}
	if limit > repository.MaxPageLimit {
		limit = repository.MaxPageLimit
	}
	return p.GetCursor(), limit
}

// toGRPCError maps repository/service errors to gRPC codes.
func toGRPCError(err error) error {
	switch {
	case errors.Is(err, repository.ErrInvalidArgument), errors.Is(err, repository.ErrUploadIncomplete), errors.Is(err, repository.ErrLockNotAcquired):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, repository.ErrVideoNotFound), errors.Is(err, repository.ErrUploadNotFound), errors.Is(err, repository.ErrTaskNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, repository.ErrPermissionDenied):
		return status.Error(codes.PermissionDenied, err.Error())
	case errors.Is(err, repository.ErrNotTranscoded):
		return status.Error(codes.FailedPrecondition, err.Error())
	default:
		return status.Error(codes.Internal, "internal error")
	}
}

func toProtoVideo(v *repository.Video) *videopb.Video {
	if v == nil {
		return nil
	}
	return &videopb.Video{
		Id:          int64(v.ID),
		UploaderId:  int64(v.UploaderID),
		Title:       v.Title,
		Description: v.Description,
		Visibility:  videoVisibility(v.Visibility),
		Status:      videoStatus(v.Status),
		RawKey:      v.RawKey,
		HlsKey:      v.HLSKey,
		ThumbKey:    v.ThumbKey,
		Duration:    int64(v.Duration),
		CreatedAt:   v.CreatedAt.UnixMilli(),
	}
}

func videoStatus(s string) videopb.VideoStatus {
	switch s {
	case "pending":
		return videopb.VideoStatus_VIDEO_STATUS_PENDING
	case "processing":
		return videopb.VideoStatus_VIDEO_STATUS_PROCESSING
	case "completed":
		return videopb.VideoStatus_VIDEO_STATUS_COMPLETED
	case "failed":
		return videopb.VideoStatus_VIDEO_STATUS_FAILED
	default:
		return videopb.VideoStatus_VIDEO_STATUS_UNSPECIFIED
	}
}

func videoVisibility(v string) videopb.VideoVisibility {
	switch v {
	case "public":
		return videopb.VideoVisibility_VIDEO_VISIBILITY_PUBLIC
	case "followers_only":
		return videopb.VideoVisibility_VIDEO_VISIBILITY_FOLLOWERS_ONLY
	case "private":
		return videopb.VideoVisibility_VIDEO_VISIBILITY_PRIVATE
	default:
		return videopb.VideoVisibility_VIDEO_VISIBILITY_UNSPECIFIED
	}
}

func visibilityString(v videopb.VideoVisibility) string {
	switch v {
	case videopb.VideoVisibility_VIDEO_VISIBILITY_PUBLIC:
		return "public"
	case videopb.VideoVisibility_VIDEO_VISIBILITY_FOLLOWERS_ONLY:
		return "followers_only"
	case videopb.VideoVisibility_VIDEO_VISIBILITY_PRIVATE:
		return "private"
	default:
		return "public"
	}
}