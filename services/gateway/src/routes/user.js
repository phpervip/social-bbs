'use strict';

// User REST routes — thin REST->gRPC forwarding to the User Service.
// Identity comes from the verified JWT (request.user); validation helpers
// are reused from the feed routes (positiveIdParam/parsePage). All
// responses use the {code,message,data} envelope.

const { badRequest, ok, sendError } = require('../middleware/post');
const { positiveIdParam, parsePage } = require('./feed');

// gRPC Empty messages deserialize to {}; expose as null in the envelope.
function isEmptyResponse(data) {
  return data && typeof data === 'object' && Object.keys(data).length === 0;
}

// Fastify plugin: registers all user REST routes on the given app.
function createUserRoutes(app, { userClient }) {
  // GET /api/user/:id — GetProfile
  app.get('/api/user/:id', async (request, reply) => {
    const id = positiveIdParam(request.params.id, reply);
    if (id === null) return;
    try {
      const data = await userClient.getProfile({ user_id: id }, { user: request.user });
      return ok(reply, data);
    } catch (err) {
      return sendError(reply, err);
    }
  });

  // PUT /api/user/profile — UpdateProfile (current user from token)
  app.put('/api/user/profile', async (request, reply) => {
    const body = request.body || {};
    const displayName = typeof body.display_name === 'string' ? body.display_name : '';
    const bio = typeof body.bio === 'string' ? body.bio : '';
    const avatarUrl = typeof body.avatar_url === 'string' ? body.avatar_url : '';
    try {
      const data = await userClient.updateProfile(
        { user_id: request.user.id, display_name: displayName, bio, avatar_url: avatarUrl },
        { user: request.user }
      );
      return ok(reply, data);
    } catch (err) {
      return sendError(reply, err);
    }
  });

  // POST /api/user/:id/follow — Follow
  app.post('/api/user/:id/follow', async (request, reply) => {
    const id = positiveIdParam(request.params.id, reply);
    if (id === null) return;
    try {
      const data = await userClient.follow(
        { follower_id: request.user.id, followee_id: id },
        { user: request.user }
      );
      return ok(reply, isEmptyResponse(data) ? null : data);
    } catch (err) {
      return sendError(reply, err);
    }
  });

  // DELETE /api/user/:id/follow — Unfollow
  app.delete('/api/user/:id/follow', async (request, reply) => {
    const id = positiveIdParam(request.params.id, reply);
    if (id === null) return;
    try {
      const data = await userClient.unfollow(
        { follower_id: request.user.id, followee_id: id },
        { user: request.user }
      );
      return ok(reply, isEmptyResponse(data) ? null : data);
    } catch (err) {
      return sendError(reply, err);
    }
  });

  // GET /api/user/:id/followers — GetFollowers
  app.get('/api/user/:id/followers', async (request, reply) => {
    const id = positiveIdParam(request.params.id, reply);
    if (id === null) return;
    const page = parsePage(request.query, reply);
    if (!page) return;
    try {
      const data = await userClient.getFollowers(
        { user_id: id, cursor: page.cursor, limit: page.limit },
        { user: request.user }
      );
      return ok(reply, data);
    } catch (err) {
      return sendError(reply, err);
    }
  });

  // GET /api/user/:id/following — GetFollowing
  app.get('/api/user/:id/following', async (request, reply) => {
    const id = positiveIdParam(request.params.id, reply);
    if (id === null) return;
    const page = parsePage(request.query, reply);
    if (!page) return;
    try {
      const data = await userClient.getFollowing(
        { user_id: id, cursor: page.cursor, limit: page.limit },
        { user: request.user }
      );
      return ok(reply, data);
    } catch (err) {
      return sendError(reply, err);
    }
  });
}

module.exports = { createUserRoutes };
