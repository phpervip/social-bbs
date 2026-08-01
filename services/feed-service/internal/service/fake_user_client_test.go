package service

import (
	"context"

	"social-bbs/feed-service/internal/repository"
)

// fakeUserClient is an in-memory UserClient for service tests: static
// profile/follow maps, no gRPC involved.
type fakeUserClient struct {
	profiles  map[int64]*repository.User
	followers map[int64][]int64
	following map[int64][]int64
}

func (f *fakeUserClient) GetProfile(_ context.Context, id int64) (*repository.User, error) {
	if u, ok := f.profiles[id]; ok {
		return u, nil
	}
	return nil, repository.ErrUserNotFound
}

func (f *fakeUserClient) GetProfiles(_ context.Context, ids []int64) (map[int64]*repository.User, error) {
	out := make(map[int64]*repository.User, len(ids))
	for _, id := range ids {
		if u, ok := f.profiles[id]; ok {
			out[id] = u
		}
	}
	return out, nil
}

func (f *fakeUserClient) GetFollowerIDs(_ context.Context, userID int64) ([]int64, error) {
	return f.followers[userID], nil
}

func (f *fakeUserClient) GetFollowingIDs(_ context.Context, userID int64) ([]int64, error) {
	return f.following[userID], nil
}

func (f *fakeUserClient) Close() error { return nil }
