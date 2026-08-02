-- 02-user.sql — user_db schema + seed users (P2)
-- Runs once on first container start via /docker-entrypoint-initdb.d (as root).
-- Repeatable-safe: explicit-id INSERT IGNORE for seeds.
-- Note: timestamp columns use BIGINT (unix ms) to match the Java User Service entity.

SET NAMES utf8mb4;
CREATE DATABASE IF NOT EXISTS user_db CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
USE user_db;

CREATE TABLE IF NOT EXISTS users (
    id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    username        VARCHAR(64)     NOT NULL UNIQUE,
    email           VARCHAR(255)    NOT NULL UNIQUE,
    password_hash   VARCHAR(255)    NOT NULL,
    display_name    VARCHAR(100)    NOT NULL DEFAULT '',
    bio             VARCHAR(500)    NOT NULL DEFAULT '',
    avatar_url      VARCHAR(500)    NOT NULL DEFAULT '',
    follower_count  INT             NOT NULL DEFAULT 0,
    following_count INT             NOT NULL DEFAULT 0,
    status          TINYINT         NOT NULL DEFAULT 1,
    created_at      BIGINT          NOT NULL DEFAULT 0,
    updated_at      BIGINT          NOT NULL DEFAULT 0
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS follows (
    follower_id BIGINT UNSIGNED NOT NULL,
    followee_id BIGINT UNSIGNED NOT NULL,
    created_at  BIGINT          NOT NULL DEFAULT 0,
    PRIMARY KEY (follower_id, followee_id),
    CONSTRAINT fk_follows_follower FOREIGN KEY (follower_id) REFERENCES users (id),
    CONSTRAINT fk_follows_followee FOREIGN KEY (followee_id) REFERENCES users (id)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci;

-- R1: 事件统一 Outbox 模式 — User Service 事件先写本表，再异步投递 Kafka
CREATE TABLE IF NOT EXISTS user_outbox (
    id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    topic       VARCHAR(64)     NOT NULL,
    payload     TEXT            NOT NULL,
    status      VARCHAR(16)     NOT NULL DEFAULT 'pending',
    retry_count INT             NOT NULL DEFAULT 0,
    created_at  BIGINT          NOT NULL DEFAULT 0,
    updated_at  BIGINT          NOT NULL DEFAULT 0,
    INDEX idx_user_outbox_status (status, id)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci;

-- R8: revoked(DB 权威态) + auth:blacklist:{jti}(Redis 快检) 双重兜底鉴权
CREATE TABLE IF NOT EXISTS user_sessions (
    token_id   VARCHAR(64)     PRIMARY KEY,
    user_id    BIGINT UNSIGNED NOT NULL,
    expires_at BIGINT          NOT NULL,
    revoked    TINYINT         NOT NULL DEFAULT 0,
    created_at BIGINT          NOT NULL DEFAULT 0,
    INDEX idx_user_sessions_user_id (user_id),
    CONSTRAINT fk_user_sessions_user FOREIGN KEY (user_id) REFERENCES users (id)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci;

-- Seed users (password = Password123! for all). Idempotent.
INSERT IGNORE INTO users (id, username, email, password_hash, display_name) VALUES
    (1, 'bob',   'bob@b.dev',   '$2b$10$1mKiflFeNx2ChiCdwYrJe.gKyVl75PhlVWac3BP2MKKEQS6VJyD2G', 'Bob咖啡师'),
    (2, 'alice', 'alice@b.dev', '$2b$10$1mKiflFeNx2ChiCdwYrJe.gKyVl75PhlVWac3BP2MKKEQS6VJyD2G', 'Alice设计师'),
    (3, 'carol', 'carol@b.dev', '$2b$10$1mKiflFeNx2ChiCdwYrJe.gKyVl75PhlVWac3BP2MKKEQS6VJyD2G', 'Carol摄影师'),
    (4, 'dave',  'dave@b.dev',  '$2b$10$1mKiflFeNx2ChiCdwYrJe.gKyVl75PhlVWac3BP2MKKEQS6VJyD2G', 'Dave开发者');

-- Dedicated MySQL account for User Service (replaces root in application.yml)
CREATE USER IF NOT EXISTS 'user'@'%' IDENTIFIED BY 'user123';
GRANT ALL PRIVILEGES ON user_db.* TO 'user'@'%';
FLUSH PRIVILEGES;
