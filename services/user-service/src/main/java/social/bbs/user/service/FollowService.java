package social.bbs.user.service;

import com.baomidou.mybatisplus.core.toolkit.Wrappers;
import com.baomidou.mybatisplus.core.toolkit.support.SFunction;
import io.grpc.Status;
import io.grpc.StatusRuntimeException;
import lombok.RequiredArgsConstructor;
import org.springframework.data.redis.core.StringRedisTemplate;
import org.springframework.data.redis.core.ZSetOperations;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;
import social.bbs.user.entity.FollowEntity;
import social.bbs.user.entity.UserEntity;
import social.bbs.user.mapper.FollowMapper;
import social.bbs.user.mapper.UserMapper;
import user.v1.*;

import java.time.Duration;
import java.time.Instant;
import java.util.ArrayList;
import java.util.List;
import java.util.Set;

@Service
@RequiredArgsConstructor
public class FollowService {

    private static final int DEFAULT_LIMIT = 20;
    private static final int MAX_LIMIT = 100;
    private static final Duration CACHE_TTL = Duration.ofMinutes(5);

    private final FollowMapper followMapper;
    private final UserOutboxService userOutboxService;
    private final StringRedisTemplate redisTemplate;
    private final UserMapper userMapper;
    private final AuthService authService;

    @Transactional
    public void follow(FollowRequest req) {
        if (req.getFollowerId() == req.getFolloweeId()) {
            throw new StatusRuntimeException(Status.INVALID_ARGUMENT.withDescription("Cannot follow yourself"));
        }

        FollowEntity entity = new FollowEntity();
        entity.setFollowerId(req.getFollowerId());
        entity.setFolloweeId(req.getFolloweeId());
        entity.setCreatedAt(Instant.now().toEpochMilli());
        followMapper.insertIgnore(entity);

        userOutboxService.writeFollowChanged(req.getFollowerId(), req.getFolloweeId(), "follow");

        evictFollowCache(req.getFollowerId(), req.getFolloweeId());
    }

    @Transactional
    public void unfollow(UnfollowRequest req) {
        followMapper.delete(req.getFollowerId(), req.getFolloweeId());
        userOutboxService.writeFollowChanged(req.getFollowerId(), req.getFolloweeId(), "unfollow");
        evictFollowCache(req.getFollowerId(), req.getFolloweeId());
    }

    public FollowListResponse getFollowers(GetFollowersRequest req) {
        return listFollows("user:followers:" + req.getUserId(),
                req.getCursor(), clampLimit(req.getLimit()),
                FollowEntity::getFolloweeId, req.getUserId(), FollowEntity::getFollowerId);
    }

    public FollowListResponse getFollowing(GetFollowingRequest req) {
        return listFollows("user:following:" + req.getUserId(),
                req.getCursor(), clampLimit(req.getLimit()),
                FollowEntity::getFollowerId, req.getUserId(), FollowEntity::getFolloweeId);
    }

    private FollowListResponse listFollows(String cacheKey, long cursor, int limit,
                                           SFunction<FollowEntity, Long> matchColumn, Long matchValue,
                                           SFunction<FollowEntity, Long> memberColumn) {
        int pageSize = limit + 1;
        List<Long> ids = readCache(cacheKey, cursor, pageSize);
        boolean hasMore;
        if (ids == null || ids.isEmpty()) {
            ids = loadFromDb(matchColumn, matchValue, memberColumn, cursor, pageSize);
            hasMore = ids.size() > limit;
            if (hasMore) {
                ids = ids.subList(0, limit);
            }
            backfillCache(cacheKey, ids);
        } else {
            hasMore = ids.size() > limit;
            if (hasMore) {
                ids = ids.subList(0, limit);
            }
        }

        List<User> users = new ArrayList<>(ids.size());
        for (Long id : ids) {
            UserEntity entity = userMapper.selectById(id);
            if (entity != null) {
                users.add(authService.toProto(entity));
            }
        }
        long nextCursor = ids.isEmpty() ? 0 : ids.get(ids.size() - 1);
        return FollowListResponse.newBuilder()
                .addAllUsers(users)
                .setNextCursor(nextCursor)
                .setHasMore(hasMore)
                .build();
    }

    private List<Long> readCache(String key, long cursor, int count) {
        try {
            Set<String> members;
            if (cursor <= 0) {
                members = redisTemplate.opsForZSet().reverseRange(key, 0, count - 1L);
            } else {
                members = redisTemplate.opsForZSet().reverseRangeByScore(key, 0, cursor - 1, 0, count);
            }
            if (members == null || members.isEmpty()) {
                return new ArrayList<>();
            }
            List<Long> ids = new ArrayList<>(members.size());
            for (String member : members) {
                ids.add(Long.parseLong(member));
            }
            return ids;
        } catch (Exception e) {
            return null;
        }
    }

    private List<Long> loadFromDb(SFunction<FollowEntity, Long> matchColumn, Long matchValue,
                                  SFunction<FollowEntity, Long> memberColumn, long cursor, int count) {
        List<FollowEntity> rows = followMapper.selectList(Wrappers.<FollowEntity>lambdaQuery()
                .eq(matchColumn, matchValue)
                .lt(cursor > 0, memberColumn, cursor)
                .orderByDesc(memberColumn)
                .last("LIMIT " + count));
        List<Long> ids = new ArrayList<>(rows.size());
        for (FollowEntity row : rows) {
            ids.add(memberColumn.apply(row));
        }
        return ids;
    }

    private void backfillCache(String key, List<Long> ids) {
        try {
            ZSetOperations<String, String> zset = redisTemplate.opsForZSet();
            for (Long id : ids) {
                zset.add(key, String.valueOf(id), id);
            }
            redisTemplate.expire(key, CACHE_TTL);
        } catch (Exception ignored) {
            // Redis unavailable in tests is acceptable; the request was already served from the DB
        }
    }

    private void evictFollowCache(Long followerId, Long followeeId) {
        try {
            redisTemplate.delete("user:followers:" + followeeId);
            redisTemplate.delete("user:following:" + followerId);
            // follower counts changed for both parties; drop their cached profiles too
            redisTemplate.delete("user:profile:" + followerId);
            redisTemplate.delete("user:profile:" + followeeId);
        } catch (Exception ignored) {
            // Redis unavailable in tests is acceptable; cache TTL handles stale data
        }
    }

    private int clampLimit(int limit) {
        if (limit <= 0) {
            return DEFAULT_LIMIT;
        }
        return Math.min(limit, MAX_LIMIT);
    }
}
