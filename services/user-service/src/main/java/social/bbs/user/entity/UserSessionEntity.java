package social.bbs.user.entity;

import com.baomidou.mybatisplus.annotation.TableName;
import lombok.Data;

@TableName("user_sessions")
@Data
public class UserSessionEntity {
    private String tokenId;
    private Long userId;
    private Long expiresAt;
    private Integer revoked;
    private Long createdAt;
}
