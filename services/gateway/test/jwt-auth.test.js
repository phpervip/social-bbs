'use strict';

// JWT round-trip through the auth middleware: valid token accepted, wrong
// secret rejected, expired token rejected, blacklisted jti rejected,
// public routes bypass auth.

const test = require('node:test');
const assert = require('node:assert/strict');
const jwt = require('jsonwebtoken');
const { createAuthMiddleware, parseBearer, isPublic } = require('../src/middleware/auth');

const SECRET = 'test-secret';

// Redis stub with no blacklist entries — every check returns null (allowed).
const noBlacklist = { get: async () => null };

function callAuth(middleware, url, headers = {}) {
  const reply = { status: (s) => ({ send: (body) => ({ status: s, body }) }) };
  const request = { url, headers, log: { warn() {} } };
  return middleware(request, reply);
}

function signToken(payload, secret = SECRET) {
  return jwt.sign(payload, secret, { algorithm: 'HS256', expiresIn: 3600 });
}

test('isPublic matches auth endpoints and /healthz only', () => {
  assert.equal(isPublic('/api/auth/register'), true);
  assert.equal(isPublic('/api/auth/login'), true);
  assert.equal(isPublic('/healthz'), true);
  assert.equal(isPublic('/healthz?x=1'), true);
  assert.equal(isPublic('/api/auth/logout'), false);
  assert.equal(isPublic('/api/feed/home'), false);
  assert.equal(isPublic('/api/user/1'), false);
  assert.equal(isPublic('/api/dev/login'), false); // dev routes removed in P2
  assert.equal(isPublic('/api/devil'), false); // prefix, not path segment
});

test('parseBearer extracts token from Authorization header', () => {
  assert.deepEqual(parseBearer('Bearer abc.def.ghi'), { token: 'abc.def.ghi' });
  assert.equal(parseBearer('Basic abc'), null);
  assert.equal(parseBearer(undefined), null);
  assert.equal(parseBearer('Bearer  '), null);
});

test('valid JWT is accepted and request.user is injected', async () => {
  const token = signToken({ sub: '3', username: 'carol', displayName: 'Carol摄影师', jti: 'jti-1', iat: Math.floor(Date.now() / 1000) });
  const middleware = createAuthMiddleware({ jwtSecret: SECRET, redis: noBlacklist });

  const request = { url: '/api/feed/home', headers: { authorization: `Bearer ${token}` }, log: { warn() {} } };
  const reply = { status: (s) => ({ send: (body) => ({ status: s, body }) }) };
  const result = await middleware(request, reply);

  assert.equal(result, undefined); // no response -> passed
  assert.equal(request.user.id, 3);
  assert.equal(request.user.username, 'carol');
  assert.equal(request.user.displayName, 'Carol摄影师');
  assert.equal(request.user.jti, 'jti-1');
});

test('token signed with wrong secret is rejected with 401', async () => {
  const token = jwt.sign({ sub: '1' }, 'other-secret', { algorithm: 'HS256', expiresIn: 3600 });
  const middleware = createAuthMiddleware({ jwtSecret: SECRET, redis: noBlacklist });
  const result = await callAuth(middleware, '/api/feed/home', { authorization: `Bearer ${token}` });
  assert.equal(result.status, 401);
  assert.deepEqual(result.body, { code: 401, message: 'unauthorized', data: null });
});

test('expired token is rejected with 401', async () => {
  const token = jwt.sign({ sub: '1' }, SECRET, { algorithm: 'HS256', expiresIn: -10 }); // already expired
  const middleware = createAuthMiddleware({ jwtSecret: SECRET, redis: noBlacklist });
  const result = await callAuth(middleware, '/api/feed/home', { authorization: `Bearer ${token}` });
  assert.equal(result.status, 401);
  assert.deepEqual(result.body, { code: 401, message: 'unauthorized', data: null });
});

test('missing or malformed Authorization header is rejected with 401', async () => {
  const middleware = createAuthMiddleware({ jwtSecret: SECRET, redis: noBlacklist });
  const missing = await callAuth(middleware, '/api/feed/home', {});
  assert.equal(missing.status, 401);
  const malformed = await callAuth(middleware, '/api/feed/home', { authorization: 'no scheme here' });
  assert.equal(malformed.status, 401);
});

test('token with non-numeric sub is rejected', async () => {
  const token = signToken({ sub: 'abc' });
  const middleware = createAuthMiddleware({ jwtSecret: SECRET, redis: noBlacklist });
  const result = await callAuth(middleware, '/api/feed/home', { authorization: `Bearer ${token}` });
  assert.equal(result.status, 401);
});

test('blacklisted jti is rejected with 401', async () => {
  const token = signToken({ sub: '3', jti: 'jti-logged-out' });
  const redis = { get: async (key) => (key === 'auth:blacklist:jti-logged-out' ? '1' : null) };
  const middleware = createAuthMiddleware({ jwtSecret: SECRET, redis });
  const result = await callAuth(middleware, '/api/feed/home', { authorization: `Bearer ${token}` });
  assert.equal(result.status, 401);
  assert.deepEqual(result.body, { code: 401, message: 'unauthorized', data: null });
});

test('token whose jti is NOT blacklisted is allowed', async () => {
  const token = signToken({ sub: '3', jti: 'jti-ok' });
  const redis = { get: async (key) => (key === 'auth:blacklist:jti-ok' ? null : '1') };
  const middleware = createAuthMiddleware({ jwtSecret: SECRET, redis });
  const result = await callAuth(middleware, '/api/feed/home', { authorization: `Bearer ${token}` });
  assert.equal(result, undefined); // allowed
});

test('blacklist check degrades open when Redis is unavailable', async () => {
  const token = signToken({ sub: '3', jti: 'jti-1' });
  const redis = { get: async () => { throw new Error('ECONNREFUSED'); } };
  const middleware = createAuthMiddleware({ jwtSecret: SECRET, redis });
  const result = await callAuth(middleware, '/api/feed/home', { authorization: `Bearer ${token}` });
  assert.equal(result, undefined); // allowed despite blacklist outage
});

test('public routes bypass auth entirely', async () => {
  const middleware = createAuthMiddleware({ jwtSecret: SECRET, redis: noBlacklist });
  const result = await callAuth(middleware, '/api/auth/register', {});
  assert.equal(result, undefined); // no 401 reply -> bypassed
});
