'use strict';

// Layer 1 — pre middleware: request-id, logging, CORS, body limit.
// Fastify's built-in bodyLimit (1MB) is set in app.js; this layer owns
// request-id assignment + per-request pino logger + CORS.

const { v4: uuidv4 } = require('uuid');
const pino = require('pino');

function createPreMiddleware({ logger = pino({ level: 'info' }), corsOrigins = ['http://localhost:3000', 'http://localhost:3001'] } = {}) {
  return async function preMiddleware(request, reply) {
    // 1. Request id: honour inbound x-request-id, else uuid v4.
    const inboundId = request.headers['x-request-id'];
    const requestId = typeof inboundId === 'string' && inboundId.trim() !== '' ? inboundId : uuidv4();
    request.id = requestId;
    request.log = logger.child({ requestId });
    reply.header('x-request-id', requestId);
  };
}

module.exports = { createPreMiddleware };
