import axios from 'axios';
import { bus } from './event-bus';

/**
 * @b/shared — single axios instance (THE API CONTRACT).
 * baseURL /api, Bearer token from localStorage 'b_token',
 * 401 → logout + redirect; business error → reject(Error(message)).
 * All methods resolve with the response `data` payload directly.
 */

const TOKEN_KEY = 'b_token';

export function getToken() {
  return localStorage.getItem(TOKEN_KEY);
}

function storeToken(token) {
  if (token) localStorage.setItem(TOKEN_KEY, token);
  else localStorage.removeItem(TOKEN_KEY);
}

const api = axios.create({
  baseURL: '/api',
  timeout: 10000,
});

api.interceptors.request.use((config) => {
  const token = getToken();
  if (token) config.headers.Authorization = `Bearer ${token}`;
  return config;
});

api.interceptors.response.use(
  (response) => {
    const body = response.data;
    // Unified envelope { code, message, data } — return payload directly.
    if (body && typeof body === 'object' && 'code' in body) {
      if (body.code !== 0) {
        return Promise.reject(new Error(body.message || '请求失败'));
      }
      return body.data;
    }
    return body;
  },
  (error) => {
    if (error.response && error.response.status === 401) {
      storeToken(null);
      bus.emit(bus.events.AUTH_LOGOUT);
      if (window.location.pathname !== '/login') {
        window.location = '/login';
      }
      return Promise.reject(new Error('登录已过期，请重新登录'));
    }
    const message =
      error.response?.data?.message ||
      (error.code === 'ECONNABORTED' ? '请求超时，请稍后重试' : '网络异常，请稍后重试');
    return Promise.reject(new Error(message));
  }
);

export default {
  // Auth — pure HTTP wrappers. Token persistence + auth:login/auth:logout
  // bus emits are owned by the consumers (user-remote Auth.jsx) so the
  // client stays side-effect free; the 401 interceptor below still clears
  // the token + redirects on expired sessions.
  register: ({ username, email, password, display_name }) =>
    api.post('/auth/register', { username, email, password, display_name }),
  login: ({ account, password }) => api.post('/auth/login', { account, password }),
  logout: () => api.post('/auth/logout'),

  // User
  getProfile: (id) => api.get(`/user/${id}`),
  updateProfile: (patch) => api.put('/user/profile', patch),
  follow: (id) => api.post(`/user/${id}/follow`),
  unfollow: (id) => api.delete(`/user/${id}/follow`),
  getFollowers: ({ id, cursor = 0, limit = 20 } = {}) =>
    api.get(`/user/${id}/followers`, { params: { cursor, limit } }),
  getFollowing: ({ id, cursor = 0, limit = 20 } = {}) =>
    api.get(`/user/${id}/following`, { params: { cursor, limit } }),

  // Feed
  createPost: ({ content, media_url }) => api.post('/feed/post', { content, media_url }),
  getHomeTimeline: ({ cursor = 0, limit = 20 } = {}) =>
    api.get('/feed/home', { params: { cursor, limit } }),
  getPost: (id) => api.get(`/feed/post/${id}`),
  deletePost: (id) => api.delete(`/feed/post/${id}`),
  likePost: (postId) => api.post('/feed/like', { post_id: postId }),
  unlikePost: (postId) => api.delete('/feed/like', { data: { post_id: postId } }),
  addComment: ({ post_id, content }) => api.post('/feed/comment', { post_id, content }),
  getComments: ({ post_id, cursor = 0, limit = 20 } = {}) =>
    api.get(`/feed/post/${post_id}/comments`, { params: { cursor, limit } }),

  // Search
  search: ({ q, cursor = 0, limit = 20 } = {}) =>
    api.get('/search', { params: { q, cursor, limit } }),
};

// Video (P3) — chunked upload + playback. uploadChunk sends base64 in a JSON
// body (the gateway decodes Buffer.from(body.data, 'base64')); visibility must
// be the full proto enum (VIDEO_VISIBILITY_*).
export const videoApi = {
  initUpload: (data) => api.post('/video/init-upload', data),
  uploadChunk: ({ upload_id, part_number, data }) =>
    api.post('/video/upload-chunk', { upload_id, part_number, data }),
  completeUpload: (data) => api.post('/video/complete-upload', data),
  getVideo: (id) => api.get(`/video/${id}`),
  getPlayback: (id) => api.get(`/video/${id}/playback`),
  getTranscodeStatus: (id) => api.get(`/video/${id}/transcode-status`),
  deleteVideo: (id) => api.delete(`/video/${id}`),
  listUserVideos: (userId, params) => api.get(`/video/user/${userId}`, { params }),
};
