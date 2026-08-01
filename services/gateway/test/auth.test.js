'use strict';

// Auth + user REST routes through the assembled app: public register/login,
// logout jti forwarding, blacklist rejection, gRPC error mapping. Uses stub
// gRPC clients and a mock Redis client (no live services).

const test = require('node:test');
const assert = require('node:assert/strict');
const jwt = require('jsonwebtoken');
const { buildApp } = require('../src/app');
const { GRPC_STATUS_CODE } = require('../src/middleware/grpcCodes');

const SECRET = 'test-secret';

const FEED_METHODS = ['createPost', 'getPost', 'getHomeTimeline', 'deletePost', 'likePost', 'unlikePost', 'addComment', 'getComments', 'search'];
const USER_METHODS = ['register', 'login', 'logout', 'getProfile', 'updateProfile', 'follow', 'unfollow', 'getFollowers', 'getFollowing'];

function stubClient(methods, handlers) {
  const client = {};
  for (const m of methods) client[m] = handlers[m] || (async () => ({}));
  client.close = () => {};
  return client;
}

const allowRedis = {
  incr: async () => 1,
  expire: async () => 1,
  get: async () => null,
};

function makeApp({ userHandlers = {}, redis = allowRedis } = {}) {
  return buildApp({
    logger: require('pino')({ level: 'silent' }),
    redis,
    feedClient: stubClient(FEED_METHODS, {}),
    userClient: stubClient(USER_METHODS, userHandlers),
    jwtSecret: SECRET,
  });
}

function tokenFor(userId, extra = {}) {
  const payload = { sub: String(userId), username: 'carol', displayName: 'Carol', ...extra };
  return jwt.sign(payload, SECRET, { algorithm: 'HS256', expiresIn: 3600 });
}

const authHeader = (token) => ({ authorization: `Bearer ${token}` });

test('POST /api/auth/register is public and forwards to userClient.register', async () => {
  const calls = [];
  const app = makeApp({
    userHandlers: {
      register: async (req) => {
        calls.push(req);
        return { token: 't1', expires_in: 86400, user: { id: 1, username: 'bob' } };
      },
    },
  });
  const res = await app.inject({
    method: 'POST',
    url: '/api/auth/register',
    payload: { username: 'bob', email: 'bob@example.com', password: 'secret', display_name: 'Bob' },
  });
  assert.equal(res.statusCode, 200);
  assert.deepEqual(res.json(), { code: 0, message: 'success', data: { token: 't1', expires_in: 86400, user: { id: 1, username: 'bob' } } });
  assert.deepEqual(calls[0], { username: 'bob', email: 'bob@example.com', password: 'secret', display_name: 'Bob' });
  await app.close();
});

test('POST /api/auth/register validates required fields with 400', async () => {
  const app = makeApp();
  for (const payload of [{}, { username: 'bob' }, { username: 'bob', email: 'a@b.c' }]) {
    const res = await app.inject({ method: 'POST', url: '/api/auth/register', payload });
    assert.equal(res.statusCode, 400, JSON.stringify(payload));
    assert.equal(res.json().code, 400);
  }
  await app.close();
});

test('POST /api/auth/register maps ALREADY_EXISTS to 409', async () => {
  const app = makeApp({
    userHandlers: {
      register: async () => {
        const e = new Error('username taken');
        e.code = GRPC_STATUS_CODE.ALREADY_EXISTS;
        throw e;
      },
    },
  });
  const res = await app.inject({
    method: 'POST',
    url: '/api/auth/register',
    payload: { username: 'bob', email: 'bob@example.com', password: 'secret' },
  });
  assert.equal(res.statusCode, 409);
  assert.deepEqual(res.json(), { code: 409, message: 'already_exists', data: null });
  await app.close();
});

test('POST /api/auth/login with wrong password maps UNAUTHENTICATED to 401', async () => {
  const app = makeApp({
    userHandlers: {
      login: async () => {
        const e = new Error('bad credentials');
        e.code = GRPC_STATUS_CODE.UNAUTHENTICATED;
        throw e;
      },
    },
  });
  const res = await app.inject({ method: 'POST', url: '/api/auth/login', payload: { account: 'bob', password: 'nope' } });
  assert.equal(res.statusCode, 401);
  assert.deepEqual(res.json(), { code: 401, message: 'unauthorized', data: null });
  await app.close();
});

test('POST /api/auth/login is public and forwards to userClient.login', async () => {
  const calls = [];
  const app = makeApp({
    userHandlers: {
      login: async (req) => {
        calls.push(req);
        return { token: 't2', expires_in: 86400, user: { id: 2, username: 'alice' } };
      },
    },
  });
  const res = await app.inject({ method: 'POST', url: '/api/auth/login', payload: { account: 'alice', password: 'pw' } });
  assert.equal(res.statusCode, 200);
  assert.equal(res.json().data.token, 't2');
  assert.deepEqual(calls[0], { account: 'alice', password: 'pw' });
  await app.close();
});

test('POST /api/auth/logout forwards the token jti to userClient.logout', async () => {
  const calls = [];
  const app = makeApp({
    userHandlers: {
      logout: async (req) => {
        calls.push(req);
        return {};
      },
    },
  });
  const res = await app.inject({
    method: 'POST',
    url: '/api/auth/logout',
    headers: authHeader(tokenFor(3, { jti: 'jti-9' })),
  });
  assert.equal(res.statusCode, 200);
  assert.deepEqual(res.json(), { code: 0, message: 'success', data: null });
  assert.deepEqual(calls[0], { jti: 'jti-9' });
  await app.close();
});

test('logged-out token is rejected by the blacklist (401)', async () => {
  const blacklistRedis = {
    incr: async () => 1,
    expire: async () => 1,
    get: async (key) => (key === 'auth:blacklist:jti-dead' ? '1' : null),
  };
  const app = makeApp({ redis: blacklistRedis });
  const res = await app.inject({
    method: 'GET',
    url: '/api/feed/home',
    headers: authHeader(tokenFor(3, { jti: 'jti-dead' })),
  });
  assert.equal(res.statusCode, 401);
  assert.deepEqual(res.json(), { code: 401, message: 'unauthorized', data: null });
  await app.close();
});

test('protected route without token -> 401 envelope', async () => {
  const app = makeApp();
  for (const [method, url] of [
    ['GET', '/api/user/1'],
    ['PUT', '/api/user/profile'],
    ['POST', '/api/user/1/follow'],
    ['GET', '/api/user/1/followers'],
    ['GET', '/api/feed/home'],
  ]) {
    const res = await app.inject({ method, url });
    assert.equal(res.statusCode, 401, `${method} ${url}`);
    assert.deepEqual(res.json(), { code: 401, message: 'unauthorized', data: null });
  }
  await app.close();
});

test('GET /api/user/:id forwards to getProfile with the path id', async () => {
  const calls = [];
  const app = makeApp({
    userHandlers: {
      getProfile: async (req) => {
        calls.push(req);
        return { user: { id: 7, username: 'dave', display_name: 'Dave' } };
      },
    },
  });
  const res = await app.inject({ method: 'GET', url: '/api/user/7', headers: authHeader(tokenFor(3)) });
  assert.equal(res.statusCode, 200);
  assert.equal(res.json().data.user.id, 7);
  assert.deepEqual(calls[0], { user_id: 7 });
  await app.close();
});

test('PUT /api/user/profile forwards the current user id from the token', async () => {
  const calls = [];
  const app = makeApp({
    userHandlers: {
      updateProfile: async (req) => {
        calls.push(req);
        return { user: { id: 4, display_name: 'New' } };
      },
    },
  });
  const res = await app.inject({
    method: 'PUT',
    url: '/api/user/profile',
    headers: authHeader(tokenFor(4)),
    payload: { display_name: 'New', bio: 'hi', avatar_url: 'http://a' },
  });
  assert.equal(res.statusCode, 200);
  assert.deepEqual(calls[0], { user_id: 4, display_name: 'New', bio: 'hi', avatar_url: 'http://a' });
  await app.close();
});

test('follow/unfollow use token id as follower_id and path id as followee_id', async () => {
  const calls = [];
  const app = makeApp({
    userHandlers: {
      follow: async (req) => { calls.push(['follow', req]); return {}; },
      unfollow: async (req) => { calls.push(['unfollow', req]); return {}; },
    },
  });
  const auth = authHeader(tokenFor(3));
  let res = await app.inject({ method: 'POST', url: '/api/user/9/follow', headers: auth });
  assert.equal(res.statusCode, 200);
  assert.deepEqual(res.json(), { code: 0, message: 'success', data: null });
  res = await app.inject({ method: 'DELETE', url: '/api/user/9/follow', headers: auth });
  assert.equal(res.statusCode, 200);
  assert.deepEqual(calls, [
    ['follow', { follower_id: 3, followee_id: 9 }],
    ['unfollow', { follower_id: 3, followee_id: 9 }],
  ]);
  await app.close();
});

test('followers/following forward cursor and limit page params', async () => {
  const calls = [];
  const app = makeApp({
    userHandlers: {
      getFollowers: async (req) => { calls.push(['followers', req]); return { users: [], next_cursor: 0, has_more: false }; },
      getFollowing: async (req) => { calls.push(['following', req]); return { users: [], next_cursor: 0, has_more: false }; },
    },
  });
  const auth = authHeader(tokenFor(3));
  let res = await app.inject({ method: 'GET', url: '/api/user/5/followers?cursor=10&limit=25', headers: auth });
  assert.equal(res.statusCode, 200);
  res = await app.inject({ method: 'GET', url: '/api/user/5/following', headers: auth });
  assert.equal(res.statusCode, 200);
  assert.deepEqual(calls, [
    ['followers', { user_id: 5, cursor: 10, limit: 25 }],
    ['following', { user_id: 5, cursor: 0, limit: 20 }],
  ]);
  await app.close();
});

test('user routes reject invalid :id with 400', async () => {
  const app = makeApp();
  const auth = authHeader(tokenFor(1));
  for (const url of ['/api/user/0', '/api/user/-1', '/api/user/abc']) {
    const res = await app.inject({ method: 'GET', url, headers: auth });
    assert.equal(res.statusCode, 400, url);
    assert.equal(res.json().code, 400);
  }
  await app.close();
});
