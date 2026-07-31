'use strict';

// gRPC client factory: method dispatch through waitForReady, and
// connectivity-deadline normalization to UNAVAILABLE (so a down Feed
// Service surfaces as 503, not 500). Uses a stub service — no real server.

const test = require('node:test');
const assert = require('node:assert/strict');
const { createFeedClient } = require('../src/grpc/feed');
const { GRPC_STATUS_CODE } = require('../src/middleware/grpcCodes');

// Stub grpcService: constructor records calls; each method either resolves
// with a canned response or invokes the callback with a connectivity error.
function stubService({ respondWith = 'ok' }) {
  function ServiceStub(address) {
    ServiceStub.instances.push({ address });
  }
  ServiceStub.instances = [];
  ServiceStub.prototype.waitForReady = function waitForReady(_deadline, cb) {
    cb(); // channel "ready" immediately
  };
  for (const m of ['createPost', 'getPost', 'getHomeTimeline', 'deletePost', 'likePost', 'unlikePost', 'addComment', 'getComments', 'search']) {
    ServiceStub.prototype[m] = function (req, _opts, cb) {
      if (respondWith === 'deadline') {
        const err = new Error('Deadline exceeded after 0.004s, waiting for metadata filters');
        err.code = GRPC_STATUS_CODE.DEADLINE_EXCEEDED;
        return cb(err);
      }
      cb(null, { ok: true, ...req });
    };
  }
  return ServiceStub;
}

test('createFeedClient exposes all 9 FeedService methods and forwards requests', async () => {
  const ServiceStub = stubService({ respondWith: 'ok' });
  const client = createFeedClient({ address: 'stub:9000', grpcService: ServiceStub });

  assert.equal(ServiceStub.instances.length, 0, 'channel must be lazy (no connect before first call)');

  const res = await client.createPost({ user_id: 1, content: 'hi' });
  assert.equal(res.user_id, 1);
  assert.equal(res.content, 'hi');
  assert.equal(ServiceStub.instances.length, 1, 'connect happens on first call only');

  const search = await client.search({ query: 'x', user_id: 2, page: { cursor: 0, limit: 20 } });
  assert.equal(search.query, 'x');

  for (const m of ['getPost', 'getHomeTimeline', 'deletePost', 'likePost', 'unlikePost', 'addComment', 'getComments']) {
    const r = await client[m]({});
    assert.equal(r.ok, true, `${m} resolves`);
  }
});

test('waitForReady connectivity deadline is normalized to UNAVAILABLE (503 path)', async () => {
  const ServiceStub = stubService({ respondWith: 'deadline' });
  const client = createFeedClient({ address: 'stub:9000', grpcService: ServiceStub });

  await assert.rejects(client.getHomeTimeline({}), (err) => {
    assert.equal(err.code, GRPC_STATUS_CODE.UNAVAILABLE);
    return true;
  });
});

test('breaker-wrapped client trips after 5 downstream failures', async () => {
  const { createBreaker } = require('../src/middleware/breaker');
  const ServiceStub = stubService({ respondWith: 'deadline' });
  const breaker = createBreaker({ failureThreshold: 5, openMs: 30000 });
  const client = createFeedClient({ address: 'stub:9000', grpcService: ServiceStub, breaker });

  for (let i = 0; i < 5; i++) {
    await assert.rejects(client.likePost({}));
  }
  assert.equal(breaker.state, 'open');

  // Breaker rejects even a would-be-successful call while open.
  const okService = stubService({ respondWith: 'ok' });
  const okClient = createFeedClient({ address: 'stub:9000', grpcService: okService, breaker });
  await assert.rejects(okClient.getPost({}), (err) => {
    assert.equal(err.code, GRPC_STATUS_CODE.UNAVAILABLE);
    assert.equal(err.breakerOpen, true);
    return true;
  });
});
