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
  // Auth (dev-only in P1)
  async devLogin(userId) {
    const data = await api.post('/dev/login', { user_id: userId });
    storeToken(data.token);
    bus.emit(bus.events.AUTH_LOGIN, data.token);
    return data.token;
  },
  devUsers: () => api.get('/dev/users'),

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
