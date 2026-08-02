-- 03-video.sql — video_db schema (P3)
-- Runs once on first container start via /docker-entrypoint-initdb.d (as root).
-- Repeatable-safe: CREATE IF NOT EXISTS.

SET NAMES utf8mb4;
CREATE DATABASE IF NOT EXISTS video_db CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
USE video_db;

-- videos: canonical video metadata
CREATE TABLE IF NOT EXISTS videos (
    id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    uploader_id BIGINT UNSIGNED NOT NULL,
    title       VARCHAR(255)    NOT NULL DEFAULT '',
    description VARCHAR(1000)   NOT NULL DEFAULT '',
    visibility  ENUM ('public', 'followers_only', 'private') NOT NULL DEFAULT 'public',
    status      ENUM ('pending', 'processing', 'completed', 'failed') NOT NULL DEFAULT 'pending',
    raw_key     VARCHAR(500)    NOT NULL DEFAULT '' COMMENT 'S3 key for original file',
    hls_key     VARCHAR(500)    NOT NULL DEFAULT '' COMMENT 'S3 key prefix for HLS segments',
    thumb_key   VARCHAR(500)    NOT NULL DEFAULT '' COMMENT 'S3 key for thumbnail',
    duration    INT UNSIGNED    NOT NULL DEFAULT 0 COMMENT 'seconds',
    created_at  DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at  DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    INDEX idx_videos_uploader_id (uploader_id),
    INDEX idx_videos_status (status),
    INDEX idx_videos_created_at (created_at)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci;

-- uploads: multipart upload session tracking (断点续传)
CREATE TABLE IF NOT EXISTS uploads (
    upload_id       VARCHAR(128) PRIMARY KEY COMMENT 'S3 multipart upload ID',
    video_id        BIGINT UNSIGNED NOT NULL,
    filename        VARCHAR(255)    NOT NULL DEFAULT '',
    content_type    VARCHAR(128)    NOT NULL DEFAULT 'video/mp4',
    total_size      BIGINT UNSIGNED NOT NULL DEFAULT 0,
    chunk_size      INT UNSIGNED    NOT NULL DEFAULT 5242880 COMMENT '5MB default',
    total_chunks    INT UNSIGNED    NOT NULL DEFAULT 0,
    received_chunks INT UNSIGNED    NOT NULL DEFAULT 0,
    status          ENUM ('pending', 'uploading', 'completed', 'aborted') NOT NULL DEFAULT 'pending',
    created_at      DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at      DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    INDEX idx_uploads_video_id (video_id),
    CONSTRAINT fk_uploads_video FOREIGN KEY (video_id) REFERENCES videos (id)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci;

-- transcode_tasks: FFmpeg transcoding job records
CREATE TABLE IF NOT EXISTS transcode_tasks (
    id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    video_id    BIGINT UNSIGNED NOT NULL,
    quality     VARCHAR(16)     NOT NULL COMMENT '720p / 480p / 360p',
    status      ENUM ('pending', 'processing', 'completed', 'failed') NOT NULL DEFAULT 'pending',
    retry_count INT UNSIGNED    NOT NULL DEFAULT 0,
    max_retries INT UNSIGNED    NOT NULL DEFAULT 3,
    error_msg   VARCHAR(1000)   NOT NULL DEFAULT '',
    created_at  DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at  DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    INDEX idx_transcode_tasks_video_id (video_id),
    INDEX idx_transcode_tasks_status (status),
    CONSTRAINT fk_transcode_tasks_video FOREIGN KEY (video_id) REFERENCES videos (id)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci;

-- Dedicated MySQL account for Video Service
CREATE USER IF NOT EXISTS 'video'@'%' IDENTIFIED BY 'video123';
GRANT ALL PRIVILEGES ON video_db.* TO 'video'@'%';
FLUSH PRIVILEGES;
