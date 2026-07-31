'use strict';

// Tiny Redis abstraction: lazy ioredis client exposing only what the rate
// middleware needs (incr, expire). Lazy creation + graceful error handling
// means the gateway boots and serves traffic even when Redis is down.
// Tests inject a mock client directly into createRateMiddleware.

const Redis = require('ioredis');

function createRedis({ addr = 'localhost:6379', password = '' } = {}) {
  let client = null;

  function getClient() {
    if (client) return client;
    const [host, port] = addr.split(':');
    client = new Redis({
      host: host || 'localhost',
      port: port ? Number(port) : 6379,
      password: password || undefined,
      lazyConnect: true,
      maxRetriesPerRequest: 1,
      enableReadyCheck: false,
      retryStrategy: () => null, // no auto-reconnect loop — per-request best effort
    });
    // Swallow connection errors; rate limiter degrades to allow-all.
    client.on('error', () => {});
    return client;
  }

  return {
    async incr(key) {
      return getClient().incr(key);
    },
    async expire(key, seconds) {
      return getClient().expire(key, seconds);
    },
    close() {
      if (client) client.disconnect();
      client = null;
    },
  };
}

module.exports = { createRedis };
