'use strict';

// Dev-only auth endpoints (P1): JWT issuance + hardcoded seed users.
// These are public routes; the Feed Service validates user existence
// on create, so /api/dev/login performs NO existence check.

const jwt = require('jsonwebtoken');
const { badRequest, ok } = require('../middleware/post');

// Must match Feed Service seeds exactly (P2 will replace with User Service).
const DEV_USERS = [
  { id: 1, username: 'bob', display_name: 'Bob咖啡师', avatar_url: '' },
  { id: 2, username: 'alice', display_name: 'Alice设计师', avatar_url: '' },
  { id: 3, username: 'carol', display_name: 'Carol摄影师', avatar_url: '' },
  { id: 4, username: 'dave', display_name: 'Dave开发者', avatar_url: '' },
];

// Fastify plugin: registers dev endpoints directly on the given app.
function createDevRoutes(app, { jwtSecret, ttlSeconds = 24 * 60 * 60 }) {
  app.post('/api/dev/login', async (request, reply) => {
    const body = request.body || {};
    const userId = Number(body.user_id);
    if (!Number.isInteger(userId) || userId <= 0) {
      return badRequest(reply, 'user_id must be a positive integer');
    }

    const now = Math.floor(Date.now() / 1000);
    const token = jwt.sign(
      {
        sub: String(userId),
        username: body.username || '',
        displayName: body.display_name || '',
        iat: now,
      },
      jwtSecret,
      { algorithm: 'HS256', expiresIn: ttlSeconds }
    );

    return ok(reply, {
      token,
      user_id: userId,
      expires_in: ttlSeconds,
    });
  });

  app.get('/api/dev/users', async (_request, reply) => ok(reply, DEV_USERS));
}

module.exports = { createDevRoutes, DEV_USERS };
