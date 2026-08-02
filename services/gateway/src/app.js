'use strict';

// Assembles the full middleware chain + routes into a Fastify instance.
// Layer order (design contract): pre -> auth -> rate -> breaker/forward -> post.
// Exported as buildApp(deps) so tests can inject mocks for every dependency.

const Fastify = require('fastify');
const cors = require('@fastify/cors');
const pino = require('pino');

const config = require('./config');
const { createPreMiddleware } = require('./middleware/pre');
const { createAuthMiddleware } = require('./middleware/auth');
const { createRateMiddleware } = require('./middleware/rate');
const { createBreaker } = require('./middleware/breaker');
const { createFeedClient } = require('./grpc/feed');
const { createUserClient } = require('./grpc/user');
const { getVideoClient } = require('./grpc/video');
const { createAuthRoutes } = require('./routes/auth');
const { createUserRoutes } = require('./routes/user');
const { createFeedRoutes } = require('./routes/feed');
const { createVideoRoutes } = require('./routes/video');

function buildApp({
  logger = pino({ level: process.env.LOG_LEVEL || 'info' }),
  redis = null, // Redis-like client (incr/expire/get); real one created lazily by default
  feedClient = null, // injectable gRPC stub for tests
  userClient = null, // injectable gRPC stub for tests
  breaker = createBreaker(config.breaker),
  jwtSecret = config.jwtSecret,
} = {}) {
  // Fastify 5 accepts a pino *instance* via loggerInstance; a plain config
  // object (or true) goes to the logger option.
  const isPinoInstance = logger && typeof logger.child === 'function';
  const app = Fastify({
    ...(isPinoInstance ? { loggerInstance: logger } : { logger }),
    bodyLimit: config.bodyLimitBytes,
    // request logging is handled by pre middleware (with the real request id)
    logController: new Fastify.LogController({ disableRequestLogging: true }),
    genReqId: () => 'pending', // real request id is assigned by pre middleware
  });

  // --- Layer 1: pre (request-id, logger, CORS, body limit is fastify-level) ---
  app.addHook('onRequest', createPreMiddleware({ logger, corsOrigins: config.corsOrigins }));
  app.register(cors, {
    origin: config.corsOrigins,
    methods: ['GET', 'POST', 'PUT', 'DELETE', 'OPTIONS'],
    allowedHeaders: ['Authorization', 'Content-Type'],
  });

  // --- Layer 2: auth (JWT verify + blacklist check via redis) ---
  app.addHook('onRequest', createAuthMiddleware({ jwtSecret, redis }));

  // --- Layer 3: rate limiting ---
  app.addHook('onRequest', createRateMiddleware({ client: redis }));

  // --- Layer 4/5: routes + envelope (forwarding through breaker-wrapped clients) ---
  const feed = feedClient || createFeedClient({ address: config.feedAddr, breaker });
  const user = userClient || createUserClient({ address: config.userAddr, breaker });
  const video = getVideoClient({ address: config.videoAddr, breaker });

  app.register(createAuthRoutes, { userClient: user });
  app.register(createUserRoutes, { userClient: user });
  app.register(createFeedRoutes, { feedClient: feed });
  app.register(createVideoRoutes, { videoClient: video });

  // Healthz — no auth, no rate limit (exempt in both middlewares).
  app.get('/healthz', async (_request, reply) => reply.send({ code: 0, message: 'ok', data: null }));

  // Unknown routes -> 404 in envelope shape.
  app.setNotFoundHandler((request, reply) =>
    reply.status(404).send({ code: 404, message: 'not_found', data: null })
  );

  app.decorate('feedClient', feed);
  app.decorate('userClient', user);
  app.decorate('videoClient', video);
  return app;
}

module.exports = { buildApp };
