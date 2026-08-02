'use strict';

// gRPC client factory for the VideoService (proto: src/proto/video.proto).
// Mirrors grpc/feed.js: lazy connect (gateway boots even when the Video
// Service is down), waitForReady retry per call, breaker-wrapped dispatch,
// and DEADLINE_EXCEEDED connectivity errors normalized to UNAVAILABLE.
// R3: calls made on behalf of an authenticated user also carry the
// x-user-id / x-user-name identity metadata headers.

const path = require('path');
const grpc = require('@grpc/grpc-js');
const protoLoader = require('@grpc/proto-loader');
const { runWithBreaker } = require('../middleware/breaker');
const { GRPC_STATUS_CODE } = require('../middleware/grpcCodes');
const config = require('../config');

const PROTO_PATH = path.join(__dirname, '..', 'proto', 'video.proto');

const PROTO_OPTIONS = {
  keepCase: true,
  longs: Number, // int64 fields arrive as JS numbers (safe for P3 ids/timestamps)
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
    const normalized = new Error(err.details || 'video service unavailable');
    normalized.code = GRPC_STATUS_CODE.UNAVAILABLE;
    return normalized;
  }
  return err;
}

function loadDefinition() {
  const packageDefinition = protoLoader.loadSync(PROTO_PATH, PROTO_OPTIONS);
  const loaded = grpc.loadPackageDefinition(packageDefinition);
  return loaded.video.v1.VideoService;
}

// grpcService is injectable for tests (a stub exposing the same method names).
function createVideoClient({ address = 'localhost:9002', grpcService = loadDefinition(), breaker, logger } = {}) {
  let channel = null;
  let client = null;

  function getClient() {
    if (client) return client;
    channel = new grpc.Client(address, grpc.ChannelCredentials.createInsecure());
    client = new grpcService(address, grpc.ChannelCredentials.createInsecure());
    return client;
  }

  // Wraps a client method into a promise + breaker + waitForReady.
  // callOptions may carry { user } to inject R3 identity metadata.
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
          // grpc-js unary API: client.method(request, metadata, options, callback).
          // Metadata must be a SEPARATE argument — not nested inside options.
          const opts = { deadline };
          let md;
          if (callOptions.user && callOptions.user.id != null) {
            md = new grpc.Metadata();
            md.set('x-user-id', String(callOptions.user.id));
            md.set('x-user-name', callOptions.user.username || '');
          }
          function doCall() {
            if (md) {
              c[methodName](req, md, opts, handler);
            } else {
              c[methodName](req, opts, handler);
            }
          }
          if (waitForReady) {
            c.waitForReady(deadline, doCall);
          } else {
            doCall();
          }
        });

      return breaker ? runWithBreaker(breaker, invoke) : invoke();
    };
  }

  const METHODS = [
    'initUpload',
    'uploadChunk',
    'completeUpload',
    'getVideo',
    'getPlaybackURL',
    'getTranscodeStatus',
    'deleteVideo',
    'listUserVideos',
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

// Singleton lazy client for the running gateway. The channel is created on
// first use (so the gateway boots even when the Video Service is down).
let singleton = null;

function getVideoClient({ address = config.videoAddr, breaker, logger } = {}) {
  if (!singleton) {
    singleton = createVideoClient({ address, breaker, logger });
  }
  return singleton;
}

module.exports = { createVideoClient, getVideoClient, loadDefinition, PROTO_PATH };