'use strict';

// Layer 5 — post middleware: unified response envelope + error mapping.
// Success:  { code: 0, message: "success", data }
// Errors:   gRPC status -> HTTP status + envelope code (see map below).
// Local validation errors (from routes) use code 400 / "invalid_argument".

const { GRPC_STATUS_CODE } = require('./grpcCodes');

const GRPC_TO_HTTP = Object.freeze({
  [GRPC_STATUS_CODE.INVALID_ARGUMENT]: { http: 400, code: 400, message: 'invalid_argument' },
  [GRPC_STATUS_CODE.UNAUTHENTICATED]: { http: 401, code: 401, message: 'unauthorized' },
  [GRPC_STATUS_CODE.PERMISSION_DENIED]: { http: 403, code: 403, message: 'permission_denied' },
  [GRPC_STATUS_CODE.NOT_FOUND]: { http: 404, code: 404, message: 'not_found' },
  [GRPC_STATUS_CODE.ALREADY_EXISTS]: { http: 409, code: 409, message: 'already_exists' },
  [GRPC_STATUS_CODE.INTERNAL]: { http: 500, code: 500, message: 'internal_error' },
  [GRPC_STATUS_CODE.UNAVAILABLE]: { http: 503, code: 503, message: 'service_unavailable' },
  // A deadline hit while the downstream is unreachable is an availability
  // problem, not a client timeout the gateway should surface as 500.
  [GRPC_STATUS_CODE.DEADLINE_EXCEEDED]: { http: 503, code: 503, message: 'service_unavailable' },
});

const DEFAULT_ERROR = { http: 500, code: 500, message: 'internal_error' };

// Pure function — export for tests.
function mapError(err) {
  const code = err && typeof err.code === 'number' ? err.code : undefined;
  return GRPC_TO_HTTP[code] || DEFAULT_ERROR;
}

function ok(reply, data) {
  return reply.send({ code: 0, message: 'success', data });
}

function sendError(reply, err) {
  const mapped = mapError(err);
  if (mapped.http >= 500 && err?.stack) {
    // Log server-side errors with detail; clients only see the envelope.
    reply.request?.log?.error({ err }, 'request failed');
  }
  return reply.status(mapped.http).send({ code: mapped.code, message: mapped.message, data: null });
}

// Helper for routes validating their own input (bad query/body).
function badRequest(reply, message) {
  return reply.status(400).send({ code: 400, message: message || 'invalid_argument', data: null });
}

module.exports = { ok, sendError, badRequest, mapError, GRPC_TO_HTTP };
