// Package handler implements the feed.v1.FeedService gRPC server.
// It is intentionally thin: input conversion + gRPC error mapping only.
// Semantic validation lives in the service layer where it is unit-tested.
package handler

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	feedpb "social-bbs/feed-service/proto/gen"
	"social-bbs/feed-service/internal/repository"
	"social-bbs/feed-service/internal/service"
)

// FeedHandler implements feedpb.FeedServiceServer.
type FeedHandler struct {
	feedpb.UnimplementedFeedServiceServer
	posts        service.PostService
	timeline     service.TimelineService
	interactions service.InteractionService
	comments     service.CommentService
	search       service.SearchService
}

// NewFeedHandler wires the gRPC handler to its services.
func NewFeedHandler(
	posts service.PostService,
	timeline service.TimelineService,
	interactions service.InteractionService,
	comments service.CommentService,
	search service.SearchService,
) *FeedHandler {
	return &FeedHandler{
		posts:        posts,
		timeline:     timeline,
		interactions: interactions,
		comments:     comments,
		search:       search,
	}
}

// CreatePost validates and inserts a new post, then fans out asynchronously.
func (h *FeedHandler) CreatePost(ctx context.Context, req *feedpb.CreatePostRequest) (*feedpb.Post, error) {
	p, err := h.posts.CreatePost(ctx, req.GetUserId(), req.GetContent(), req.GetMediaUrl())
	if err != nil {
		return nil, toGRPCError(err)
	}
	return toProtoPost(p), nil
}

// GetPost returns a single post by id.
func (h *FeedHandler) GetPost(ctx context.Context, req *feedpb.GetPostRequest) (*feedpb.Post, error) {
	p, err := h.posts.GetPost(ctx, req.GetId(), req.GetViewerId())
	if err != nil {
		return nil, toGRPCError(err)
	}
	return toProtoPost(p), nil
}

// GetHomeTimeline returns the viewer's home timeline.
func (h *FeedHandler) GetHomeTimeline(ctx context.Context, req *feedpb.GetHomeTimelineRequest) (*feedpb.TimelineResponse, error) {
	cursor, limit := pageFrom(req.GetPage())
	posts, nextCursor, hasMore, err := h.timeline.GetHomeTimeline(ctx, req.GetUserId(), cursor, limit)
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &feedpb.TimelineResponse{
		Posts:      toProtoPosts(posts),
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// DeletePost soft-deletes a post owned by user_id.
func (h *FeedHandler) DeletePost(ctx context.Context, req *feedpb.DeletePostRequest) (*feedpb.Empty, error) {
	if err := h.posts.DeletePost(ctx, req.GetId(), req.GetUserId()); err != nil {
		return nil, toGRPCError(err)
	}
	return &feedpb.Empty{}, nil
}

// LikePost likes a post (idempotent).
func (h *FeedHandler) LikePost(ctx context.Context, req *feedpb.LikeRequest) (*feedpb.Empty, error) {
	if err := h.interactions.Like(ctx, req.GetUserId(), req.GetPostId()); err != nil {
		return nil, toGRPCError(err)
	}
	return &feedpb.Empty{}, nil
}

// UnlikePost unlikes a post (idempotent).
func (h *FeedHandler) UnlikePost(ctx context.Context, req *feedpb.LikeRequest) (*feedpb.Empty, error) {
	if err := h.interactions.Unlike(ctx, req.GetUserId(), req.GetPostId()); err != nil {
		return nil, toGRPCError(err)
	}
	return &feedpb.Empty{}, nil
}

// AddComment adds a comment to a post.
func (h *FeedHandler) AddComment(ctx context.Context, req *feedpb.CommentRequest) (*feedpb.Comment, error) {
	c, err := h.comments.AddComment(ctx, req.GetPostId(), req.GetUserId(), req.GetContent())
	if err != nil {
		return nil, toGRPCError(err)
	}
	return toProtoComment(c), nil
}

// GetComments paginates a post's comments, newest first.
func (h *FeedHandler) GetComments(ctx context.Context, req *feedpb.GetCommentsRequest) (*feedpb.CommentsResponse, error) {
	cursor, limit := pageFrom(req.GetPage())
	comments, nextCursor, hasMore, err := h.comments.GetComments(ctx, req.GetPostId(), cursor, limit)
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &feedpb.CommentsResponse{
		Comments:   toProtoComments(comments),
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// Search runs the P1 MySQL LIKE search.
func (h *FeedHandler) Search(ctx context.Context, req *feedpb.SearchRequest) (*feedpb.SearchResponse, error) {
	cursor, limit := pageFrom(req.GetPage())
	posts, nextCursor, hasMore, err := h.search.Search(ctx, req.GetQuery(), req.GetUserId(), cursor, limit)
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &feedpb.SearchResponse{
		Posts:      toProtoPosts(posts),
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// pageFrom extracts (cursor, limit) from a possibly-nil CursorPage.
func pageFrom(p *feedpb.CursorPage) (int64, int) {
	if p == nil {
		return 0, 0
	}
	return p.GetCursor(), int(p.GetLimit())
}

// toGRPCError maps repository/service errors to the P1 gRPC codes (contract §2).
func toGRPCError(err error) error {
	switch {
	case errors.Is(err, repository.ErrInvalidArgument), errors.Is(err, repository.ErrUserNotFound):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, repository.ErrPostNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, repository.ErrPermissionDenied):
		return status.Error(codes.PermissionDenied, err.Error())
	default:
		return status.Error(codes.Internal, "internal error")
	}
}

func toProtoPost(p *repository.Post) *feedpb.Post {
	if p == nil {
		return nil
	}
	return &feedpb.Post{
		Id:             p.ID,
		UserId:         p.UserID,
		Username:       p.Username,
		DisplayName:    p.DisplayName,
		AvatarUrl:      p.AvatarURL,
		Content:        p.Content,
		MediaUrl:       p.MediaURL,
		LikeCount:      p.LikeCount,
		CommentCount:   p.CommentCount,
		LikedByViewer:  p.LikedByViewer,
		CreatedAt:      p.CreatedAtMs(),
	}
}

func toProtoPosts(posts []*repository.Post) []*feedpb.Post {
	out := make([]*feedpb.Post, 0, len(posts))
	for _, p := range posts {
		out = append(out, toProtoPost(p))
	}
	return out
}

func toProtoComment(c *repository.Comment) *feedpb.Comment {
	if c == nil {
		return nil
	}
	return &feedpb.Comment{
		Id:          c.ID,
		PostId:      c.PostID,
		UserId:      c.UserID,
		Username:    c.Username,
		DisplayName: c.DisplayName,
		AvatarUrl:   c.AvatarURL,
		Content:     c.Content,
		CreatedAt:   c.CreatedAtMs(),
	}
}

func toProtoComments(comments []*repository.Comment) []*feedpb.Comment {
	out := make([]*feedpb.Comment, 0, len(comments))
	for _, c := range comments {
		out = append(out, toProtoComment(c))
	}
	return out
}
