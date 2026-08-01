package social.bbs.user.outbox;

import com.baomidou.mybatisplus.core.toolkit.Wrappers;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.test.mock.mockito.MockBean;
import org.springframework.kafka.core.KafkaTemplate;
import social.bbs.user.UserServiceApplication;
import social.bbs.user.entity.UserOutboxEntity;
import social.bbs.user.mapper.UserOutboxMapper;
import social.bbs.user.service.AuthService;
import user.v1.RegisterRequest;
import user.v1.User;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.anyString;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.verify;

@SpringBootTest(classes = UserServiceApplication.class)
class UserOutboxDispatcherTest {

    @Autowired
    private AuthService authService;

    @Autowired
    private UserOutboxDispatcher dispatcher;

    @Autowired
    private UserOutboxMapper outboxMapper;

    @MockBean
    private KafkaTemplate<String, String> kafkaTemplate;

    @Test
    void pendingOutboxRow_isPublishedToKafka_andMarkedDelivered() {
        User user = authService.register(RegisterRequest.newBuilder()
                .setUsername("dispatch_eve")
                .setEmail("eve@example.com")
                .setPassword("Password123!")
                .setDisplayName("Eve")
                .build()).getUser();

        UserOutboxEntity row = outboxMapper.selectOne(Wrappers.<UserOutboxEntity>lambdaQuery()
                .eq(UserOutboxEntity::getTopic, "user.registered")
                .eq(UserOutboxEntity::getStatus, "pending")
                .orderByDesc(UserOutboxEntity::getId)
                .last("LIMIT 1"));
        assertThat(row).isNotNull();
        assertThat(row.getPayload()).contains("\"user_id\":" + user.getId());

        dispatcher.dispatchPending();

        UserOutboxEntity delivered = outboxMapper.selectById(row.getId());
        assertThat(delivered.getStatus()).isEqualTo("delivered");
        verify(kafkaTemplate).send(eq("user.registered"), anyString());
    }
}
