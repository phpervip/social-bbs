'use strict';

// Layer 2 — auth middleware: JWT verification (HS256) for protected routes.
// Public routes: /api/dev/* and /healthz. Success injects request.user.

const jwt = require('jsonwebtoken');

const PUBLIC_PREFIXES = ['/api/dev', '/healthz'];

function isPublic(url) {
  return PUBLIC_PREFIXES.some((p) => url === p || url.startsWith(p + '/') || url.startsWith(p + '?'));
}

// bearer() returns { token } or null when the header is missing/malformed.
function parseBearer(headerValue) {
  if (typeof headerValue !== 'string') return null;
  const match = /^Bearer\s+(.+)$/i.exec(headerValue.trim());
  return match ? { token: match[1].trim() } : null;
}

function createAuthMiddleware({ jwtSecret }) {
  return async function authMiddleware(request, reply) {
    if (isPublic(request.url)) return;

    const parsed = parseBearer(request.headers.authorization);
    if (!parsed) {
      return reply.status(401).send({ code: 401, message: 'unauthorized', data: null });
    }

    let decoded;
    try {
      decoded = jwt.verify(parsed.token, jwtSecret, { algorithms: ['HS256'] });
    } catch (err) {
      request.log?.warn({ err: err.message }, 'jwt verification failed');
      return reply.status(401).send({ code: 401, message: 'unauthorized', data: null });
    }

    const id = Number.parseInt(decoded.sub, 10);
    if (!Number.isInteger(id) || id <= 0) {
      return reply.status(401).send({ code: 401, message: 'unauthorized', data: null });
    }

    request.user = {
      id,
      username: typeof decoded.username === 'string' ? decoded.username : '',
      displayName: typeof decoded.displayName === 'string' ? decoded.displayName : '',
    };
  };
}

module.exports = { createAuthMiddleware, isPublic, parseBearer };
