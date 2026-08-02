'use strict';

// Video REST routes — thin REST->gRPC forwarding to the Video Service.
// Identity comes from the verified JWT (request.user); validation helpers
// are reused from the feed routes (positiveIdParam/parsePage). All
// responses use the {code,message,data} envelope. Public read routes
// (get video / transcode status / list user videos) are exempted from auth
// in middleware/auth.js and pass viewer_id only when a token is present.

const { badRequest, ok, sendError } = require('../middleware/post');
const { positiveIdParam, parsePage } = require('./feed');

function toInt(value) {
  if (value === undefined || value === null || value === '') return undefined;
  const n = Number(value);
  return Number.isInteger(n) ? n : undefined;
}

// gRPC Empty messages deserialize to {}; expose as null in the envelope.
function isEmptyResponse(data) {
  return data && typeof data === 'object' && Object.keys(data).length === 0;
}

const VISIBILITY_VALUES = [
  'VIDEO_VISIBILITY_PUBLIC',
  'VIDEO_VISIBILITY_FOLLOWERS_ONLY',
  'VIDEO_VISIBILITY_PRIVATE',
];

function normalizeVisibility(value) {
  return typeof value === 'string' && VISIBILITY_VALUES.includes(value)
    ? value
    : 'VIDEO_VISIBILITY_PUBLIC';
}

// viewerId is only present when the request carried a valid JWT (public
// read routes may be called anonymously).
function viewerIdOf(request) {
  return request.user && request.user.id != null ? request.user.id : 0;
}

// Fastify plugin: registers all video REST routes on the given app.
function createVideoRoutes(app, { videoClient }) {
  // POST /api/video/init-upload — InitUpload
  app.post('/api/video/init-upload', async (request, reply) => {
    const body = request.body || {};
    const filename = typeof body.filename === 'string' ? body.filename : '';
    if (filename.trim() === '') return badRequest(reply, 'filename is required');
    const contentType = typeof body.content_type === 'string' ? body.content_type : '';
    const totalSize = toInt(body.total_size);
    if (totalSize === undefined || totalSize < 0) {
      return badRequest(reply, 'total_size must be a non-negative integer');
    }
    try {
      const data = await videoClient.initUpload(
        { user_id: request.user.id, filename, content_type: contentType, total_size: totalSize },
        { user: request.user }
      );
      return ok(reply, data);
    } catch (err) {
      return sendError(reply, err);
    }
  });

  // POST /api/video/upload-chunk — UploadChunk (data is base64 in the JSON body)
  app.post('/api/video/upload-chunk', async (request, reply) => {
    const body = request.body || {};
    const uploadId = typeof body.upload_id === 'string' ? body.upload_id : '';
    if (uploadId.trim() === '') return badRequest(reply, 'upload_id is required');
    const partNumber = toInt(body.part_number);
    if (partNumber === undefined || partNumber < 0) {
      return badRequest(reply, 'part_number must be a non-negative integer');
    }
    const data = typeof body.data === 'string' ? Buffer.from(body.data, 'base64') : Buffer.alloc(0);
    try {
      const res = await videoClient.uploadChunk(
        { user_id: request.user.id, upload_id: uploadId, part_number: partNumber, data },
        { user: request.user }
      );
      return ok(reply, res);
    } catch (err) {
      return sendError(reply, err);
    }
  });

  // POST /api/video/complete-upload — CompleteUpload
  app.post('/api/video/complete-upload', async (request, reply) => {
    const body = request.body || {};
    const uploadId = typeof body.upload_id === 'string' ? body.upload_id : '';
    if (uploadId.trim() === '') return badRequest(reply, 'upload_id is required');
    const title = typeof body.title === 'string' ? body.title : '';
    if (title.trim() === '') return badRequest(reply, 'title is required');
    const description = typeof body.description === 'string' ? body.description : '';
    const visibility = normalizeVisibility(body.visibility);
    try {
      const data = await videoClient.completeUpload(
        { user_id: request.user.id, upload_id: uploadId, title, description, visibility },
        { user: request.user }
      );
      return ok(reply, data && data.video ? data.video : data);
    } catch (err) {
      return sendError(reply, err);
    }
  });

  // GET /api/video/:id — GetVideo (public; viewer_id optional)
  app.get('/api/video/:id', async (request, reply) => {
    const id = positiveIdParam(request.params.id, reply);
    if (id === null) return;
    try {
      const data = await videoClient.getVideo(
        { video_id: id, viewer_id: viewerIdOf(request) },
        { user: request.user }
      );
      return ok(reply, data);
    } catch (err) {
      return sendError(reply, err);
    }
  });

  // GET /api/video/:id/playback — GetPlaybackURL (auth)
  app.get('/api/video/:id/playback', async (request, reply) => {
    const id = positiveIdParam(request.params.id, reply);
    if (id === null) return;
    try {
      const data = await videoClient.getPlaybackURL(
        { video_id: id, user_id: request.user.id },
        { user: request.user }
      );
      return ok(reply, data);
    } catch (err) {
      return sendError(reply, err);
    }
  });

  // GET /api/video/:id/transcode-status — GetTranscodeStatus (public)
  app.get('/api/video/:id/transcode-status', async (request, reply) => {
    const id = positiveIdParam(request.params.id, reply);
    if (id === null) return;
    try {
      const data = await videoClient.getTranscodeStatus(
        { video_id: id },
        { user: request.user }
      );
      return ok(reply, data);
    } catch (err) {
      return sendError(reply, err);
    }
  });

  // DELETE /api/video/:id — DeleteVideo (auth)
  app.delete('/api/video/:id', async (request, reply) => {
    const id = positiveIdParam(request.params.id, reply);
    if (id === null) return;
    try {
      const data = await videoClient.deleteVideo(
        { video_id: id, user_id: request.user.id },
        { user: request.user }
      );
      return ok(reply, isEmptyResponse(data) ? null : data);
    } catch (err) {
      return sendError(reply, err);
    }
  });

  // GET /api/video/user/:userId — ListUserVideos (public; viewer_id optional)
  app.get('/api/video/user/:userId', async (request, reply) => {
    const userId = positiveIdParam(request.params.userId, reply);
    if (userId === null) return;
    const page = parsePage(request.query, reply);
    if (!page) return;
    try {
      const data = await videoClient.listUserVideos(
        {
          user_id: userId,
          viewer_id: viewerIdOf(request),
          page: { cursor: page.cursor, limit: page.limit },
        },
        { user: request.user }
      );
      return ok(reply, data);
    } catch (err) {
      return sendError(reply, err);
    }
  });
}

module.exports = { createVideoRoutes };