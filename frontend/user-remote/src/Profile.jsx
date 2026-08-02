import { useCallback, useEffect, useRef, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { api, ui } from '@b/shared';
import { getCurrentUserId } from './currentUser.js';
import './styles.css'; // required — component styles must travel with the lazy MF chunk

const PAGE_SIZE = 20;

/* ---------------- UserRow ---------------- */

function UserRow({ user }) {
  return (
    <Link to={`/profile/${user.id}`} className="user-row">
      <ui.Avatar src={user.avatar_url || undefined} name={user.display_name || user.username} size={40} />
      <div className="user-row-main">
        <div className="user-row-name">{user.display_name || user.username}</div>
        <div className="user-row-username">@{user.username}</div>
      </div>
    </Link>
  );
}

/* ---------------- Profile ---------------- */

/**
 * Profile — /profile/:id (no id → current user from the JWT).
 * Header (avatar/display_name/@username/bio + follower/following counts),
 * follow/unfollow toggle for other users (optimistic counts),
 * 粉丝列表 / 关注列表 tabs with cursor pagination (IntersectionObserver).
 */
export default function Profile() {
  const { id } = useParams();
  const navigate = useNavigate();

  const viewerId = getCurrentUserId();
  const profileId = id ? Number(id) : viewerId;
  const isOwn = profileId != null && viewerId != null && profileId === viewerId;

  const [profile, setProfile] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  // Follow state: initialized from the profile's is_following if present
  // (the P2 User message has no such field yet — pragmatic local state).
  const [following, setFollowing] = useState(false);
  const [followBusy, setFollowBusy] = useState(false);

  // 粉丝/关注 tabs
  const [tab, setTab] = useState('followers');
  const [users, setUsers] = useState([]);
  const [nextCursor, setNextCursor] = useState(0);
  const [hasMore, setHasMore] = useState(false);
  const [usersLoading, setUsersLoading] = useState(false);
  const [usersLoadingMore, setUsersLoadingMore] = useState(false);
  const [usersError, setUsersError] = useState(null);

  // Mirror pagination state for the scroll-triggered loader (no stale closures).
  const pagerRef = useRef({ hasMore: false, nextCursor: 0 });
  pagerRef.current = { hasMore, nextCursor };

  const fetchingRef = useRef(false); // concurrent-load guard
  const sentinelRef = useRef(null);

  const load = useCallback(async () => {
    if (profileId == null) return;
    setLoading(true);
    setError(null);
    try {
      const res = await api.getProfile(profileId);
      setProfile(res.user);
      setFollowing(Boolean(res.user.is_following));
    } catch (e) {
      setError(e.message || '加载失败');
    } finally {
      setLoading(false);
    }
  }, [profileId]);

  const listEndpoint = (cursor) =>
    tab === 'followers'
      ? api.getFollowers({ id: profileId, cursor, limit: PAGE_SIZE })
      : api.getFollowing({ id: profileId, cursor, limit: PAGE_SIZE });

  const loadUsers = useCallback(async () => {
    if (profileId == null) return;
    setUsersLoading(true);
    setUsersError(null);
    try {
      const res = await listEndpoint(0);
      setUsers(res.users || []);
      setNextCursor(res.next_cursor || 0);
      setHasMore(Boolean(res.has_more));
    } catch (e) {
      setUsersError(e.message || '加载失败');
    } finally {
      setUsersLoading(false);
    }
  }, [tab, profileId]);

  const loadMoreUsers = useCallback(async () => {
    const { hasMore: more, nextCursor: cursor } = pagerRef.current;
    if (fetchingRef.current || !more || profileId == null) return;
    fetchingRef.current = true;
    setUsersLoadingMore(true);
    try {
      const res = await listEndpoint(cursor);
      setUsers((prev) => [...prev, ...(res.users || [])]);
      setNextCursor(res.next_cursor || cursor);
      setHasMore(Boolean(res.has_more));
    } catch (e) {
      ui.Toast.show({ type: 'error', message: e.message || '加载失败' });
    } finally {
      fetchingRef.current = false;
      setUsersLoadingMore(false);
    }
  }, [tab, profileId]);

  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => {
    loadUsers();
  }, [loadUsers]);

  // Infinite scroll: IntersectionObserver on a sentinel near the list bottom.
  useEffect(() => {
    const el = sentinelRef.current;
    if (!el || !hasMore) return undefined;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0] && entries[0].isIntersecting) loadMoreUsers();
      },
      { rootMargin: '200px' }
    );
    observer.observe(el);
    return () => observer.disconnect();
  }, [hasMore, loadMoreUsers]);

  const toggleFollow = async () => {
    if (followBusy || !profile) return;
    const willFollow = !following;
    const delta = willFollow ? 1 : -1;
    // Optimistic UI: flip state + counts, roll back on failure.
    setFollowing(willFollow);
    setProfile((p) =>
      p ? { ...p, follower_count: Math.max(0, (p.follower_count || 0) + delta) } : p
    );
    setFollowBusy(true);
    try {
      if (willFollow) await api.follow(profile.id);
      else await api.unfollow(profile.id);
      ui.Toast.show({ type: 'success', message: willFollow ? '已关注' : '已取消关注' });
    } catch (e) {
      setFollowing(!willFollow);
      setProfile((p) =>
        p ? { ...p, follower_count: Math.max(0, (p.follower_count || 0) - delta) } : p
      );
      ui.Toast.show({ type: 'error', message: e.message || '操作失败，请重试' });
    } finally {
      setFollowBusy(false);
    }
  };

  if (profileId == null) {
    return (
      <ui.EmptyState
        icon="🔒"
        title="请先登录"
        description="登录后即可查看个人主页。"
        action={<ui.Button variant="primary" onClick={() => navigate('/login')}>去登录</ui.Button>}
      />
    );
  }

  if (loading) {
    return (
      <div>
        <div className="user-profile-head"><ui.Skeleton circle width={72} height={72} /></div>
        <div className="user-profile-head"><ui.Skeleton width="100%" height={64} /></div>
        <div className="user-list">
          <div className="user-row"><ui.Skeleton width="100%" height={40} /></div>
          <div className="user-row"><ui.Skeleton width="100%" height={40} /></div>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <ui.EmptyState
        icon="⚠️"
        title="加载失败"
        description={error}
        action={<ui.Button variant="primary" onClick={load}>重试</ui.Button>}
      />
    );
  }

  return (
    <div>
      <div className="user-profile-head">
        <ui.Avatar src={profile.avatar_url || undefined} name={profile.display_name || profile.username} size={72} />
        <div className="user-profile-meta">
          <h2 className="user-profile-name">{profile.display_name || profile.username}</h2>
          <span className="user-profile-username">@{profile.username}</span>
          {profile.bio && <p className="user-profile-bio">{profile.bio}</p>}
          <div className="user-stats">
            <button
              type="button"
              className={`user-stat${tab === 'followers' ? ' active' : ''}`}
              onClick={() => setTab('followers')}
            >
              <strong>{profile.follower_count || 0}</strong> 粉丝
            </button>
            <button
              type="button"
              className={`user-stat${tab === 'following' ? ' active' : ''}`}
              onClick={() => setTab('following')}
            >
              <strong>{profile.following_count || 0}</strong> 关注
            </button>
          </div>
        </div>
        {!isOwn && (
          <ui.Button
            variant={following ? 'ghost' : 'primary'}
            className="user-profile-follow"
            loading={followBusy}
            onClick={toggleFollow}
          >
            {following ? '已关注' : '关注'}
          </ui.Button>
        )}
      </div>

      <div className="user-tabs" role="tablist">
        <button
          type="button"
          role="tab"
          aria-selected={tab === 'followers'}
          className={`user-tab${tab === 'followers' ? ' active' : ''}`}
          onClick={() => setTab('followers')}
        >
          粉丝
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={tab === 'following'}
          className={`user-tab${tab === 'following' ? ' active' : ''}`}
          onClick={() => setTab('following')}
        >
          关注
        </button>
      </div>

      <div className="user-list">
        {usersLoading ? (
          <div className="user-row"><ui.Skeleton width="100%" height={40} /></div>
        ) : usersError ? (
          <ui.EmptyState
            icon="⚠️"
            title="加载失败"
            description={usersError}
            action={<ui.Button variant="primary" onClick={loadUsers}>重试</ui.Button>}
          />
        ) : users.length === 0 ? (
          <ui.EmptyState
            icon="👥"
            title={tab === 'followers' ? '还没有粉丝' : '还没有关注'}
            description={tab === 'followers' ? '关注列表空空如也，去认识些新朋友吧。' : '去首页看看有什么新鲜事吧。'}
          />
        ) : (
          <>
            {users.map((u) => (
              <UserRow key={u.id} user={u} />
            ))}
            {hasMore && <div ref={sentinelRef} />}
            {usersLoadingMore && <div className="user-loading-more">加载中…</div>}
          </>
        )}
      </div>
    </div>
  );
}
