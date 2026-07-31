package repository

import "errors"

// Domain errors returned by repositories/services and mapped to gRPC codes by the handler.
var (
	ErrPostNotFound     = errors.New("feed: post not found")
	ErrUserNotFound     = errors.New("feed: user not found")
	ErrCacheMiss        = errors.New("feed: cache miss")
	ErrInvalidArgument  = errors.New("feed: invalid argument")
	ErrPermissionDenied = errors.New("feed: permission denied")
)
