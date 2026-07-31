import { useCallback, useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { api, ui } from '@b/shared';
import PostCard from './PostCard.jsx';
import { formatRelativeTime } from './format.js';

const COMMENT_MAX = 500;

/**
 * PostDetail — /post/:id : post card + newest-first comments + comment composer.
 */
export default function PostDetail() {
  const { id } = useParams();
  const navigate = useNavigate();

  const [post, setPost] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [comments, setComments] = useState([]);
  const [commentsLoading, setCommentsLoading] = useState(true);
  const [commentText, setCommentText] = useState('');
  const [commenting, setCommenting] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    setCommentsLoading(true);
    try {
      const [postRes, commentsRes] = await Promise.all([
        api.getPost(id),
        api.getComments({ post_id: id, cursor: 0, limit: 20 }),
      ]);
      setPost(postRes);
      setComments(commentsRes.comments || []);
    } catch (e) {
      setError(e.message || '加载失败');
    } finally {
      setLoading(false);
      setCommentsLoading(false);
    }
  }, [id]);

  useEffect(() => {
    load();
  }, [load]);

  const submitComment = async () => {
    const content = commentText.trim();
    if (!content || commenting) return;
    setCommenting(true);
    try {
      const comment = await api.addComment({ post_id: Number(id), content });
      setComments((prev) => [comment, ...prev]);
      setCommentText('');
      setPost((prev) => (prev ? { ...prev, comment_count: (prev.comment_count || 0) + 1 } : prev));
      ui.Toast.show({ type: 'success', message: '评论成功' });
    } catch (e) {
      ui.Toast.show({ type: 'error', message: e.message || '评论失败' });
    } finally {
      setCommenting(false);
    }
  };

  if (loading) {
    return (
      <div className="feed-detail">
        <div className="feed-card"><ui.Skeleton width="100%" height={96} /></div>
        <div className="feed-card"><ui.Skeleton width="100%" height={160} /></div>
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
    <div className="feed-detail">
      <ui.Button variant="ghost" onClick={() => navigate(-1)}>
        <span className="feed-action">← 返回</span>
      </ui.Button>
      <PostCard post={post} onDeleted={() => navigate('/home')} />

      <div className="feed-comment-composer">
        <ui.Avatar name="我" size={36} />
        <input
          className="feed-comment-input"
          placeholder="写下你的评论…"
          value={commentText}
          maxLength={COMMENT_MAX}
          onChange={(e) => setCommentText(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && !e.shiftKey) {
              e.preventDefault();
              submitComment();
            }
          }}
        />
        <ui.Button variant="primary" disabled={!commentText.trim() || commenting} loading={commenting} onClick={submitComment}>
          发布
        </ui.Button>
      </div>

      <h3 className="feed-comments-title">评论</h3>
      <div className="feed-comments">
        {commentsLoading ? (
          <div className="feed-card"><ui.Skeleton width="100%" height={64} /></div>
        ) : comments.length === 0 ? (
          <ui.EmptyState icon="💬" title="暂无评论" description="成为第一个评论的人吧。" />
        ) : (
          comments.map((c) => (
            <div key={c.id} className="feed-comment">
              <ui.Avatar src={c.avatar_url || undefined} name={c.display_name || c.username} size={36} />
              <div className="feed-comment-main">
                <div className="feed-comment-head">
                  <span className="feed-comment-name">{c.display_name || c.username}</span>
                  <span className="feed-comment-username">@{c.username}</span>
                  <span className="feed-comment-time">· {formatRelativeTime(c.created_at)}</span>
                </div>
                <p className="feed-comment-content">{c.content}</p>
              </div>
            </div>
          ))
        )}
      </div>
    </div>
  );
}
