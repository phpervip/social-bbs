'use strict';

// Circuit breaker state machine: closed -> open (after N consecutive
// failures) -> half-open (after openMs) -> closed on probe success,
// open again on probe failure. Also verifies breaker-wrapped gRPC calls
// surface as UNAVAILABLE (503) when the breaker is open.

const test = require('node:test');
const assert = require('node:assert/strict');
const { createBreaker, runWithBreaker, STATE } = require('../src/middleware/breaker');
const { GRPC_STATUS_CODE } = require('../src/middleware/grpcCodes');

test('starts closed and allows calls', () => {
  const b = createBreaker({ failureThreshold: 5, openMs: 1000 });
  assert.equal(b.state, STATE.CLOSED);
  assert.equal(b.allow(), true);
});

test('5 consecutive failures -> open; calls rejected while open', () => {
  const b = createBreaker({ failureThreshold: 5, openMs: 30000 });
  for (let i = 0; i < 5; i++) b.onFailure();
  assert.equal(b.state, STATE.OPEN);
  assert.equal(b.allow(), false);
});

test('partial failures do not open the breaker', () => {
  const b = createBreaker({ failureThreshold: 5, openMs: 30000 });
  b.onFailure();
  b.onSuccess();
  b.onFailure();
  assert.equal(b.state, STATE.CLOSED);
  assert.equal(b.allow(), true);
});

test('after openMs -> half-open allows exactly one probe', () => {
  let t = 0;
  const b = createBreaker({ failureThreshold: 5, openMs: 1000, now: () => t });
  for (let i = 0; i < 5; i++) b.onFailure();
  assert.equal(b.state, STATE.OPEN);
  assert.equal(b.allow(), false); // still within open window
  t = 1001; // window elapsed
  assert.equal(b.allow(), true); // probe allowed
  assert.equal(b.state, STATE.HALF_OPEN);
  assert.equal(b.allow(), false); // only one probe
});

test('probe success -> closed; probe failure -> open again', () => {
  let t = 0;
  const b = createBreaker({ failureThreshold: 5, openMs: 1000, now: () => t });
  for (let i = 0; i < 5; i++) b.onFailure();
  t = 1001;
  b.allow(); // -> half-open, probe taken
  b.onSuccess();
  assert.equal(b.state, STATE.CLOSED);
  assert.equal(b.consecutiveFailures, 0);

  // failure path
  for (let i = 0; i < 5; i++) b.onFailure();
  t = 2001;
  b.allow();
  b.onFailure();
  assert.equal(b.state, STATE.OPEN);
});

test('runWithBreaker rejects with UNAVAILABLE error when breaker is open', async () => {
  let t = 0;
  const b = createBreaker({ failureThreshold: 5, openMs: 30000, now: () => t });
  const call = async () => 'ok';

  for (let i = 0; i < 5; i++) {
    const failing = () => Promise.reject(Object.assign(new Error('down'), { code: GRPC_STATUS_CODE.UNAVAILABLE }));
    await assert.rejects(runWithBreaker(b, failing));
  }
  assert.equal(b.state, STATE.OPEN);

  await assert.rejects(runWithBreaker(b, call), (err) => {
    assert.equal(err.code, GRPC_STATUS_CODE.UNAVAILABLE);
    assert.equal(err.breakerOpen, true);
    return true;
  });
});

test('runWithBreaker records success and closes the breaker', async () => {
  let t = 0;
  const b = createBreaker({ failureThreshold: 5, openMs: 1000, now: () => t });
  for (let i = 0; i < 5; i++) b.onFailure();
  assert.equal(b.state, STATE.OPEN);
  t = 1001; // open window elapsed -> runWithBreaker's allow() grants the probe
  const result = await runWithBreaker(b, async () => ({ ok: true }));
  assert.deepEqual(result, { ok: true });
  assert.equal(b.state, STATE.CLOSED);
});
