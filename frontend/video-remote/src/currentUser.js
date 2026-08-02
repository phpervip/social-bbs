/**
 * Derive the current user id from the JWT stored by the shared api client
 * (localStorage 'b_token'). Mirrors feed-remote/src/currentUser.js — used for
 * the video list's default owner. No token handling here (that belongs to @b/shared).
 * @returns {number|null}
 */
export function getCurrentUserId() {
  try {
    const token = localStorage.getItem('b_token');
    if (!token) return null;
    const segment = token.split('.')[1];
    if (!segment) return null;
    // JWT payload is base64url — normalize to standard base64 before atob.
    const b64 = segment.replace(/-/g, '+').replace(/_/g, '/');
    const payload = JSON.parse(atob(b64));
    return payload.sub == null ? null : Number(payload.sub);
  } catch {
    return null;
  }
}