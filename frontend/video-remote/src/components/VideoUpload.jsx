import React, { useState, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import { api, bus } from '@b/shared';

const CHUNK_SIZE = 5 * 1024 * 1024; // 5MB

// Gateway normalizes visibility to the full proto enum (video.proto) — anything
// else silently falls back to PUBLIC, so send the enum values verbatim.
const VISIBILITY_OPTIONS = [
  { value: 'VIDEO_VISIBILITY_PUBLIC', label: '公开' },
  { value: 'VIDEO_VISIBILITY_FOLLOWERS_ONLY', label: '仅粉丝可见' },
  { value: 'VIDEO_VISIBILITY_PRIVATE', label: '仅自己可见' },
];

// The gateway's upload-chunk route expects `data` as base64 in a JSON body
// (Buffer.from(body.data, 'base64')) — NOT multipart/form-data.
function readChunkAsBase64(blob) {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      const result = reader.result; // "data:<mime>;base64,<payload>"
      const comma = result.indexOf(',');
      resolve(comma >= 0 ? result.slice(comma + 1) : result);
    };
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(blob);
  });
}

export default function VideoUpload() {
  const [file, setFile] = useState(null);
  const [title, setTitle] = useState('');
  const [description, setDescription] = useState('');
  const [visibility, setVisibility] = useState('VIDEO_VISIBILITY_PUBLIC');
  const [uploading, setUploading] = useState(false);
  const [progress, setProgress] = useState(0);
  const [error, setError] = useState('');
  const navigate = useNavigate();
  const fileRef = useRef();

  const handleUpload = async () => {
    if (!file || !title.trim()) {
      setError('请选择视频并填写标题');
      return;
    }
    setUploading(true);
    setError('');
    setProgress(0);
    try {
      // 1. Init upload — returns upload_id + video_id (+ already-uploaded parts for resume).
      const initRes = await api.post('/video/init-upload', {
        filename: file.name,
        content_type: file.type || 'video/mp4',
        total_size: file.size,
      });
      const { upload_id, video_id, uploaded_chunks = [] } = initRes;
      bus.emit(bus.events.VIDEO_UPLOAD_START, { video_id, filename: file.name });

      // 2. Upload chunks (base64 JSON body; skip parts the server already has).
      const done = new Set(uploaded_chunks);
      const totalChunks = Math.ceil(file.size / CHUNK_SIZE);
      for (let i = 1; i <= totalChunks; i++) {
        if (done.has(i)) {
          setProgress(Math.round((i / totalChunks) * 100));
          continue;
        }
        const start = (i - 1) * CHUNK_SIZE;
        const end = Math.min(i * CHUNK_SIZE, file.size);
        const data = await readChunkAsBase64(file.slice(start, end));
        await api.post('/video/upload-chunk', { upload_id, part_number: i, data });
        setProgress(Math.round((i / totalChunks) * 100));
      }

      // 3. Complete upload — gateway unwraps data.video, so the response IS the video.
      await api.post('/video/complete-upload', {
        upload_id,
        title: title.trim(),
        description: description.trim(),
        visibility,
      });
      bus.emit(bus.events.VIDEO_UPLOAD_DONE, { video_id });
      navigate(`/video/${video_id}`);
    } catch (err) {
      setError(err.message || '上传失败');
      bus.emit(bus.events.VIDEO_UPLOAD_FAIL, { error: err.message });
    } finally {
      setUploading(false);
    }
  };

  return (
    <div style={{ maxWidth: 600, margin: '0 auto', padding: 24 }}>
      <h2 style={{ color: '#8B5A2B', marginBottom: 16 }}>上传视频</h2>
      {error && <div style={{ color: '#dc2626', marginBottom: 12 }}>{error}</div>}
      <div style={{ marginBottom: 12 }}>
        <input
          ref={fileRef}
          type="file"
          accept="video/mp4,video/webm,video/quicktime"
          onChange={(e) => {
            const picked = e.target.files?.[0];
            setFile(picked);
            setTitle(picked?.name?.replace(/\.[^.]+$/, '') || '');
          }}
          style={{ display: 'none' }}
        />
        <button
          type="button"
          onClick={() => fileRef.current?.click()}
          disabled={uploading}
          style={{
            padding: '8px 16px',
            background: '#8B5A2B',
            color: '#fff',
            border: 'none',
            borderRadius: 6,
            cursor: 'pointer',
          }}
        >
          {file ? file.name : '选择视频'}
        </button>
      </div>
      <input
        value={title}
        onChange={(e) => setTitle(e.target.value)}
        placeholder="标题"
        disabled={uploading}
        style={{ width: '100%', padding: 8, marginBottom: 12, border: '1px solid #d9d9d9', borderRadius: 6 }}
      />
      <textarea
        value={description}
        onChange={(e) => setDescription(e.target.value)}
        placeholder="描述（可选）"
        disabled={uploading}
        rows={3}
        style={{ width: '100%', padding: 8, marginBottom: 12, border: '1px solid #d9d9d9', borderRadius: 6 }}
      />
      <select
        value={visibility}
        onChange={(e) => setVisibility(e.target.value)}
        disabled={uploading}
        style={{ width: '100%', padding: 8, marginBottom: 12, border: '1px solid #d9d9d9', borderRadius: 6 }}
      >
        {VISIBILITY_OPTIONS.map((opt) => (
          <option key={opt.value} value={opt.value}>
            {opt.label}
          </option>
        ))}
      </select>
      {uploading && (
        <div style={{ marginBottom: 12 }}>
          <div style={{ height: 8, background: '#f0f0f0', borderRadius: 4 }}>
            <div
              style={{
                height: '100%',
                width: `${progress}%`,
                background: '#8B5A2B',
                borderRadius: 4,
                transition: 'width 0.3s',
              }}
            />
          </div>
          <span style={{ fontSize: 12, color: '#888' }}>{progress}%</span>
        </div>
      )}
      <button
        type="button"
        onClick={handleUpload}
        disabled={uploading || !file}
        style={{
          padding: '10px 24px',
          background: '#8B5A2B',
          color: '#fff',
          border: 'none',
          borderRadius: 6,
          cursor: 'pointer',
          opacity: !file || uploading ? 0.5 : 1,
        }}
      >
        {uploading ? '上传中...' : '开始上传'}
      </button>
    </div>
  );
}