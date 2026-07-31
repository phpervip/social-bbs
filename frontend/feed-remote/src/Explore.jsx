import { useCallback, useEffect, useRef, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { api, ui } from '@b/shared';
import PostCard from './PostCard.jsx';

const PAGE_SIZE = 20;

/**
 * Explore — /explore?q= : search box (updates URL q) + cursor-paginated results.
 */
export default function Explore() {
  const [searchParams, setSearchParams] = useSearchParams();
  const q = (searchParams.get('q') || '').trim();

  const [input, setInput] = useState(q);
  const [posts, setPosts] = useState([]);
  const [nextCursor, setNextCursor] = useState(0);
  const [hasMore, setHasMore] = useState(false);
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState(null);

  const pagerRef = useRef({ hasMore: false, nextCursor: 0 });
  pagerRef.current = { hasMore, nextCursor };
  const fetchingRef = useRef(false);
  const sentinelRef = useRef(null);

  const submit = () => {
    const next = input.trim();
    setSearchParams(next ? { q: next } : {});
  };

  // Initial + q-change load (cancelled when q changes mid-flight).
  useEffect(() => {
    setInput(q);
    if (!q) {
      setPosts([]);
      setHasMore(false);
      setError(null);
      setLoading(false);
      return undefined;
    }
    let cancelled = false;
    (async () => {
      setLoading(true);
      setError(null);
      try {
        const res = await api.search({ q, cursor: 0, limit: PAGE_SIZE });
        if (cancelled) return;
        setPosts(res.posts || []);
        setNextCursor(res.next_cursor || 0);
        setHasMore(Boolean(res.has_more));
      } catch (err) {
        if (!cancelled) setError(err.message || '搜索失败');
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [q]);

  const loadMore = useCallback(async () => {
    if (!q) return;
    const { hasMore: more, nextCursor: cursor } = pagerRef.current;
    if (fetchingRef.current || !more) return;
    fetchingRef.current = true;
    setLoadingMore(true);
    try {
      const res = await api.search({ q, cursor, limit: PAGE_SIZE });
      setPosts((prev) => [...prev, ...(res.posts || [])]);
      setNextCursor(res.next_cursor || cursor);
      setHasMore(Boolean(res.has_more));
    } catch (err) {
      ui.Toast.show({ type: 'error', message: err.message || '加载失败' });
    } finally {
      fetchingRef.current = false;
      setLoadingMore(false);
    }
  }, [q]);

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

  let results;
  if (!q) {
    results = (
      <ui.EmptyState
        icon="🔍"
        title="输入关键词搜索帖子"
        description="搜索全站帖子内容，试试「咖啡」或「设计」。"
      />
    );
  } else if (loading) {
    results = (
      <div className="feed-list">
        <div className="feed-card"><ui.Skeleton width="100%" height={64} /></div>
        <div className="feed-card"><ui.Skeleton width="100%" height={64} /></div>
      </div>
    );
  } else if (error) {
    results = (
      <ui.EmptyState
        icon="⚠️"
        title="搜索失败"
        description={error}
        action={<ui.Button variant="primary" onClick={() => setSearchParams({ q })}>重试</ui.Button>}
      />
    );
  } else if (posts.length === 0) {
    results = <ui.EmptyState icon="📭" title="没有找到相关帖子" description="换个关键词试试吧。" />;
  } else {
    results = (
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
      <h2 className="feed-page-title">探索</h2>
      <div className="feed-search-form">
        <input
          className="feed-search-input"
          placeholder="搜索帖子…"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault();
              submit();
            }
          }}
        />
        <ui.Button variant="primary" onClick={submit}>搜索</ui.Button>
      </div>
      {results}
    </div>
  );
}
