package social.bbs.user.mapper;

import com.baomidou.mybatisplus.core.mapper.BaseMapper;
import org.apache.ibatis.annotations.Mapper;
import social.bbs.user.entity.UserEntity;

@Mapper
public interface UserMapper extends BaseMapper<UserEntity> {
}
