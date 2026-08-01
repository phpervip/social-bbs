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
class AuthServiceTest {

    @Autowired
    private AuthService authService;

    @Test
    void register_duplicateUsername_throwsAlreadyExists() {
        RegisterRequest req = RegisterRequest.newBuilder()
                .setUsername("alice")
                .setEmail("alice@example.com")
                .setPassword("Password123!")
                .setDisplayName("Alice")
                .build();

        authService.register(req);

        assertThatThrownBy(() -> authService.register(req))
                .isInstanceOf(StatusRuntimeException.class)
                .satisfies(ex -> {
                    StatusRuntimeException sre = (StatusRuntimeException) ex;
                    assertThat(sre.getStatus().getCode()).isEqualTo(Status.Code.ALREADY_EXISTS);
                });
    }

    @Test
    void login_wrongPassword_throwsUnauthenticated() {
        authService.register(RegisterRequest.newBuilder()
                .setUsername("bob")
                .setEmail("bob@example.com")
                .setPassword("Password123!")
                .setDisplayName("Bob")
                .build());

        LoginRequest login = LoginRequest.newBuilder()
                .setAccount("bob")
                .setPassword("WrongPassword!")
                .build();

        assertThatThrownBy(() -> authService.login(login))
                .isInstanceOf(StatusRuntimeException.class)
                .satisfies(ex -> {
                    StatusRuntimeException sre = (StatusRuntimeException) ex;
                    assertThat(sre.getStatus().getCode()).isEqualTo(Status.Code.UNAUTHENTICATED);
                });
    }

    @Test
    void login_byEmail_works() {
        authService.register(RegisterRequest.newBuilder()
                .setUsername("carol")
                .setEmail("carol@example.com")
                .setPassword("Password123!")
                .setDisplayName("Carol")
                .build());

        LoginRequest login = LoginRequest.newBuilder()
                .setAccount("carol@example.com")
                .setPassword("Password123!")
                .build();

        AuthResponse resp = authService.login(login);

        assertThat(resp.getToken()).isNotBlank();
        assertThat(resp.getUser().getUsername()).isEqualTo("carol");
    }

    @Test
    void jwt_roundtrip_preservesClaims() {
        User user = authService.register(RegisterRequest.newBuilder()
                .setUsername("dave")
                .setEmail("dave@example.com")
                .setPassword("Password123!")
                .setDisplayName("Dave")
                .build());

        AuthResponse resp = authService.login(LoginRequest.newBuilder()
                .setAccount("dave")
                .setPassword("Password123!")
                .build());

        assertThat(resp.getToken()).isNotBlank();
        assertThat(resp.getUser().getId()).isEqualTo(user.getId());
        assertThat(resp.getUser().getUsername()).isEqualTo("dave");
        assertThat(resp.getUser().getDisplayName()).isEqualTo("Dave");
    }
}
