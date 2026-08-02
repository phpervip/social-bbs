package repository

import "errors"

// Domain errors returned by repositories/services and mapped to gRPC codes by the handler.
var (
	ErrVideoNotFound     = errors.New("video: video not found")
	ErrUploadNotFound    = errors.New("video: upload not found")
	ErrTaskNotFound      = errors.New("video: transcode task not found")
	ErrCacheMiss         = errors.New("video: cache miss")
	ErrInvalidArgument   = errors.New("video: invalid argument")
	ErrPermissionDenied  = errors.New("video: permission denied")
	ErrUploadIncomplete  = errors.New("video: upload incomplete")
	ErrLockNotAcquired   = errors.New("video: lock not acquired")
	ErrNotTranscoded     = errors.New("video: not transcoded")
)