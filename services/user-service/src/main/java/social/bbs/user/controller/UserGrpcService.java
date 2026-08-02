package social.bbs.user.controller;

import io.grpc.Status;
import io.grpc.StatusRuntimeException;
import io.grpc.stub.StreamObserver;
import lombok.RequiredArgsConstructor;
import net.devh.boot.grpc.server.service.GrpcService;
import social.bbs.user.service.AuthService;
import social.bbs.user.service.FollowService;
import user.v1.AuthResponse;
import user.v1.FollowListResponse;
import user.v1.FollowRequest;
import user.v1.FollowResponse;
import user.v1.GetFollowersRequest;
import user.v1.GetFollowingRequest;
import user.v1.GetProfileRequest;
import user.v1.GetProfileResponse;
import user.v1.LoginRequest;
import user.v1.LogoutRequest;
import user.v1.LogoutResponse;
import user.v1.RegisterRequest;
import user.v1.UnfollowRequest;
import user.v1.UnfollowResponse;
import user.v1.UpdateProfileRequest;
import user.v1.UpdateProfileResponse;
import user.v1.User;
import user.v1.UserServiceGrpc;

import java.util.function.Supplier;

@GrpcService
@RequiredArgsConstructor
public class UserGrpcService extends UserServiceGrpc.UserServiceImplBase {

    private final AuthService authService;
    private final FollowService followService;

    @Override
    public void register(RegisterRequest request, StreamObserver<AuthResponse> responseObserver) {
        unary(() -> authService.register(request), responseObserver);
    }

    @Override
    public void login(LoginRequest request, StreamObserver<AuthResponse> responseObserver) {
        unary(() -> authService.login(request), responseObserver);
    }

    @Override
    public void logout(LogoutRequest request, StreamObserver<LogoutResponse> responseObserver) {
        unary(() -> {
            authService.logout(request);
            return LogoutResponse.getDefaultInstance();
        }, responseObserver);
    }

    @Override
    public void getProfile(GetProfileRequest request, StreamObserver<GetProfileResponse> responseObserver) {
        unary(() -> GetProfileResponse.newBuilder()
                .setUser(authService.getProfile(request))
                .build(), responseObserver);
    }

    @Override
    public void updateProfile(UpdateProfileRequest request, StreamObserver<UpdateProfileResponse> responseObserver) {
        unary(() -> UpdateProfileResponse.newBuilder()
                .setUser(authService.updateProfile(request))
                .build(), responseObserver);
    }

    @Override
    public void follow(FollowRequest request, StreamObserver<FollowResponse> responseObserver) {
        unary(() -> {
            followService.follow(request);
            return FollowResponse.getDefaultInstance();
        }, responseObserver);
    }

    @Override
    public void unfollow(UnfollowRequest request, StreamObserver<UnfollowResponse> responseObserver) {
        unary(() -> {
            followService.unfollow(request);
            return UnfollowResponse.getDefaultInstance();
        }, responseObserver);
    }

    @Override
    public void getFollowers(GetFollowersRequest request, StreamObserver<FollowListResponse> responseObserver) {
        unary(() -> followService.getFollowers(request), responseObserver);
    }

    @Override
    public void getFollowing(GetFollowingRequest request, StreamObserver<FollowListResponse> responseObserver) {
        unary(() -> followService.getFollowing(request), responseObserver);
    }

    private <T> void unary(Supplier<T> action, StreamObserver<T> responseObserver) {
        try {
            responseObserver.onNext(action.get());
            responseObserver.onCompleted();
        } catch (StatusRuntimeException e) {
            responseObserver.onError(e);
        } catch (Exception e) {
            responseObserver.onError(Status.INTERNAL
                    .withDescription("Internal server error")
                    .withCause(e)
                    .asRuntimeException());
        }
    }
}
