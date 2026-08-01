package social.bbs.user.service;

import io.grpc.Status;
import io.grpc.StatusRuntimeException;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.transaction.annotation.Transactional;
import social.bbs.user.UserServiceApplication;
import user.v1.*;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

@SpringBootTest(classes = UserServiceApplication.class)
@Transactional
class FollowServiceTest {

    @Autowired
    private AuthService authService;

    @Autowired
    private FollowService followService;

    @Test
    void follow_duplicate_isIdempotent() {
        User alice = authService.register(RegisterRequest.newBuilder()
                .setUsername("alice").setEmail("alice@example.com")
                .setPassword("Password123!").setDisplayName("Alice").build()).getUser();
        User bob = authService.register(RegisterRequest.newBuilder()
                .setUsername("bob").setEmail("bob@example.com")
                .setPassword("Password123!").setDisplayName("Bob").build()).getUser();

        FollowRequest req = FollowRequest.newBuilder()
                .setFollowerId(alice.getId())
                .setFolloweeId(bob.getId())
                .build();

        followService.follow(req);
        followService.follow(req); // should not throw on duplicate

        User bobProfile = authService.getProfile(GetProfileRequest.newBuilder()
                .setUserId(bob.getId()).build());
        assertThat(bobProfile.getFollowerCount()).isEqualTo(1);

        User aliceProfile = authService.getProfile(GetProfileRequest.newBuilder()
                .setUserId(alice.getId()).build());
        assertThat(aliceProfile.getFollowingCount()).isEqualTo(1);
    }

    @Test
    void unfollow_notFollowing_isNoop() {
        User alice = authService.register(RegisterRequest.newBuilder()
                .setUsername("alice2").setEmail("alice2@example.com")
                .setPassword("Password123!").setDisplayName("Alice2").build()).getUser();
        User bob = authService.register(RegisterRequest.newBuilder()
                .setUsername("bob2").setEmail("bob2@example.com")
                .setPassword("Password123!").setDisplayName("Bob2").build()).getUser();

        UnfollowRequest req = UnfollowRequest.newBuilder()
                .setFollowerId(alice.getId())
                .setFolloweeId(bob.getId())
                .build();

        followService.unfollow(req); // should not throw

        User bobProfile = authService.getProfile(GetProfileRequest.newBuilder()
                .setUserId(bob.getId()).build());
        assertThat(bobProfile.getFollowerCount()).isEqualTo(0);
    }

    @Test
    void follow_self_rejected() {
        User alice = authService.register(RegisterRequest.newBuilder()
                .setUsername("alice3").setEmail("alice3@example.com")
                .setPassword("Password123!").setDisplayName("Alice3").build()).getUser();

        FollowRequest req = FollowRequest.newBuilder()
                .setFollowerId(alice.getId())
                .setFolloweeId(alice.getId())
                .build();

        assertThatThrownBy(() -> followService.follow(req))
                .isInstanceOf(StatusRuntimeException.class)
                .satisfies(ex -> {
                    StatusRuntimeException sre = (StatusRuntimeException) ex;
                    assertThat(sre.getStatus().getCode()).isEqualTo(Status.Code.INVALID_ARGUMENT);
                });
    }
}
