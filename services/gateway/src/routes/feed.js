'use strict';

// Feed REST routes — thin REST->gRPC forwarding.
// Validation happens here (params/body/query); downstream failures surface
// through the post.js error mapping. All calls go through the feed client,
// which is wrapped by the circuit breaker (see app.js).

const { badRequest, ok, sendError } = require('../middleware/post');

function toInt(value) {
  if (value === undefined || value === null || value === '') return undefined;
  const n = Number(value);
  return Number.isInteger(n) ? n : undefined;
}

// Positive-integer path param or a 400.
function positiveIdParam(value, reply) {
  const n = toInt(value);
  if (n === undefined || n <= 0) {
    badRequest(reply, 'id must be a positive integer');
    return null;
  }
  return n;
}

// cursor/limit from query string: cursor default 0, limit default 20 (max 50).
function parsePage(query, reply) {
  let cursor = toInt(query.cursor);
  if (cursor === undefined) cursor = 0;
  if (!Number.isInteger(cursor) || cursor < 0) {
    badRequest(reply, 'cursor must be a non-negative integer');
    return null;
  }
  let limit = toInt(query.limit);
  if (limit === undefined) limit = 20;
  if (!Number.isInteger(limit) || limit < 1 || limit > 50) {
    badRequest(reply, 'limit must be an integer between 1 and 50');
    return null;
  }
  return { cursor, limit };
}

function isEmptyResponse(data) {
  // gRPC Empty messages deserialize to {}; expose as null in the envelope.
  return data && typeof data === 'object' && Object.keys(data).length === 0;
}

// Fastify plugin: registers all feed REST routes on the given app.
function createFeedRoutes(app, { feedClient }) {
  // POST /api/feed/post — CreatePost
  app.post('/api/feed/post', async (request, reply) => {
    const body = request.body || {};
    const content = typeof body.content === 'string' ? body.content : '';
    if (content.trim() === '') return badRequest(reply, 'content is required');
    try {
      const data = await feedClient.createPost({
        user_id: request.user.id,
        content,
        media_url: typeof body.media_url === 'string' ? body.media_url : '',
      });
      return ok(reply, data);
    } catch (err) {
      return sendError(reply, err);
    }
  });

  // GET /api/feed/home — GetHomeTimeline
  app.get('/api/feed/home', async (request, reply) => {
    const page = parsePage(request.query, reply);
    if (!page) return;
    try {
      const data = await feedClient.getHomeTimeline({
        user_id: request.user.id,
        page,
      });
      return ok(reply, data);
    } catch (err) {
      return sendError(reply, err);
    }
  });

  // GET /api/feed/post/:id — GetPost
  app.get('/api/feed/post/:id', async (request, reply) => {
    const id = positiveIdParam(request.params.id, reply);
    if (id === null) return;
    try {
      const data = await feedClient.getPost({ id, viewer_id: request.user.id });
      return ok(reply, data);
    } catch (err) {
      return sendError(reply, err);
    }
  });

  // DELETE /api/feed/post/:id — DeletePost
  app.delete('/api/feed/post/:id', async (request, reply) => {
    const id = positiveIdParam(request.params.id, reply);
    if (id === null) return;
    try {
      const data = await feedClient.deletePost({ id, user_id: request.user.id });
      return ok(reply, isEmptyResponse(data) ? null : data);
    } catch (err) {
      return sendError(reply, err);
    }
  });

  // POST /api/feed/like — LikePost
  app.post('/api/feed/like', async (request, reply) => {
    const postId = positiveIdParam(request.body && request.body.post_id, reply);
    if (postId === null) return;
    try {
      const data = await feedClient.likePost({ user_id: request.user.id, post_id: postId });
      return ok(reply, isEmptyResponse(data) ? null : data);
    } catch (err) {
      return sendError(reply, err);
    }
  });

  // DELETE /api/feed/like — UnlikePost
  app.delete('/api/feed/like', async (request, reply) => {
    const postId = positiveIdParam(request.body && request.body.post_id, reply);
    if (postId === null) return;
    try {
      const data = await feedClient.unlikePost({ user_id: request.user.id, post_id: postId });
      return ok(reply, isEmptyResponse(data) ? null : data);
    } catch (err) {
      return sendError(reply, err);
    }
  });

  // POST /api/feed/comment — AddComment
  app.post('/api/feed/comment', async (request, reply) => {
    const body = request.body || {};
    const postId = positiveIdParam(body.post_id, reply);
    if (postId === null) return;
    const content = typeof body.content === 'string' ? body.content : '';
    if (content.trim() === '') return badRequest(reply, 'content is required');
    try {
      const data = await feedClient.addComment({
        post_id: postId,
        user_id: request.user.id,
        content,
      });
      return ok(reply, data);
    } catch (err) {
      return sendError(reply, err);
    }
  });

  // GET /api/feed/post/:id/comments — GetComments
  app.get('/api/feed/post/:id/comments', async (request, reply) => {
    const id = positiveIdParam(request.params.id, reply);
    if (id === null) return;
    const page = parsePage(request.query, reply);
    if (!page) return;
    try {
      const data = await feedClient.getComments({ post_id: id, page });
      return ok(reply, data);
    } catch (err) {
      return sendError(reply, err);
    }
  });

  // GET /api/search — Search
  app.get('/api/search', async (request, reply) => {
    const q = typeof request.query.q === 'string' ? request.query.q.trim() : '';
    if (q === '') return badRequest(reply, 'q is required');
    const page = parsePage(request.query, reply);
    if (!page) return;
    try {
      const data = await feedClient.search({ query: q, user_id: request.user.id, page });
      return ok(reply, data);
    } catch (err) {
      return sendError(reply, err);
    }
  });
}

module.exports = { createFeedRoutes, parsePage, positiveIdParam };
