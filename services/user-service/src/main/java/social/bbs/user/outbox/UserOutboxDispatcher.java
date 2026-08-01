package social.bbs.user.outbox;

import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Component;
import social.bbs.user.entity.UserOutboxEntity;
import social.bbs.user.kafka.UserEventPublisher;
import social.bbs.user.service.UserOutboxService;

import java.nio.charset.StandardCharsets;
import java.util.List;

@Slf4j
@Component
@RequiredArgsConstructor
public class UserOutboxDispatcher {

    private static final int BATCH_SIZE = 100;
    private static final int MAX_RETRY_COUNT = 3;

    private final UserOutboxService outboxService;
    private final UserEventPublisher eventPublisher;

    @Scheduled(fixedDelay = 2000)
    public void dispatchPending() {
        List<UserOutboxEntity> pending = outboxService.claimPending(BATCH_SIZE);
        for (UserOutboxEntity row : pending) {
            try {
                eventPublisher.publish(row.getTopic(), row.getPayload().getBytes(StandardCharsets.UTF_8));
                outboxService.markDelivered(row.getId());
            } catch (Exception e) {
                log.warn("Failed to dispatch outbox row id={} topic={}", row.getId(), row.getTopic(), e);
                int retryCount = outboxService.incrementRetry(row.getId());
                if (retryCount >= MAX_RETRY_COUNT) {
                    outboxService.markFailed(row.getId());
                }
            }
        }
    }
}
