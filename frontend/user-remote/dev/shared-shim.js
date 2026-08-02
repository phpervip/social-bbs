/* ============================================================
 * dev/shared-shim.js — LOCAL-DEV-ONLY stand-in for the Shell's
 * @b/shared module. Webpack alias (webpack.standalone.js) maps
 * '@b/shared' → this file so the exposed components can be verified
 * without the host. NEVER part of the Module Federation build.
 * API surface mirrors the frozen contract exactly.
 * ============================================================ */
import { useEffect } from 'react';
import { createRoot } from 'react-dom/client';
import axios from 'axios';

/* ------------------------------ api ------------------------------ */

const http = axios.create({ baseURL: '/api', timeout: 10000 });

http.interceptors.request.use((config) => {
  const token = localStorage.getItem('b_token');
  if (token) config.headers.Authorization = `Bearer ${token}`;
  return config;
});

http.interceptors.response.use(
  (res) => res.data?.data,
  (err) => Promise.reject(new Error(err.response?.data?.message || err.message))
);

const bus = {
  events: {
    AUTH_LOGIN: 'auth:login',
    AUTH_LOGOUT: 'auth:logout',
  },
  on(event, fn) {
    (bus._listeners[event] ||= []).push(fn);
  },
  off(event, fn) {
    bus._listeners[event] = (bus._listeners[event] || []).filter((f) => f !== fn);
  },
  emit(event, payload) {
    (bus._listeners[event] || []).forEach((fn) => {
      try {
        fn(payload);
      } catch (e) {
        console.error('[user-standalone bus]', e);
      }
    });
  },
  _listeners: {},
};

// AuthResponse { token, expires_in, user } — Auth.jsx stores the token and
// emits AUTH_LOGIN itself (same flow as the Shell's api-client contract).
const api = {
  register: (body) => http.post('/auth/register', body),
  login: (body) => http.post('/auth/login', body),
  logout: () => http.post('/auth/logout'),
  getProfile: (id) => http.get(`/user/${id}`), // data: { user }
  updateProfile: (body) => http.put('/user/profile', body), // data: { user }
  follow: (id) => http.post(`/user/${id}/follow`),
  unfollow: (id) => http.delete(`/user/${id}/follow`),
  getFollowers: ({ id, cursor, limit }) =>
    http.get(`/user/${id}/followers`, { params: { cursor, limit } }),
  getFollowing: ({ id, cursor, limit }) =>
    http.get(`/user/${id}/following`, { params: { cursor, limit } }),
};

/* ------------------------------- ui ------------------------------ */

const avatarStyle = (size) => ({
  width: size,
  height: size,
  borderRadius: '50%',
  display: 'inline-flex',
  alignItems: 'center',
  justifyContent: 'center',
  background: 'var(--brand-soft, rgba(139,90,43,.08))',
  color: 'var(--brand, #8B5A2B)',
  fontWeight: 600,
  fontSize: size / 2.4,
  overflow: 'hidden',
  flexShrink: 0,
});

function Avatar({ src, name, size = 40 }) {
  const initial = name ? [...name][0] : '?';
  return src ? (
    <img src={src} alt={name || 'avatar'} style={avatarStyle(size)} />
  ) : (
    <span style={avatarStyle(size)}>{initial}</span>
  );
}

const buttonStyle = (variant) => ({
  display: 'inline-flex',
  alignItems: 'center',
  justifyContent: 'center',
  gap: 6,
  padding: '8px 16px',
  border: 'none',
  borderRadius: 'var(--radius, 12px)',
  fontFamily: 'var(--font, sans-serif)',
  fontSize: 14,
  fontWeight: 600,
  cursor: 'pointer',
  transition: 'background-color 150ms ease, color 150ms ease',
  background:
    variant === 'primary'
      ? 'var(--brand, #8B5A2B)'
      : variant === 'danger'
        ? 'var(--danger, #ff4d4f)'
        : 'transparent',
  color: variant === 'primary' || variant === 'danger' ? '#fff' : 'var(--text-2, #536471)',
  opacity: 1,
});

function Button({ variant = 'ghost', disabled, loading, onClick, children }) {
  return (
    <button
      type="button"
      style={buttonStyle(variant)}
      disabled={disabled || loading}
      onClick={onClick}
    >
      {loading ? '…' : children}
    </button>
  );
}

const modalRootStyle = {
  position: 'fixed',
  inset: 0,
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
  background: 'rgba(0,0,0,.35)',
  zIndex: 1000,
};

const modalCardStyle = {
  background: 'var(--bg, #fff)',
  borderRadius: 'var(--radius, 12px)',
  padding: 20,
  fontFamily: 'var(--font, sans-serif)',
  maxWidth: '90vw',
};

function Modal({ open, onClose, title, children, width = 400 }) {
  useEffect(() => {
    if (!open) return undefined;
    const onKey = (e) => e.key === 'Escape' && onClose && onClose();
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [open, onClose]);

  if (!open) return null;
  return (
    <div style={modalRootStyle} onClick={onClose}>
      <div style={{ ...modalCardStyle, width }} onClick={(e) => e.stopPropagation()}>
        {title && (
          <h3 style={{ margin: '0 0 12px', fontSize: 16, color: 'var(--text, #0f1419)' }}>{title}</h3>
        )}
        {children}
      </div>
    </div>
  );
}

const toastStyle = {
  position: 'fixed',
  top: 16,
  left: '50%',
  transform: 'translateX(-50%)',
  zIndex: 2000,
  padding: '10px 16px',
  borderRadius: 'var(--radius, 12px)',
  fontFamily: 'var(--font, sans-serif)',
  fontSize: 14,
  color: '#fff',
  boxShadow: '0 4px 16px rgba(0,0,0,.15)',
};

const Toast = {
  show({ type = 'success', message }) {
    const root = document.createElement('div');
    root.style.cssText = 'position:fixed;inset:0;pointer-events:none;z-index:2000';
    document.body.appendChild(root);
    const onDone = () => {
      root.remove();
      document.removeEventListener('click', onDone);
    };
    document.addEventListener('click', onDone, { once: true });
    createRoot(root).render(
      <div
        style={{
          ...toastStyle,
          background: type === 'error' ? 'var(--danger, #ff4d4f)' : 'var(--success, #4caf50)',
        }}
      >
        {message}
      </div>
    );
    setTimeout(onDone, 2500);
  },
};

function Skeleton({ width = '100%', height = 16, circle = false }) {
  return (
    <span
      style={{
        display: 'inline-block',
        width: circle ? height : width,
        height,
        borderRadius: circle ? '50%' : 6,
        background: 'var(--border, #eff3f4)',
        animation: 'userShimmer 1.4s ease-in-out infinite',
      }}
    />
  );
}

if (typeof document !== 'undefined') {
  const style = document.createElement('style');
  style.textContent = `@keyframes userShimmer { 0%,100% { opacity: 1 } 50% { opacity: .45 } }`;
  document.head.appendChild(style);
}

function EmptyState({ icon, title, description, action }) {
  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        gap: 8,
        padding: '48px 16px',
        textAlign: 'center',
        fontFamily: 'var(--font, sans-serif)',
      }}
    >
      <span style={{ fontSize: 40 }}>{icon || '📭'}</span>
      <div style={{ fontWeight: 700, fontSize: 16, color: 'var(--text, #0f1419)' }}>{title}</div>
      {description && <div style={{ fontSize: 13, color: 'var(--text-2, #536471)' }}>{description}</div>}
      {action}
    </div>
  );
}

const ui = { Avatar, Button, Modal, Toast, Skeleton, EmptyState };

export { api, bus, ui };
