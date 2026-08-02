package social.bbs.user.service;

import com.baomidou.mybatisplus.core.toolkit.Wrappers;
import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import io.grpc.Status;
import io.grpc.StatusRuntimeException;
import lombok.RequiredArgsConstructor;
import org.springframework.data.redis.core.StringRedisTemplate;
import org.springframework.security.crypto.bcrypt.BCryptPasswordEncoder;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;
import social.bbs.user.entity.UserEntity;
import social.bbs.user.entity.UserSessionEntity;
import social.bbs.user.mapper.FollowMapper;
import social.bbs.user.mapper.UserMapper;
import social.bbs.user.mapper.UserSessionMapper;
import social.bbs.user.util.JwtUtil;
import user.v1.*;

import java.time.Duration;
import java.time.Instant;
import java.util.HashMap;
import java.util.Map;

@Service
@RequiredArgsConstructor
public class AuthService {

    private static final String PROFILE_KEY_PREFIX = "user:profile:";
    private static final Duration PROFILE_TTL = Duration.ofMinutes(10);

    private final UserMapper userMapper;
    private final UserSessionMapper userSessionMapper;
    private final FollowMapper followMapper;
    private final JwtUtil jwtUtil;
    private final UserOutboxService userOutboxService;
    private final StringRedisTemplate redisTemplate;
    private final BCryptPasswordEncoder passwordEncoder = new BCryptPasswordEncoder();
    private final ObjectMapper objectMapper = new ObjectMapper();

    @Transactional
    public AuthResponse register(RegisterRequest req) {
        if (userMapper.selectCount(Wrappers.<UserEntity>lambdaQuery()
                .eq(UserEntity::getUsername, req.getUsername())) > 0) {
            throw new StatusRuntimeException(Status.ALREADY_EXISTS.withDescription("Username already exists"));
        }
        if (userMapper.selectCount(Wrappers.<UserEntity>lambdaQuery()
                .eq(UserEntity::getEmail, req.getEmail())) > 0) {
            throw new StatusRuntimeException(Status.ALREADY_EXISTS.withDescription("Email already exists"));
        }

        long now = Instant.now().toEpochMilli();
        UserEntity entity = new UserEntity();
        entity.setUsername(req.getUsername());
        entity.setEmail(req.getEmail());
        entity.setPasswordHash(passwordEncoder.encode(req.getPassword()));
        entity.setDisplayName(req.getDisplayName());
        entity.setBio("");
        entity.setAvatarUrl("");
        entity.setStatus(1);
        entity.setCreatedAt(now);
        entity.setUpdatedAt(now);
        userMapper.insert(entity);

        userOutboxService.writeRegistered(entity.getId(), entity.getUsername(), entity.getDisplayName());

        User user = toProto(entity);
        cacheProfile(entity.getId(), user);

        String token = jwtUtil.generateToken(entity.getId(), entity.getUsername(), entity.getDisplayName());
        long exp = Instant.now().plusSeconds(86400).toEpochMilli();
        UserSessionEntity session = new UserSessionEntity();
        session.setTokenId(jwtUtil.parseToken(token).get("jti", String.class));
        session.setUserId(entity.getId());
        session.setExpiresAt(exp);
        session.setRevoked(0);
        session.setCreatedAt(Instant.now().toEpochMilli());
        userSessionMapper.insert(session);

        return AuthResponse.newBuilder()
                .setToken(token)
                .setExpiresIn(86400)
                .setUser(user)
                .build();
    }

    @Transactional
    public AuthResponse login(LoginRequest req) {
        UserEntity entity = userMapper.selectOne(Wrappers.<UserEntity>lambdaQuery()
                .eq(UserEntity::getUsername, req.getAccount())
                .or()
                .eq(UserEntity::getEmail, req.getAccount()));
        if (entity == null || !passwordEncoder.matches(req.getPassword(), entity.getPasswordHash())) {
            throw new StatusRuntimeException(Status.UNAUTHENTICATED.withDescription("Invalid credentials"));
        }

        String token = jwtUtil.generateToken(entity.getId(), entity.getUsername(), entity.getDisplayName());
        long exp = Instant.now().plusSeconds(86400).toEpochMilli();
        UserSessionEntity session = new UserSessionEntity();
        session.setTokenId(jwtUtil.parseToken(token).get("jti", String.class));
        session.setUserId(entity.getId());
        session.setExpiresAt(exp);
        session.setRevoked(0);
        session.setCreatedAt(Instant.now().toEpochMilli());
        userSessionMapper.insert(session);

        User user = toProto(entity);
        cacheProfile(entity.getId(), user);
        return AuthResponse.newBuilder()
                .setToken(token)
                .setExpiresIn(86400)
                .setUser(user)
                .build();
    }

    @Transactional
    public void logout(LogoutRequest req) {
        UserSessionEntity session = userSessionMapper.selectById(req.getJti());
        if (session != null) {
            session.setRevoked(1);
            userSessionMapper.updateById(session);
        }
    }

    public User getProfile(GetProfileRequest req) {
        User cached = readProfileCache(req.getUserId());
        if (cached != null) {
            return cached;
        }
        UserEntity entity = userMapper.selectById(req.getUserId());
        if (entity == null) {
            throw new StatusRuntimeException(Status.NOT_FOUND.withDescription("User not found"));
        }
        User user = toProto(entity);
        cacheProfile(req.getUserId(), user);
        return user;
    }

    public User updateProfile(UpdateProfileRequest req) {
        UserEntity entity = userMapper.selectById(req.getUserId());
        if (entity == null) {
            throw new StatusRuntimeException(Status.NOT_FOUND.withDescription("User not found"));
        }
        if (!req.getDisplayName().isEmpty()) {
            entity.setDisplayName(req.getDisplayName());
        }
        if (!req.getBio().isEmpty()) {
            entity.setBio(req.getBio());
        }
        if (!req.getAvatarUrl().isEmpty()) {
            entity.setAvatarUrl(req.getAvatarUrl());
        }
        entity.setUpdatedAt(Instant.now().toEpochMilli());
        userMapper.updateById(entity);
        evictProfileCache(req.getUserId());
        return toProto(entity);
    }

    User toProto(UserEntity entity) {
        long id = entity.getId();
        long followers = followMapper.selectCount(Wrappers.<social.bbs.user.entity.FollowEntity>lambdaQuery()
                .eq(social.bbs.user.entity.FollowEntity::getFolloweeId, id));
        long following = followMapper.selectCount(Wrappers.<social.bbs.user.entity.FollowEntity>lambdaQuery()
                .eq(social.bbs.user.entity.FollowEntity::getFollowerId, id));
        return User.newBuilder()
                .setId(id)
                .setUsername(entity.getUsername())
                .setDisplayName(entity.getDisplayName())
                .setEmail(entity.getEmail())
                .setBio(entity.getBio() == null ? "" : entity.getBio())
                .setAvatarUrl(entity.getAvatarUrl() == null ? "" : entity.getAvatarUrl())
                .setFollowerCount(followers)
                .setFollowingCount(following)
                .setCreatedAt(entity.getCreatedAt())
                .build();
    }

    private void cacheProfile(Long userId, User user) {
        try {
            redisTemplate.opsForValue().set(PROFILE_KEY_PREFIX + userId,
                    objectMapper.writeValueAsString(toCacheMap(user)), PROFILE_TTL);
        } catch (Exception ignored) {
            // Redis unavailable; the database remains the source of truth
        }
    }

    private User readProfileCache(Long userId) {
        try {
            String json = redisTemplate.opsForValue().get(PROFILE_KEY_PREFIX + userId);
            if (json == null) {
                return null;
            }
            Map<String, Object> map = objectMapper.readValue(json, new TypeReference<>() {
            });
            return userFromCacheMap(map);
        } catch (Exception e) {
            return null;
        }
    }

    private void evictProfileCache(Long userId) {
        try {
            redisTemplate.delete(PROFILE_KEY_PREFIX + userId);
        } catch (Exception ignored) {
            // Redis unavailable; the TTL bounds staleness
        }
    }

    private Map<String, Object> toCacheMap(User user) {
        Map<String, Object> map = new HashMap<>();
        map.put("id", user.getId());
        map.put("username", user.getUsername());
        map.put("display_name", user.getDisplayName());
        map.put("email", user.getEmail());
        map.put("bio", user.getBio());
        map.put("avatar_url", user.getAvatarUrl());
        map.put("follower_count", user.getFollowerCount());
        map.put("following_count", user.getFollowingCount());
        map.put("created_at", user.getCreatedAt());
        return map;
    }

    private User userFromCacheMap(Map<String, Object> map) {
        return User.newBuilder()
                .setId(((Number) map.get("id")).longValue())
                .setUsername((String) map.get("username"))
                .setDisplayName((String) map.get("display_name"))
                .setEmail((String) map.get("email"))
                .setBio((String) map.getOrDefault("bio", ""))
                .setAvatarUrl((String) map.getOrDefault("avatar_url", ""))
                .setFollowerCount(((Number) map.getOrDefault("follower_count", 0L)).longValue())
                .setFollowingCount(((Number) map.getOrDefault("following_count", 0L)).longValue())
                .setCreatedAt(((Number) map.getOrDefault("created_at", 0L)).longValue())
                .build();
    }
}
