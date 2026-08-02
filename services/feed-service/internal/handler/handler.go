// Package handler implements the feed.v1.FeedService gRPC server.
// It is intentionally thin: input conversion + gRPC error mapping only.
// Semantic validation lives in the service layer where it is unit-tested.
package handler

import (
	"context"
	"errors"
	"strconv"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"social-bbs/feed-service/internal/repository"
	"social-bbs/feed-service/internal/service"
	feedpb "social-bbs/feed-service/proto/gen"
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

// CreatePost validates and inserts a new post, then enqueues the post.created
// outbox event (fanned out to real followers by the Kafka consumer).
func (h *FeedHandler) CreatePost(ctx context.Context, req *feedpb.CreatePostRequest) (*feedpb.Post, error) {
	uid, err := currentUserID(ctx)
	if err != nil {
		return nil, toGRPCError(err)
	}
	p, err := h.posts.CreatePost(ctx, uid, req.GetContent(), req.GetMediaUrl())
	if err != nil {
		return nil, toGRPCError(err)
	}
	return toProtoPost(p), nil
}

// GetPost returns a single post by id.
func (h *FeedHandler) GetPost(ctx context.Context, req *feedpb.GetPostRequest) (*feedpb.Post, error) {
	p, err := h.posts.GetPost(ctx, req.GetId(), userIDFromContext(ctx))
	if err != nil {
		return nil, toGRPCError(err)
	}
	return toProtoPost(p), nil
}

// GetHomeTimeline returns the viewer's home timeline.
func (h *FeedHandler) GetHomeTimeline(ctx context.Context, req *feedpb.GetHomeTimelineRequest) (*feedpb.TimelineResponse, error) {
	uid, err := currentUserID(ctx)
	if err != nil {
		return nil, toGRPCError(err)
	}
	cursor, limit := pageFrom(req.GetPage())
	posts, nextCursor, hasMore, err := h.timeline.GetHomeTimeline(ctx, uid, cursor, limit)
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &feedpb.TimelineResponse{
		Posts:      toProtoPosts(posts),
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// DeletePost soft-deletes a post owned by the current user.
func (h *FeedHandler) DeletePost(ctx context.Context, req *feedpb.DeletePostRequest) (*feedpb.Empty, error) {
	uid, err := currentUserID(ctx)
	if err != nil {
		return nil, toGRPCError(err)
	}
	if err := h.posts.DeletePost(ctx, req.GetId(), uid); err != nil {
		return nil, toGRPCError(err)
	}
	return &feedpb.Empty{}, nil
}

// LikePost likes a post (idempotent).
func (h *FeedHandler) LikePost(ctx context.Context, req *feedpb.LikeRequest) (*feedpb.Empty, error) {
	uid, err := currentUserID(ctx)
	if err != nil {
		return nil, toGRPCError(err)
	}
	if err := h.interactions.Like(ctx, uid, req.GetPostId()); err != nil {
		return nil, toGRPCError(err)
	}
	return &feedpb.Empty{}, nil
}

// UnlikePost unlikes a post (idempotent).
func (h *FeedHandler) UnlikePost(ctx context.Context, req *feedpb.LikeRequest) (*feedpb.Empty, error) {
	uid, err := currentUserID(ctx)
	if err != nil {
		return nil, toGRPCError(err)
	}
	if err := h.interactions.Unlike(ctx, uid, req.GetPostId()); err != nil {
		return nil, toGRPCError(err)
	}
	return &feedpb.Empty{}, nil
}

// AddComment adds a comment to a post.
func (h *FeedHandler) AddComment(ctx context.Context, req *feedpb.CommentRequest) (*feedpb.Comment, error) {
	uid, err := currentUserID(ctx)
	if err != nil {
		return nil, toGRPCError(err)
	}
	c, err := h.comments.AddComment(ctx, req.GetPostId(), uid, req.GetContent())
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
	posts, nextCursor, hasMore, err := h.search.Search(ctx, req.GetQuery(), userIDFromContext(ctx), cursor, limit)
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &feedpb.SearchResponse{
		Posts:      toProtoPosts(posts),
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// userIDFromContext returns the current user id injected by the Gateway as the
// X-User-ID gRPC metadata header (design R3 — Feed trusts the Gateway's
// identity and never verifies a JWT itself). 0 = header absent/unparsable.
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

// currentUserID is the authoritative variant: RPCs that act as the user
// (create/like/comment/delete/timeline) reject requests without a valid
// X-User-ID instead of silently degrading.
func currentUserID(ctx context.Context) (int64, error) {
	uid := userIDFromContext(ctx)
	if uid <= 0 {
		return 0, repository.ErrInvalidArgument
	}
	return uid, nil
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
		Id:            p.ID,
		UserId:        p.UserID,
		Username:      p.Username,
		DisplayName:   p.DisplayName,
		AvatarUrl:     p.AvatarURL,
		Content:       p.Content,
		MediaUrl:      p.MediaURL,
		LikeCount:     p.LikeCount,
		CommentCount:  p.CommentCount,
		LikedByViewer: p.LikedByViewer,
		CreatedAt:     p.CreatedAtMs(),
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
