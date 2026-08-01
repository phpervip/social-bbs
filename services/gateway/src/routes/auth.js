'use strict';

// Auth REST routes — thin REST->gRPC forwarding to the User Service.
// register/login are public (see auth middleware PUBLIC_PREFIXES); logout
// requires a valid token and hands the token's jti to the User Service so
// it can blacklist it. All responses use the {code,message,data} envelope.

const { badRequest, ok, sendError } = require('../middleware/post');

// gRPC Empty messages deserialize to {}; expose as null in the envelope.
function isEmptyResponse(data) {
  return data && typeof data === 'object' && Object.keys(data).length === 0;
}

// Fastify plugin: registers all auth REST routes on the given app.
function createAuthRoutes(app, { userClient }) {
  // POST /api/auth/register — Register
  app.post('/api/auth/register', async (request, reply) => {
    const body = request.body || {};
    const username = typeof body.username === 'string' ? body.username.trim() : '';
    const email = typeof body.email === 'string' ? body.email.trim() : '';
    const password = typeof body.password === 'string' ? body.password : '';
    const displayName = typeof body.display_name === 'string' ? body.display_name : '';
    if (username === '') return badRequest(reply, 'username is required');
    if (email === '') return badRequest(reply, 'email is required');
    if (password === '') return badRequest(reply, 'password is required');
    try {
      const data = await userClient.register({ username, email, password, display_name: displayName });
      return ok(reply, data);
    } catch (err) {
      return sendError(reply, err);
    }
  });

  // POST /api/auth/login — Login (account = username or email)
  app.post('/api/auth/login', async (request, reply) => {
    const body = request.body || {};
    const account = typeof body.account === 'string' ? body.account.trim() : '';
    const password = typeof body.password === 'string' ? body.password : '';
    if (account === '') return badRequest(reply, 'account is required');
    if (password === '') return badRequest(reply, 'password is required');
    try {
      const data = await userClient.login({ account, password });
      return ok(reply, data);
    } catch (err) {
      return sendError(reply, err);
    }
  });

  // POST /api/auth/logout — Logout (blacklists the token's jti downstream)
  app.post('/api/auth/logout', async (request, reply) => {
    const jti = request.user && request.user.jti;
    if (!jti) return badRequest(reply, 'missing token jti');
    try {
      const data = await userClient.logout({ jti });
      return ok(reply, isEmptyResponse(data) ? null : data);
    } catch (err) {
      return sendError(reply, err);
    }
  });
}

module.exports = { createAuthRoutes };
