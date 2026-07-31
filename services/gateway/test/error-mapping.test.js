'use strict';

// Error mapping table: gRPC status codes -> HTTP status + envelope shape.
// Tests the pure mapError function from post.js.

const test = require('node:test');
const assert = require('node:assert/strict');
const { mapError, GRPC_TO_HTTP } = require('../src/middleware/post');
const { GRPC_STATUS_CODE } = require('../src/middleware/grpcCodes');

const CASES = [
  [GRPC_STATUS_CODE.INVALID_ARGUMENT, 400, 'invalid_argument'],
  [GRPC_STATUS_CODE.UNAUTHENTICATED, 401, 'unauthorized'],
  [GRPC_STATUS_CODE.PERMISSION_DENIED, 403, 'permission_denied'],
  [GRPC_STATUS_CODE.NOT_FOUND, 404, 'not_found'],
  [GRPC_STATUS_CODE.ALREADY_EXISTS, 409, 'already_exists'],
  [GRPC_STATUS_CODE.INTERNAL, 500, 'internal_error'],
  [GRPC_STATUS_CODE.UNAVAILABLE, 503, 'service_unavailable'],
  [GRPC_STATUS_CODE.DEADLINE_EXCEEDED, 503, 'service_unavailable'],
];

test('maps every contract gRPC code to its HTTP status + envelope code', () => {
  for (const [grpcCode, http, envelopeCode] of CASES) {
    const mapped = mapError({ code: grpcCode });
    assert.equal(mapped.http, http, `grpc ${grpcCode} -> http ${http}`);
    assert.equal(mapped.code, http, `grpc ${grpcCode} -> envelope code ${http}`);
    assert.equal(mapped.message, envelopeCode, `grpc ${grpcCode} -> envelope message ${envelopeCode}`);
  }
});

test('unknown / missing error codes fall back to 500 internal_error', () => {
  assert.deepEqual(mapError({ message: 'boom' }), { http: 500, code: 500, message: 'internal_error' });
  assert.deepEqual(mapError(new Error('boom')), { http: 500, code: 500, message: 'internal_error' });
  assert.deepEqual(mapError(undefined), { http: 500, code: 500, message: 'internal_error' });
  assert.deepEqual(mapError({ code: 9999 }), { http: 500, code: 500, message: 'internal_error' });
});

test('mapping table covers exactly the codes present in GRPC_TO_HTTP', () => {
  assert.equal(CASES.length, Object.keys(GRPC_TO_HTTP).length);
});
