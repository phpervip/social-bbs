package social.bbs.user.kafka;

import lombok.RequiredArgsConstructor;
import org.springframework.kafka.core.KafkaTemplate;
import org.springframework.stereotype.Component;

import java.nio.charset.StandardCharsets;

@Component
@RequiredArgsConstructor
public class UserEventPublisher {

    private final KafkaTemplate<String, String> kafkaTemplate;

    public void publish(String topic, byte[] payloadBytes) {
        kafkaTemplate.send(topic, new String(payloadBytes, StandardCharsets.UTF_8));
    }
}
