'use strict';

// Rate tier selection (pure function) + rate middleware behavior with a
// mocked Redis client. No live Redis required.

const test = require('node:test');
const assert = require('node:assert/strict');
const { rateTierFor, createRateMiddleware, TIER_LIMITS } = require('../src/middleware/rate');

test('anon requests classify as anon tier (30/min)', () => {
  assert.equal(rateTierFor({ method: 'GET', url: '/api/feed/home', isAuthed: false }), 'anon');
  assert.equal(TIER_LIMITS.anon, 30);
});

test('authed requests classify as authed tier (100/min)', () => {
  assert.equal(rateTierFor({ method: 'GET', url: '/api/feed/home', isAuthed: true }), 'authed');
  assert.equal(TIER_LIMITS.authed, 100);
});

test('publish endpoints classify as publish tier (10/min) for authed users', () => {
  const publishes = [
    ['POST', '/api/feed/post'],
    ['POST', '/api/feed/like'],
    ['DELETE', '/api/feed/like'],
    ['POST', '/api/feed/comment'],
  ];
  for (const [method, url] of publishes) {
    assert.equal(rateTierFor({ method, url, isAuthed: true }), 'publish', `${method} ${url}`);
  }
  assert.equal(TIER_LIMITS.publish, 10);
});

test('publish endpoint for anon user is still anon tier', () => {
  assert.equal(rateTierFor({ method: 'POST', url: '/api/feed/post', isAuthed: false }), 'anon');
});

test('follow endpoints classify as publish tier (10/min) for authed users', () => {
  for (const method of ['POST', 'DELETE']) {
    assert.equal(rateTierFor({ method, url: '/api/user/1/follow', isAuthed: true }), 'publish', `${method} /api/user/1/follow`);
    assert.equal(rateTierFor({ method, url: '/api/user/42/follow?x=1', isAuthed: true }), 'publish', `${method} with query string`);
  }
  // Read-only user routes stay on the authed tier.
  assert.equal(rateTierFor({ method: 'GET', url: '/api/user/1/followers', isAuthed: true }), 'authed');
  assert.equal(rateTierFor({ method: 'GET', url: '/api/user/1/following', isAuthed: true }), 'authed');
  assert.equal(rateTierFor({ method: 'GET', url: '/api/user/1', isAuthed: true }), 'authed');
  assert.equal(rateTierFor({ method: 'PUT', url: '/api/user/profile', isAuthed: true }), 'authed');
  // Non-follow paths with a similar shape must NOT match.
  assert.equal(rateTierFor({ method: 'POST', url: '/api/user/1/followers', isAuthed: true }), 'authed');
  assert.equal(rateTierFor({ method: 'POST', url: '/api/user/follow', isAuthed: true }), 'authed');
  // Anon follow requests stay anon.
  assert.equal(rateTierFor({ method: 'POST', url: '/api/user/1/follow', isAuthed: false }), 'anon');
});

test('rate middleware blocks when count exceeds tier limit', async () => {
  let count = 0;
  const mockRedis = {
    incr: async () => ++count,
    expire: async () => 1,
  };
  const middleware = createRateMiddleware({ client: mockRedis });

  const req = { method: 'POST', url: '/api/feed/post', ip: '1.2.3.4', user: { id: 7 }, log: { warn() {} } };
  const reply = { status: (s) => ({ send: (body) => ({ status: s, body }) }) };

  // 11th request on the publish tier -> 429 (limit is 10/min)
  let result;
  for (let i = 0; i < 10; i++) result = await middleware(req, reply);
  assert.equal(result, undefined); // first 10 allowed
  result = await middleware(req, reply);
  assert.equal(result.status, 429);
  assert.deepEqual(result.body, { code: 429, message: 'rate_limited', data: null });

  // Key shape: rate:{window}:{ip}:{userId or anon}
  let seenKey = null;
  const captured = { incr: async (k) => { seenKey = k; return 1; }, expire: async () => 1 };
  const m2 = createRateMiddleware({ client: captured });
  await m2({ method: 'GET', url: '/api/feed/home', ip: '9.9.9.9', user: { id: 42 }, log: { warn() {} } }, reply);
  assert.match(seenKey, /^rate:\d+:9\.9\.9\.9:42$/);
  const m3 = createRateMiddleware({ client: captured });
  await m3({ method: 'GET', url: '/api/feed/home', ip: '9.9.9.9', log: { warn() {} } }, reply);
  assert.match(seenKey, /^rate:\d+:9\.9\.9\.9:anon$/);
});

test('rate middleware allows when count within limit', async () => {
  let count = 0;
  const mockRedis = {
    incr: async () => ++count,
    expire: async () => 1,
  };
  const middleware = createRateMiddleware({ client: mockRedis });
  const req = { method: 'GET', url: '/api/feed/home', ip: '1.2.3.4', user: { id: 7 }, log: { warn() {} } };
  const reply = { status: () => ({ send: () => 'sent' }) };
  const result = await middleware(req, reply); // count 1 of 100
  assert.equal(result, undefined); // no reply -> allowed
});

test('rate middleware fails open when Redis is down', async () => {
  const broken = { incr: async () => { throw new Error('ECONNREFUSED'); }, expire: async () => 1 };
  const middleware = createRateMiddleware({ client: broken });
  const req = { method: 'GET', url: '/api/feed/home', ip: '1.2.3.4', user: { id: 7 }, log: { warn() {} } };
  const reply = { status: () => ({ send: () => 'sent' }) };
  const result = await middleware(req, reply);
  assert.equal(result, undefined); // request allowed despite Redis outage
});

test('first request in a window sets expiry on the key', async () => {
  const calls = [];
  const mockRedis = {
    incr: async (k) => { calls.push(['incr', k]); return 1; },
    expire: async (k, s) => { calls.push(['expire', k, s]); return 1; },
  };
  const middleware = createRateMiddleware({ client: mockRedis });
  await middleware({ method: 'GET', url: '/api/feed/home', ip: '1.2.3.4', log: { warn() {} } }, { status: () => ({ send: () => 'sent' }) });
  assert.equal(calls.length, 2);
  assert.equal(calls[0][0], 'incr');
  assert.equal(calls[1][0], 'expire');
  assert.equal(calls[1][2], 60);
});
