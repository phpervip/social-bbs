'use strict';

// Central environment configuration for the API Gateway.
// All env vars are optional; dev defaults are the P1 contract values.

function intEnv(name, fallback) {
  const raw = process.env[name];
  if (raw === undefined || raw === '') return fallback;
  const n = Number.parseInt(raw, 10);
  return Number.isNaN(n) ? fallback : n;
}

function strEnv(name, fallback) {
  const raw = process.env[name];
  return raw === undefined || raw === '' ? fallback : raw;
}

const config = {
  port: intEnv('GW_PORT', 8080),
  host: strEnv('GW_HOST', '0.0.0.0'),
  feedAddr: strEnv('GW_FEED_ADDR', 'localhost:9000'),
  userAddr: strEnv('GW_USER_ADDR', 'localhost:9001'),
  videoAddr: strEnv('GW_VIDEO_ADDR', 'localhost:9002'),
  jwtSecret: strEnv('JWT_SECRET', 'dev-secret'),
  jwtTtlSeconds: intEnv('JWT_TTL', 24 * 60 * 60), // 24h, matches User Service JWT TTL
  redisAddr: strEnv('GW_REDIS_ADDR', 'localhost:6379'),
  redisPassword: strEnv('GW_REDIS_PASSWORD', ''),
  corsOrigins: ['http://localhost:3000', 'http://localhost:3001', 'http://localhost:3002'],
  bodyLimitBytes: 1024 * 1024, // 1MB
  rate: {
    anonPerMinute: 30,
    authedPerMinute: 100,
    publishPerMinute: 10,
  },
  breaker: {
    failureThreshold: 5,
    openMs: 30 * 1000, // 30s
  },
};

module.exports = config;
