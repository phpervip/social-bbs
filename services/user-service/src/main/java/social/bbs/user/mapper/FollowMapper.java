package social.bbs.user.mapper;

import com.baomidou.mybatisplus.core.mapper.BaseMapper;
import org.apache.ibatis.annotations.Delete;
import org.apache.ibatis.annotations.Insert;
import org.apache.ibatis.annotations.Mapper;
import org.apache.ibatis.annotations.Param;
import social.bbs.user.entity.FollowEntity;

@Mapper
public interface FollowMapper extends BaseMapper<FollowEntity> {
    @Insert("INSERT IGNORE INTO follows (follower_id, followee_id, created_at) VALUES (#{followerId}, #{followeeId}, #{createdAt})")
    int insertIgnore(FollowEntity entity);

    @Delete("DELETE FROM follows WHERE follower_id = #{followerId} AND followee_id = #{followeeId}")
    int delete(@Param("followerId") Long followerId, @Param("followeeId") Long followeeId);
}
