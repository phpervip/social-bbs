'use strict';

// gRPC client factory for the FeedService (proto: src/proto/feed.proto).
// Lazy connect: the channel is created on first use (so the gateway boots
// even when the Feed Service is down) and every call waits for the channel
// to become ready with retry (waitForReady), so transient outages recover
// without a gateway restart.

const path = require('path');
const grpc = require('@grpc/grpc-js');
const protoLoader = require('@grpc/proto-loader');
const { runWithBreaker } = require('../middleware/breaker');
const { GRPC_STATUS_CODE } = require('../middleware/grpcCodes');

const PROTO_PATH = path.join(__dirname, '..', 'proto', 'feed.proto');

const PROTO_OPTIONS = {
  keepCase: true,
  longs: Number, // int64 fields arrive as JS numbers (safe for P1 timestamps)
  enums: String,
  defaults: true,
  oneofs: true,
};

// deadline must be computed per call (not at module load) — a module-level
// constant would expire while the gateway is running and every first gRPC
// call would fail with DEADLINE_EXCEEDED ("waiting for name resolution").
const DEFAULT_CALL_OPTIONS = {
  // waitForReady retries while the server is unavailable; deadline caps one attempt.
  deadline: () => Date.now() + 5000,
  waitForReady: true,
};

// When the downstream is unreachable, waitForReady gives up with
// DEADLINE_EXCEEDED ("waiting for metadata filters" / connectivity) — that
// is a service-unavailable condition, not a deadline the caller cares about.
function normalizeConnectivityError(err) {
  if (err && err.code === GRPC_STATUS_CODE.DEADLINE_EXCEEDED) {
    const normalized = new Error(err.details || 'feed service unavailable');
    normalized.code = GRPC_STATUS_CODE.UNAVAILABLE;
    return normalized;
  }
  return err;
}

function loadDefinition() {
  const packageDefinition = protoLoader.loadSync(PROTO_PATH, PROTO_OPTIONS);
  const loaded = grpc.loadPackageDefinition(packageDefinition);
  return loaded.feed.v1.FeedService;
}

// grpcService is injectable for tests (a stub exposing the same method names).
function createFeedClient({ address = 'localhost:9000', grpcService = loadDefinition(), breaker, logger } = {}) {
  let channel = null;
  let client = null;

  function getClient() {
    if (client) return client;
    channel = new grpc.Client(address, grpc.ChannelCredentials.createInsecure());
    client = new grpcService(address, grpc.ChannelCredentials.createInsecure());
    return client;
  }

  // Wraps a client method into a promise + breaker + waitForReady.
  function call(methodName) {
    return (req, callOptions = {}) => {
      const invoke = () =>
        new Promise((resolve, reject) => {
          const c = getClient();
          const deadline = callOptions.deadline || DEFAULT_CALL_OPTIONS.deadline();
          const waitForReady = callOptions.waitForReady !== undefined ? callOptions.waitForReady : DEFAULT_CALL_OPTIONS.waitForReady;
          const handler = (err, response) => {
            if (err) {
              reject(normalizeConnectivityError(err));
            } else {
              resolve(response);
            }
          };
          if (waitForReady) {
            c.waitForReady(deadline, () => c[methodName](req, { deadline }, handler));
          } else {
            c[methodName](req, { deadline }, handler);
          }
        });

      return breaker ? runWithBreaker(breaker, invoke) : invoke();
    };
  }

  const METHODS = [
    'createPost',
    'getPost',
    'getHomeTimeline',
    'deletePost',
    'likePost',
    'unlikePost',
    'addComment',
    'getComments',
    'search',
  ];

  const api = {};
  for (const m of METHODS) {
    api[m] = call(m);
  }

  api.close = () => {
    if (channel) channel.close();
    channel = null;
    client = null;
  };

  api._getClient = getClient; // test hook
  return api;
}

module.exports = { createFeedClient, loadDefinition, PROTO_PATH };
