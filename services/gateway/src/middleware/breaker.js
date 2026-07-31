'use strict';

// Layer 4 — per-downstream circuit breaker: closed / open / half-open.
// closed:      normal operation; N consecutive failures -> open
// open:        reject all calls for openMs (30s) -> half-open
// half-open:   allow exactly 1 probe; success -> closed, failure -> open

const { GRPC_STATUS_CODE } = require('./grpcCodes');

const STATE = Object.freeze({ CLOSED: 'closed', OPEN: 'open', HALF_OPEN: 'half-open' });

function createBreaker({ failureThreshold = 5, openMs = 30 * 1000, now = () => Date.now() } = {}) {
  let state = STATE.CLOSED;
  let consecutiveFailures = 0;
  let openedAt = 0;
  let probing = false;

  return {
    get state() {
      return state;
    },
    get consecutiveFailures() {
      return consecutiveFailures;
    },
    // Returns true when a call may be attempted.
    allow() {
      const t = now();
      if (state === STATE.OPEN && t - openedAt >= openMs) {
        state = STATE.HALF_OPEN;
        probing = true; // exactly one probe allowed
      }
      if (state === STATE.CLOSED) return true;
      if (state === STATE.HALF_OPEN) {
        if (!probing) return false;
        probing = false; // consume the single probe
        return true;
      }
      return false;
    },
    onSuccess() {
      if (state === STATE.HALF_OPEN) probing = false;
      state = STATE.CLOSED;
      consecutiveFailures = 0;
    },
    onFailure() {
      consecutiveFailures += 1;
      if (state === STATE.HALF_OPEN) {
        probing = false;
        state = STATE.OPEN;
        openedAt = now();
        return;
      }
      if (consecutiveFailures >= failureThreshold) {
        state = STATE.OPEN;
        openedAt = now();
      }
    },
    // Force-reset for tests.
    reset() {
      state = STATE.CLOSED;
      consecutiveFailures = 0;
      openedAt = 0;
      probing = false;
    },
  };
}

// Wraps an async RPC call with the breaker; when the breaker is open the
// call is rejected with a gRPC-style UNAVAILABLE error so post.js maps it.
async function runWithBreaker(breaker, call) {
  if (!breaker.allow()) {
    const err = new Error('circuit breaker open');
    err.code = GRPC_STATUS_CODE.UNAVAILABLE;
    err.breakerOpen = true;
    throw err;
  }
  try {
    const result = await call();
    breaker.onSuccess();
    return result;
  } catch (err) {
    breaker.onFailure();
    throw err;
  }
}

module.exports = { createBreaker, runWithBreaker, STATE };
