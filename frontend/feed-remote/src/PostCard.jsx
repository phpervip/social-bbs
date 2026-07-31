import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { api, ui } from '@b/shared';
import { formatRelativeTime } from './format.js';
import { getCurrentUserId } from './currentUser.js';

const IMAGE_RE = /\.(png|jpe?g|gif|webp|svg|bmp)(\?.*)?$/i;

function isImage(url) {
  return IMAGE_RE.test(url);
}

/**
 * PostCard — avatar/name/time/content/media + action row
 * (♥ like optimistic, 💬 comments → /post/:id, 🗑 delete if owner).
 * @param {{ post: object, onDeleted?: (postId: number) => void }} props
 */
export default function PostCard({ post, onDeleted }) {
  const navigate = useNavigate();
  const currentUserId = getCurrentUserId();
  const [liked, setLiked] = useState(Boolean(post.liked_by_viewer));
  const [likeCount, setLikeCount] = useState(post.like_count || 0);
  const [liking, setLiking] = useState(false);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);

  const isOwner = currentUserId != null && Number(post.user_id) === currentUserId;

  const toggleLike = async () => {
    if (liking) return;
    const wasLiked = liked;
    // Optimistic flip — revert + Toast on failure.
    setLiked(!wasLiked);
    setLikeCount((c) => Math.max(0, c + (wasLiked ? -1 : 1)));
    setLiking(true);
    try {
      if (wasLiked) {
        await api.unlikePost(post.id);
      } else {
        await api.likePost(post.id);
      }
    } catch (e) {
      setLiked(wasLiked);
      setLikeCount((c) => Math.max(0, c + (wasLiked ? 1 : -1)));
      ui.Toast.show({ type: 'error', message: e.message || '操作失败，请重试' });
    } finally {
      setLiking(false);
    }
  };

  const confirmDelete = async () => {
    if (deleting) return;
    setDeleting(true);
    try {
      await api.deletePost(post.id);
      setConfirmOpen(false);
      ui.Toast.show({ type: 'success', message: '已删除' });
      if (onDeleted) onDeleted(post.id);
    } catch (e) {
      setConfirmOpen(false);
      ui.Toast.show({ type: 'error', message: e.message || '删除失败' });
    } finally {
      setDeleting(false);
    }
  };

  return (
    <article className="feed-card">
      <ui.Avatar src={post.avatar_url || undefined} name={post.display_name || post.username} size={48} />
      <div className="feed-card-main">
        <div className="feed-card-head">
          <span className="feed-card-name">{post.display_name || post.username}</span>
          <span className="feed-card-username">@{post.username}</span>
          <span className="feed-card-time">· {formatRelativeTime(post.created_at)}</span>
        </div>
        <p className="feed-card-content">{post.content}</p>
        {post.media_url &&
          (isImage(post.media_url) ? (
            <img className="feed-media" src={post.media_url} alt="媒体" loading="lazy" />
          ) : (
            <a className="feed-media-link" href={post.media_url} target="_blank" rel="noopener noreferrer">
              {post.media_url}
            </a>
          ))}
        <div className="feed-card-actions">
          <ui.Button variant="ghost" onClick={toggleLike} disabled={liking}>
            <span className={`feed-action${liked ? ' active' : ''}`}>
              {liked ? '♥' : '♡'} {likeCount}
            </span>
          </ui.Button>
          <ui.Button variant="ghost" onClick={() => navigate(`/post/${post.id}`)}>
            <span className="feed-action">💬 {post.comment_count || 0}</span>
          </ui.Button>
          {isOwner && (
            <ui.Button variant="ghost" onClick={() => setConfirmOpen(true)}>
              <span className="feed-action danger">🗑 删除</span>
            </ui.Button>
          )}
        </div>
      </div>

      <ui.Modal
        open={confirmOpen}
        onClose={() => setConfirmOpen(false)}
        title="删除帖子"
        width={360}
      >
        <p className="feed-confirm-text">确定删除这条帖子吗？此操作不可恢复。</p>
        <div className="feed-confirm-actions">
          <ui.Button variant="ghost" onClick={() => setConfirmOpen(false)}>
            取消
          </ui.Button>
          <ui.Button variant="danger" loading={deleting} onClick={confirmDelete}>
            删除
          </ui.Button>
        </div>
      </ui.Modal>
    </article>
  );
}
