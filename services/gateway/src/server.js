'use strict';

// Gateway bootstrap: build the app, listen on GW_PORT. Must boot cleanly
// even when the Feed Service and/or Redis are down (all clients are lazy).

const pino = require('pino');
const config = require('./config');
const { buildApp } = require('./app');
const { createRedis } = require('./redis');
const { createBreaker } = require('./middleware/breaker');

const logger = pino({ level: process.env.LOG_LEVEL || 'info' });

async function main() {
  const redis = createRedis({ addr: config.redisAddr, password: config.redisPassword });
  const breaker = createBreaker(config.breaker);
  const app = buildApp({ logger, redis, breaker });

  app.addHook('onClose', async () => {
    redis.close();
    try {
      app.feedClient.close && app.feedClient.close();
    } catch (err) {
      logger.warn({ err: err.message }, 'failed to close feed client');
    }
  });

  try {
    await app.listen({ port: config.port, host: config.host });
    logger.info(`gateway listening on ${config.host}:${config.port} (feed: ${config.feedAddr})`);
  } catch (err) {
    logger.error({ err }, 'gateway failed to start');
    process.exit(1);
  }
}

main();
