import React, { useEffect, useRef, useState } from 'react';
import { useParams } from 'react-router-dom';
import Hls from 'hls.js';
import { api } from '@b/shared';

// Proto enum (video.proto) — the gateway returns these verbatim.
const STATUS_LABELS = {
  VIDEO_STATUS_PENDING: '⏳ 等待处理',
  VIDEO_STATUS_PROCESSING: '⏳ 转码中...',
  VIDEO_STATUS_COMPLETED: '✓ 已转码',
  VIDEO_STATUS_FAILED: '✗ 转码失败',
};

export default function VideoPlayer() {
  const { id } = useParams();
  const [video, setVideo] = useState(null);
  const [playbackUrl, setPlaybackUrl] = useState('');
  const [error, setError] = useState('');
  const videoRef = useRef();

  useEffect(() => {
    let cancelled = false;
    api
      .get(`/video/${id}`)
      .then((res) => {
        if (!cancelled) setVideo(res);
      })
      .catch((err) => {
        if (!cancelled) setError(err.message || '无法加载视频');
      });
    api
      .get(`/video/${id}/playback`)
      .then((res) => {
        if (!cancelled) setPlaybackUrl(res.playback_url || '');
      })
      .catch(() => {
        if (!cancelled) setError('无法获取播放地址');
      });
    return () => {
      cancelled = true;
    };
  }, [id]);

  useEffect(() => {
    if (!playbackUrl || !videoRef.current) return undefined;
    const video = videoRef.current;
    if (Hls.isSupported()) {
      const hls = new Hls();
      hls.loadSource(playbackUrl);
      hls.attachMedia(video);
      hls.on(Hls.Events.ERROR, (_, data) => {
        if (data.fatal) setError('播放失败');
      });
      return () => hls.destroy();
    }
    if (video.canPlayType('application/vnd.apple.mpegurl')) {
      video.src = playbackUrl;
    }
    return undefined;
  }, [playbackUrl]);

  if (error) return <div style={{ padding: 24, color: '#dc2626' }}>{error}</div>;
  if (!video) return <div style={{ padding: 24 }}>加载中...</div>;

  return (
    <div style={{ maxWidth: 800, margin: '0 auto', padding: 24 }}>
      <video ref={videoRef} controls style={{ width: '100%', borderRadius: 8, background: '#000' }} />
      <h3 style={{ color: '#8B5A2B', marginTop: 12 }}>{video.title}</h3>
      <p style={{ color: '#888' }}>{video.description}</p>
      <div style={{ fontSize: 12, color: '#aaa' }}>{STATUS_LABELS[video.status] || video.status}</div>
    </div>
  );
}