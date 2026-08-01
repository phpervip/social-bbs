package social.bbs.user.service;

import io.grpc.Status;
import io.grpc.StatusRuntimeException;
import lombok.RequiredArgsConstructor;
import org.springframework.data.redis.core.StringRedisTemplate;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;
import social.bbs.user.entity.FollowEntity;
import social.bbs.user.mapper.FollowMapper;
import user.v1.FollowRequest;
import user.v1.UnfollowRequest;

import java.time.Instant;

@Service
@RequiredArgsConstructor
public class FollowService {

    private final FollowMapper followMapper;
    private final UserOutboxService userOutboxService;
    private final StringRedisTemplate redisTemplate;

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

    private void evictFollowCache(Long followerId, Long followeeId) {
        try {
            redisTemplate.delete("user:followers:" + followeeId);
            redisTemplate.delete("user:following:" + followerId);
        } catch (Exception ignored) {
            // Redis unavailable in tests is acceptable; cache TTL handles stale data
        }
    }
}
