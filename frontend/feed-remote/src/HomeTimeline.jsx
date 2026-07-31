import { useCallback, useEffect, useRef, useState } from 'react';
import { api, bus, ui } from '@b/shared';
import PostCard from './PostCard.jsx';
import './styles.css'; // required — component styles must travel with the lazy MF chunk

const PAGE_SIZE = 20;
const MAX_CHARS = 280;

/* ---------------- PostComposer ---------------- */

function PostComposer({ onPublished }) {
  const [content, setContent] = useState('');
  const [mediaUrl, setMediaUrl] = useState('');
  const [publishing, setPublishing] = useState(false);

  const count = [...content].length;
  const overLimit = count > MAX_CHARS;
  const canPublish = count > 0 && !overLimit && !publishing;

  const submit = async () => {
    if (!canPublish) return;
    setPublishing(true);
    try {
      await api.createPost({ content: content.trim(), media_url: mediaUrl.trim() });
      setContent('');
      setMediaUrl('');
      bus.emit(bus.events.POST_CREATED);
      ui.Toast.show({ type: 'success', message: '发布成功' });
      if (onPublished) onPublished();
    } catch (e) {
      ui.Toast.show({ type: 'error', message: e.message || '发布失败，请重试' });
    } finally {
      setPublishing(false);
    }
  };

  return (
    <div className="feed-composer">
      <ui.Avatar name="我" size={40} />
      <div className="feed-composer-body">
        <textarea
          className="feed-composer-input"
          placeholder="有什么新鲜事？"
          value={content}
          maxLength={MAX_CHARS + 100}
          onChange={(e) => setContent(e.target.value)}
        />
        <input
          className="feed-media-input"
          placeholder="媒体 URL（可选）"
          value={mediaUrl}
          onChange={(e) => setMediaUrl(e.target.value)}
        />
        <div className="feed-composer-actions">
          <span className={`feed-counter${overLimit ? ' over' : ''}`}>{count}/{MAX_CHARS}</span>
          <ui.Button variant="primary" disabled={!canPublish} loading={publishing} onClick={submit}>
            发布
          </ui.Button>
        </div>
      </div>
    </div>
  );
}

/* ---------------- HomeTimeline ---------------- */

export default function HomeTimeline() {
  const [posts, setPosts] = useState([]);
  const [nextCursor, setNextCursor] = useState(0);
  const [hasMore, setHasMore] = useState(false);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState(null);

  // Mirror pagination state for the scroll-triggered loader (no stale closures).
  const pagerRef = useRef({ hasMore: false, nextCursor: 0 });
  pagerRef.current = { hasMore, nextCursor };

  const fetchingRef = useRef(false); // concurrent-load guard
  const sentinelRef = useRef(null);

  const loadFirst = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await api.getHomeTimeline({ cursor: 0, limit: PAGE_SIZE });
      setPosts(res.posts || []);
      setNextCursor(res.next_cursor || 0);
      setHasMore(Boolean(res.has_more));
    } catch (e) {
      setError(e.message || '加载失败');
    } finally {
      setLoading(false);
    }
  }, []);

  const loadMore = useCallback(async () => {
    const { hasMore: more, nextCursor: cursor } = pagerRef.current;
    if (fetchingRef.current || !more) return;
    fetchingRef.current = true;
    setLoadingMore(true);
    try {
      const res = await api.getHomeTimeline({ cursor, limit: PAGE_SIZE });
      setPosts((prev) => [...prev, ...(res.posts || [])]);
      setNextCursor(res.next_cursor || cursor);
      setHasMore(Boolean(res.has_more));
    } catch (e) {
      ui.Toast.show({ type: 'error', message: e.message || '加载失败' });
    } finally {
      fetchingRef.current = false;
      setLoadingMore(false);
    }
  }, []);

  useEffect(() => {
    loadFirst();
  }, [loadFirst]);

  // Infinite scroll: IntersectionObserver on a sentinel near the list bottom.
  useEffect(() => {
    const el = sentinelRef.current;
    if (!el || !hasMore) return undefined;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0] && entries[0].isIntersecting) loadMore();
      },
      { rootMargin: '200px' }
    );
    observer.observe(el);
    return () => observer.disconnect();
  }, [hasMore, loadMore]);

  const removePost = useCallback((postId) => {
    setPosts((prev) => prev.filter((p) => p.id !== postId));
  }, []);

  let body;
  if (loading) {
    body = (
      <div className="feed-list">
        <div className="feed-card"><ui.Skeleton circle width={48} height={48} /></div>
        <div className="feed-card"><ui.Skeleton width="100%" height={64} /></div>
        <div className="feed-card"><ui.Skeleton width="100%" height={64} /></div>
        <div className="feed-card"><ui.Skeleton width="100%" height={64} /></div>
      </div>
    );
  } else if (error) {
    body = (
      <ui.EmptyState
        icon="⚠️"
        title="加载失败"
        description={error}
        action={<ui.Button variant="primary" onClick={loadFirst}>重试</ui.Button>}
      />
    );
  } else if (posts.length === 0) {
    body = (
      <ui.EmptyState
        icon="📭"
        title="还没有帖子，来发第一条吧"
        description="点击上方输入框，分享你的新鲜事。"
      />
    );
  } else {
    body = (
      <div className="feed-list">
        {posts.map((post) => (
          <PostCard key={post.id} post={post} onDeleted={removePost} />
        ))}
        {hasMore && <div ref={sentinelRef} />}
        {loadingMore && <div className="feed-loading-more">加载中…</div>}
      </div>
    );
  }

  return (
    <div>
      <PostComposer onPublished={loadFirst} />
      <div className="feed-refresh-row">
        <ui.Button variant="ghost" onClick={loadFirst}>刷新</ui.Button>
      </div>
      {body}
    </div>
  );
}
