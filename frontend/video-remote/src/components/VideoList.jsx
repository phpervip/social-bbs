import React, { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { api } from '@b/shared';
import { getCurrentUserId } from '../currentUser';

const PAGE_SIZE = 20;

export default function VideoList({ userId }) {
  const [videos, setVideos] = useState([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const uid = userId || getCurrentUserId();
    if (!uid) {
      setLoading(false);
      return;
    }
    let cancelled = false;
    api
      .get(`/video/user/${uid}`, { params: { cursor: 0, limit: PAGE_SIZE } })
      .then((res) => {
        if (!cancelled) setVideos(res.videos || []);
      })
      .catch(() => {
        // Public read route — a failure just renders the empty state.
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [userId]);

  if (loading) return <div style={{ padding: 24 }}>加载中...</div>;
  if (!videos.length) return <div style={{ padding: 24, color: '#888' }}>暂无视频</div>;

  return (
    <div style={{ padding: 24 }}>
      <h2 style={{ color: '#8B5A2B', marginBottom: 16 }}>视频列表</h2>
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(280px, 1fr))', gap: 16 }}>
        {videos.map((v) => (
          <Link key={v.id} to={`/video/${v.id}`} style={{ textDecoration: 'none', color: 'inherit' }}>
            <div style={{ background: '#fff', borderRadius: 8, overflow: 'hidden', boxShadow: '0 1px 3px rgba(0,0,0,.1)' }}>
              <div
                style={{
                  height: 160,
                  background: '#f5f5f5',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  color: '#8B5A2B',
                  fontSize: 48,
                }}
              >
                ▶
              </div>
              <div style={{ padding: 12 }}>
                <div style={{ fontWeight: 600, marginBottom: 4 }}>{v.title || '无标题'}</div>
                <div style={{ fontSize: 12, color: '#888' }}>
                  {v.created_at ? new Date(v.created_at).toLocaleDateString() : ''}
                </div>
              </div>
            </div>
          </Link>
        ))}
      </div>
    </div>
  );
}