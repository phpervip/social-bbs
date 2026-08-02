'use strict';

// Unified envelope + REST->gRPC forwarding through the assembled app,
// using a stub gRPC client and a mock Redis client (no live services).

const test = require('node:test');
const assert = require('node:assert/strict');
const jwt = require('jsonwebtoken');
const { buildApp } = require('../src/app');
const { GRPC_STATUS_CODE } = require('../src/middleware/grpcCodes');

const SECRET = 'test-secret';

// Stub feed client: controllable per-method behavior.
function stubFeedClient(handlers) {
  const METHODS = ['createPost', 'getPost', 'getHomeTimeline', 'deletePost', 'likePost', 'unlikePost', 'addComment', 'getComments', 'search'];
  const client = {};
  for (const m of METHODS) {
    client[m] = handlers[m] || (async () => ({}));
  }
  client.close = () => {};
  return client;
}

const allowRedis = {
  incr: async () => 1,
  expire: async () => 1,
  get: async () => null,
};

function makeApp({ handlers = {}, redis = allowRedis } = {}) {
  return buildApp({
    logger: require('pino')({ level: 'silent' }),
    redis,
    feedClient: stubFeedClient(handlers),
    jwtSecret: SECRET,
  });
}

function tokenFor(userId) {
  return jwt.sign({ sub: String(userId), iat: Math.floor(Date.now() / 1000) }, SECRET, { algorithm: 'HS256', expiresIn: 3600 });
}

test('GET /healthz returns envelope without auth', async () => {
  const app = makeApp();
  const res = await app.inject({ method: 'GET', url: '/healthz' });
  assert.equal(res.statusCode, 200);
  assert.deepEqual(res.json(), { code: 0, message: 'ok', data: null });
  await app.close();
});

test('protected route without token -> 401 envelope', async () => {
  const app = makeApp();
  const res = await app.inject({ method: 'GET', url: '/api/feed/home' });
  assert.equal(res.statusCode, 401);
  assert.deepEqual(res.json(), { code: 401, message: 'unauthorized', data: null });
  await app.close();
});

test('successful forwarding returns {code:0,message:"success",data}', async () => {
  const post = { id: 10, user_id: 1, username: 'bob', content: 'hello', like_count: 0, comment_count: 0, liked_by_viewer: false, created_at: 123 };
  const app = makeApp({
    handlers: { createPost: async (req) => ({ ...post, ...req }) },
  });
  const res = await app.inject({
    method: 'POST',
    url: '/api/feed/post',
    headers: { authorization: `Bearer ${tokenFor(1)}` },
    payload: { content: 'hello world' },
  });
  assert.equal(res.statusCode, 200);
  const body = res.json();
  assert.deepEqual(
    Object.keys(body).sort(),
    ['code', 'data', 'message'],
    'envelope keys must be exactly code/message/data'
  );
  assert.equal(body.code, 0);
  assert.equal(body.message, 'success');
  assert.equal(body.data.content, 'hello world');
  await app.close();
});

test('gRPC error codes surface through post mapping (404/400/503/409)', async () => {
  const handlers = {
    getPost: async () => { const e = new Error('no post'); e.code = GRPC_STATUS_CODE.NOT_FOUND; throw e; },
    search: async () => { const e = new Error('bad q'); e.code = GRPC_STATUS_CODE.INVALID_ARGUMENT; throw e; },
    getHomeTimeline: async () => { const e = new Error('feed down'); e.code = GRPC_STATUS_CODE.UNAVAILABLE; throw e; },
    likePost: async () => { const e = new Error('dup'); e.code = GRPC_STATUS_CODE.ALREADY_EXISTS; throw e; },
  };
  const app = makeApp({ handlers });
  const auth = { authorization: `Bearer ${tokenFor(1)}` };

  let res = await app.inject({ method: 'GET', url: '/api/feed/post/5', headers: auth });
  assert.equal(res.statusCode, 404);
  assert.deepEqual(res.json(), { code: 404, message: 'not_found', data: null });

  res = await app.inject({ method: 'GET', url: '/api/search?q=x', headers: auth });
  assert.equal(res.statusCode, 400);
  assert.equal(res.json().code, 400);

  res = await app.inject({ method: 'GET', url: '/api/feed/home', headers: auth });
  assert.equal(res.statusCode, 503);
  assert.deepEqual(res.json(), { code: 503, message: 'service_unavailable', data: null });

  res = await app.inject({ method: 'POST', url: '/api/feed/like', headers: auth, payload: { post_id: 1 } });
  assert.equal(res.statusCode, 409);
  assert.deepEqual(res.json(), { code: 409, message: 'already_exists', data: null });

  await app.close();
});

test('user_id is taken from the token, not the body', async () => {
  let received = null;
  const app = makeApp({
    handlers: { createPost: async (req) => { received = req; return {}; } },
  });
  await app.inject({
    method: 'POST',
    url: '/api/feed/post',
    headers: { authorization: `Bearer ${tokenFor(4)}` },
    payload: { content: 'spoofed', user_id: 999 },
  });
  assert.equal(received.user_id, 4);
  await app.close();
});

test('path :id must be a positive integer else 400', async () => {
  const app = makeApp();
  const auth = { authorization: `Bearer ${tokenFor(1)}` };
  for (const url of ['/api/feed/post/0', '/api/feed/post/-1', '/api/feed/post/abc']) {
    const res = await app.inject({ method: 'GET', url, headers: auth });
    assert.equal(res.statusCode, 400, url);
    assert.equal(res.json().code, 400);
  }
  const res = await app.inject({ method: 'GET', url: '/api/feed/post/0/comments', headers: auth });
  assert.equal(res.statusCode, 400);
  await app.close();
});

test('search requires q else 400', async () => {
  const app = makeApp();
  const res = await app.inject({
    method: 'GET',
    url: '/api/search',
    headers: { authorization: `Bearer ${tokenFor(1)}` },
  });
  assert.equal(res.statusCode, 400);
  assert.equal(res.json().code, 400);
  await app.close();
});

test('unknown route -> 404 envelope (after auth)', async () => {
  const app = makeApp();
  const res = await app.inject({
    method: 'GET',
    url: '/api/nope',
    headers: { authorization: `Bearer ${tokenFor(1)}` },
  });
  assert.equal(res.statusCode, 404);
  assert.deepEqual(res.json(), { code: 404, message: 'not_found', data: null });
  await app.close();
});

test('rate middleware counts requests via redis key and enforces publish tier', async () => {
  const keys = new Map();
  const countingRedis = {
    incr: async (k) => {
      const v = (keys.get(k) || 0) + 1;
      keys.set(k, v);
      return v;
    },
    expire: async () => 1,
  };
  const app = makeApp({ redis: countingRedis, handlers: { likePost: async () => ({}) } });
  const auth = { authorization: `Bearer ${tokenFor(1)}` };

  let lastStatus = 200;
  for (let i = 0; i < 12; i++) {
    const res = await app.inject({ method: 'POST', url: '/api/feed/like', headers: auth, payload: { post_id: 1 } });
    lastStatus = res.statusCode;
  }
  assert.equal(lastStatus, 429);
  const last = await app.inject({ method: 'POST', url: '/api/feed/like', headers: auth, payload: { post_id: 1 } });
  assert.deepEqual(last.json(), { code: 429, message: 'rate_limited', data: null });
  await app.close();
});
