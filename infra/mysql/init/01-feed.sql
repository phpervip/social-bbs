-- 01-feed.sql — feed_db schema + seed users (P1)
-- Runs once on first container start via /docker-entrypoint-initdb.d (as root).
-- Repeatable-safe: explicit-id INSERT IGNORE for seeds.

SET NAMES utf8mb4;
USE feed_db;

-- users: demo accounts (P1: no User Service, seeded here)
CREATE TABLE IF NOT EXISTS users (
    id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    username    VARCHAR(64)  NOT NULL UNIQUE,
    display_name VARCHAR(64) NOT NULL,
    avatar_url  VARCHAR(255) NOT NULL DEFAULT '',
    created_at  DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci;

-- posts: feed items, soft delete via deleted_at
CREATE TABLE IF NOT EXISTS posts (
    id            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id       BIGINT UNSIGNED NOT NULL,
    content       TEXT            NOT NULL,
    media_url     VARCHAR(500)    NOT NULL DEFAULT '',
    like_count    INT             NOT NULL DEFAULT 0,
    comment_count INT             NOT NULL DEFAULT 0,
    created_at    DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    deleted_at    DATETIME(3)     NULL,
    INDEX idx_posts_user_id (user_id),
    INDEX idx_posts_created_at (created_at)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci;

-- post_likes: composite PK, physical FK to posts (demo environment)
CREATE TABLE IF NOT EXISTS post_likes (
    post_id    BIGINT UNSIGNED NOT NULL,
    user_id    BIGINT UNSIGNED NOT NULL,
    created_at DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (post_id, user_id),
    CONSTRAINT fk_post_likes_post FOREIGN KEY (post_id) REFERENCES posts (id)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci;

-- post_comments
CREATE TABLE IF NOT EXISTS post_comments (
    id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    post_id    BIGINT UNSIGNED NOT NULL,
    user_id    BIGINT UNSIGNED NOT NULL,
    content    VARCHAR(500)    NOT NULL,
    created_at DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    INDEX idx_post_comments_post_id (post_id),
    INDEX idx_post_comments_created_at (created_at)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci;

-- outbox_events: created in P1, written from P2 (Kafka backfill)
CREATE TABLE IF NOT EXISTS outbox_events (
    id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    topic       VARCHAR(64) NOT NULL,
    payload     JSON        NOT NULL,
    status      ENUM ('pending', 'delivered', 'failed') NOT NULL DEFAULT 'pending',
    retry_count INT         NOT NULL DEFAULT 0,
    created_at  DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    INDEX idx_outbox_events_status (status)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci;

-- Seed users (must match Gateway /api/dev/users hardcoded list). Idempotent.
INSERT IGNORE INTO users (id, username, display_name, avatar_url) VALUES
    (1, 'bob',   'Bob咖啡师',   ''),
    (2, 'alice', 'Alice设计师', ''),
    (3, 'carol', 'Carol摄影师', ''),
    (4, 'dave',  'Dave开发者',  '');
