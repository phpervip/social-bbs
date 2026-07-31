'use strict';

// gRPC status codes used by @grpc/grpc-js — kept as a local constant table
// so middleware and post.js share one source of truth without importing grpc.

const GRPC_STATUS_CODE = Object.freeze({
  OK: 0,
  INVALID_ARGUMENT: 3,
  DEADLINE_EXCEEDED: 4,
  NOT_FOUND: 5,
  ALREADY_EXISTS: 6,
  PERMISSION_DENIED: 7,
  INTERNAL: 13,
  UNAVAILABLE: 14,
  UNAUTHENTICATED: 16,
});

module.exports = { GRPC_STATUS_CODE };
