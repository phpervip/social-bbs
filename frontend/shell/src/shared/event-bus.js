/**
 * @b/shared — Event bus (pub/sub). Singleton, no dependencies.
 * Contract: { on(event, fn), off(event, fn), emit(event, payload) }
 * Listeners run synchronously; errors are console.error'd and never thrown.
 */
export const EVENTS = Object.freeze({
  AUTH_LOGIN: 'auth:login',
  AUTH_LOGOUT: 'auth:logout',
  POST_CREATED: 'post:created',
  PROFILE_UPDATED: 'profile:updated',
  // Video (P3)
  VIDEO_UPLOAD_START: 'video:upload:start',
  VIDEO_UPLOAD_DONE: 'video:upload:done',
  VIDEO_UPLOAD_FAIL: 'video:upload:fail',
  VIDEO_TRANSCODE_DONE: 'video:transcode:done',
});

const listeners = new Map();

function on(event, fn) {
  if (typeof fn !== 'function') return;
  if (!listeners.has(event)) listeners.set(event, new Set());
  listeners.get(event).add(fn);
  return () => off(event, fn);
}

function off(event, fn) {
  const set = listeners.get(event);
  if (!set) return;
  set.delete(fn);
  if (set.size === 0) listeners.delete(event);
}

function emit(event, payload) {
  const set = listeners.get(event);
  if (!set) return;
  set.forEach((fn) => {
    try {
      fn(payload);
    } catch (err) {
      console.error(`[bus] listener error on "${event}"`, err);
    }
  });
}

export const bus = {
  on,
  off,
  emit,
  events: EVENTS,
};
