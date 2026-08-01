package social.bbs.user.service;

import com.baomidou.mybatisplus.core.toolkit.Wrappers;
import io.grpc.Status;
import io.grpc.StatusRuntimeException;
import lombok.RequiredArgsConstructor;
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

import java.time.Instant;

@Service
@RequiredArgsConstructor
public class AuthService {

    private final UserMapper userMapper;
    private final UserSessionMapper userSessionMapper;
    private final FollowMapper followMapper;
    private final JwtUtil jwtUtil;
    private final BCryptPasswordEncoder passwordEncoder = new BCryptPasswordEncoder();

    @Transactional
    public User register(RegisterRequest req) {
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

        return toProto(entity);
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

        return AuthResponse.newBuilder()
                .setToken(token)
                .setExpiresIn(86400)
                .setUser(toProto(entity))
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
        UserEntity entity = userMapper.selectById(req.getUserId());
        if (entity == null) {
            throw new StatusRuntimeException(Status.NOT_FOUND.withDescription("User not found"));
        }
        return toProto(entity);
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
        return toProto(entity);
    }

    private User toProto(UserEntity entity) {
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
}
