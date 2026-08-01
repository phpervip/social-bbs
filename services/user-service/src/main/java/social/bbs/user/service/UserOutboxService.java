package social.bbs.user.service;

import com.fasterxml.jackson.databind.ObjectMapper;
import lombok.RequiredArgsConstructor;
import lombok.SneakyThrows;
import org.springframework.data.redis.core.StringRedisTemplate;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;
import social.bbs.user.entity.UserOutboxEntity;
import social.bbs.user.mapper.UserOutboxMapper;

import java.time.Instant;
import java.util.HashMap;
import java.util.Map;

@Service
@RequiredArgsConstructor
public class UserOutboxService {

    private final UserOutboxMapper outboxMapper;
    private final ObjectMapper objectMapper = new ObjectMapper();

    @Transactional
    @SneakyThrows
    public void writeFollowChanged(Long followerId, Long followeeId, String action) {
        Map<String, Object> payload = new HashMap<>();
        payload.put("follower_id", followerId);
        payload.put("followee_id", followeeId);
        payload.put("action", action);
        payload.put("timestamp", Instant.now().toEpochMilli());
        write("user.follow-changed", payload);
    }

    @Transactional
    @SneakyThrows
    public void writeRegistered(Long userId, String username, String displayName) {
        Map<String, Object> payload = new HashMap<>();
        payload.put("user_id", userId);
        payload.put("username", username);
        payload.put("display_name", displayName);
        payload.put("timestamp", Instant.now().toEpochMilli());
        write("user.registered", payload);
    }

    private void write(String topic, Object payload) {
        long now = Instant.now().toEpochMilli();
        UserOutboxEntity entity = new UserOutboxEntity();
        entity.setTopic(topic);
        entity.setPayload(toJson(payload));
        entity.setStatus("pending");
        entity.setRetryCount(0);
        entity.setCreatedAt(now);
        entity.setUpdatedAt(now);
        outboxMapper.insert(entity);
    }

    @SneakyThrows
    private String toJson(Object payload) {
        return objectMapper.writeValueAsString(payload);
    }
}
