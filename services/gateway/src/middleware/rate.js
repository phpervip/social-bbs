'use strict';

// Layer 3 — rate limiting: Redis fixed-window via INCR + EXPIRE.
// Key: rate:{window}:{clientIp}:{userId or 'anon'}
// Tiers: anon 30/min, authed 100/min, publish endpoints 10/min.
// The Redis client is abstracted behind a tiny interface so unit tests
// can inject a mock (no live Redis needed).

// Publish endpoints get the strictest tier. Keyed as `${method} ${path}`.
const PUBLISH_ENDPOINTS = new Set([
  'POST /api/feed/post',
  'POST /api/feed/like',
  'DELETE /api/feed/like',
  'POST /api/feed/comment',
]);

// Pure function — decides which tier applies to a request. Export for tests.
function rateTierFor({ method, url, isAuthed }) {
  const path = url.split('?')[0];
  if (isAuthed && PUBLISH_ENDPOINTS.has(`${method} ${path}`)) return 'publish';
  return isAuthed ? 'authed' : 'anon';
}

const TIER_LIMITS = {
  anon: 30,
  authed: 100,
  publish: 10,
};

const WINDOW_SECONDS = 60;

function createRateMiddleware({ client, limits = TIER_LIMITS }) {
  if (!client || typeof client.incr !== 'function') {
    throw new Error('rate middleware requires a Redis-like client with incr()');
  }

  return async function rateMiddleware(request, reply) {
    if (request.url === '/healthz') return;

    const isAuthed = Boolean(request.user);
    const tier = rateTierFor({ method: request.method, url: request.url, isAuthed });
    const limit = limits[tier];

    const window = Math.floor(Date.now() / 1000 / WINDOW_SECONDS);
    const clientIp = request.ip || 'unknown';
    const userId = isAuthed ? String(request.user.id) : 'anon';
    const key = `rate:${window}:${clientIp}:${userId}`;

    let count;
    try {
      count = await client.incr(key);
      if (count === 1) {
        await client.expire(key, WINDOW_SECONDS);
      }
    } catch (err) {
      // Redis is a soft dependency for rate limiting — never fail the request
      // because the counter is down (P1 dev environment may run without Redis).
      request.log?.warn({ err: err.message, tier }, 'rate limiter unavailable, allowing request');
      return;
    }

    if (count > limit) {
      return reply.status(429).send({ code: 429, message: 'rate_limited', data: null });
    }
  };
}

module.exports = { createRateMiddleware, rateTierFor, TIER_LIMITS, WINDOW_SECONDS, PUBLISH_ENDPOINTS };
