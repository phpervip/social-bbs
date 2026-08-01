package social.bbs.user.entity;

import com.baomidou.mybatisplus.annotation.IdType;
import com.baomidou.mybatisplus.annotation.TableId;
import com.baomidou.mybatisplus.annotation.TableName;
import lombok.Data;

@TableName("user_outbox")
@Data
public class UserOutboxEntity {
    @TableId(type = IdType.AUTO)
    private Long id;
    private String topic;
    private String payload;
    private String status;
    private Integer retryCount;
    private Long createdAt;
    private Long updatedAt;
}
