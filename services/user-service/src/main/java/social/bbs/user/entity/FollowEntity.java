package social.bbs.user.entity;

import com.baomidou.mybatisplus.annotation.TableName;
import lombok.Data;

@TableName("follows")
@Data
public class FollowEntity {
    private Long followerId;
    private Long followeeId;
    private Long createdAt;
}
