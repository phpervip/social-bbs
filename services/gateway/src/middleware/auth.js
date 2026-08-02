'use strict';

// Layer 2 — auth middleware: JWT verification (HS256) for protected routes.
// Public routes: /api/auth/register, /api/auth/login and /healthz. Success
// injects request.user. After verification the token's jti is checked
// against the Redis blacklist (auth:blacklist:{jti}, written by the User
// Service on logout); Redis being unavailable degrades open (allow).

const crypto = require('crypto');
const jwt = require('jsonwebtoken');

const PUBLIC_PREFIXES = ['/api/auth/register', '/api/auth/login', '/healthz'];

// The User Service signs JWTs with Keys.hmacShaKeyFor(SHA256(jwtSecret))
// (JwtUtil), i.e. the HMAC key is the SHA-256 digest of the secret. The
// gateway MUST derive the identical key or every token verification fails
// with "invalid signature". jsonwebtoken accepts a Buffer key.
function deriveKey(jwtSecret) {
  return crypto.createHash('sha256').update(jwtSecret).digest();
}

function isPublic(url) {
  return PUBLIC_PREFIXES.some((p) => url === p || url.startsWith(p + '/') || url.startsWith(p + '?'));
}

// bearer() returns { token } or null when the header is missing/malformed.
function parseBearer(headerValue) {
  if (typeof headerValue !== 'string') return null;
  const match = /^Bearer\s+(.+)$/i.exec(headerValue.trim());
  return match ? { token: match[1].trim() } : null;
}

function createAuthMiddleware({ jwtSecret, redis = null }) {
  return async function authMiddleware(request, reply) {
    if (isPublic(request.url)) return;

    const parsed = parseBearer(request.headers.authorization);
    if (!parsed) {
      return reply.status(401).send({ code: 401, message: 'unauthorized', data: null });
    }

    let decoded;
    try {
      decoded = jwt.verify(parsed.token, deriveKey(jwtSecret), { algorithms: ['HS256'] });
    } catch (err) {
      request.log?.warn({ err: err.message }, 'jwt verification failed');
      return reply.status(401).send({ code: 401, message: 'unauthorized', data: null });
    }

    // Logged-out tokens are blacklisted by jti (User Service writes the key
    // on logout with TTL = remaining JWT lifetime; gateway only reads it).
    if (redis && typeof redis.get === 'function' && typeof decoded.jti === 'string') {
      try {
        const blacklisted = await redis.get(`auth:blacklist:${decoded.jti}`);
        if (blacklisted !== null && blacklisted !== undefined) {
          return reply.status(401).send({ code: 401, message: 'unauthorized', data: null });
        }
      } catch (err) {
        // Redis is a soft dependency for auth — never fail the request
        // because the blacklist is unreachable.
        request.log?.warn({ err: err.message }, 'blacklist check unavailable, allowing request');
      }
    }

    const id = Number.parseInt(decoded.sub, 10);
    if (!Number.isInteger(id) || id <= 0) {
      return reply.status(401).send({ code: 401, message: 'unauthorized', data: null });
    }

    request.user = {
      id,
      username: typeof decoded.username === 'string' ? decoded.username : '',
      displayName: typeof decoded.displayName === 'string' ? decoded.displayName : '',
      jti: typeof decoded.jti === 'string' ? decoded.jti : undefined,
    };
  };
}

module.exports = { createAuthMiddleware, isPublic, parseBearer };
