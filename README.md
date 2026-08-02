# B — 简化版 社交平台（微服务学习项目）

Logo：**B** · 咖啡棕 #8B5A2B 品牌色 · 极简黑白中性底

架构设计：`.superpowers/brainstorm/IMPLEMENTATION_HANDOFF.md`（含全部设计图索引）
当前阶段：**P3 ✅ Video Service + S3 分片 + FFmpeg HLS 播放**

## P3 组件

| 组件 | 技术 | 目录 | 端口 |
|---|---|---|---|
| Shell (MF Host) | React 18 + Webpack 5 MF | `frontend/shell/` | 3000 |
| Feed Remote (MF Remote) | React 18 + Webpack 5 MF | `frontend/feed-remote/` | 3001 |
| **User Remote (MF Remote)** | **React 18 + Webpack 5 MF** | **`frontend/user-remote/`** | **3002** |
| **Video Remote (MF Remote)** | **React 18 + Webpack 5 MF** | **`frontend/video-remote/`** | **3003** |
| API Gateway | Node.js + Fastify | `services/gateway/` | 8080 |
| Feed Service | Go + gRPC + GORM | `services/feed-service/` | 9000 |
| **User Service** | **Java 21 + Spring Boot 3 + gRPC** | **`services/user-service/`** | **9001** |
| **Video Service** | **Go + gRPC + GORM + MinIO S3 + FFmpeg** | **`services/video-service/`** | **9002** |
| MySQL 8 / Redis 7 / Kafka (KRaft) | docker-compose | `infra/` | 3306 / 6379 / **9092** |
| **MinIO (S3)** | **Object storage** | **docker-compose** | **9000 (API) / 9001 (Console)** |

请求链路：`Browser → http://localhost:3000 → /api (proxy) → Gateway:8080 → gRPC → Feed:9000 / User:9001 / Video:9002 → MySQL/Redis/Kafka/MinIO`

## 快速启动（详见 `infra/README.md`）

```powershell
# 一键启动（推荐）
.\infra\start-all.ps1

# 或手动启动：
# 1. 基础设施（MySQL + Redis + Kafka + MinIO）
cd infra; docker compose up -d

# 2. Feed Service（Go）
cd ../services/feed-service; go run ./cmd/server

# 3. Video Service（Go）
cd ../services/video-service; go run ./cmd/server

# 4. Gateway（Node）
cd ../services/gateway; npm install; npm run dev

# 5. 前端（四个终端）
cd ../../frontend/shell; npm install; npm run dev          # :3000
cd ../../frontend/feed-remote; npm install; npm run dev    # :3001
cd ../../frontend/user-remote; npm install; npm run dev    # :3002
cd ../../frontend/video-remote; npm install; npm run dev   # :3003

# 6. 浏览器打开 http://localhost:3000 → 注册/登录 → 发帖/上传视频
```

## 阶段说明（计划 B 纵向切片）

- P1 ✅ 发帖 → 首页时间线
- P2 ✅ 账号体系 + 关注 + JWT 鉴权 + Kafka/outbox 事件化（本阶段）
- P3 Video Service + S3 分片 + FFmpeg HLS
- P4 Kafka 事件化 + Notification + ES Search

## P2 验证结果

| 功能 | 状态 | 证据 |
|---|---|---|
| 注册 | ✅ | 200, user + token 返回 |
| 登录 | ✅ | 200, JWT 签发 |
| 关注/取关 | ✅ | 200, home 动态更新 |
| JWT 鉴权 | ✅ | 401 on invalid token |
| Feed outbox 事件化 | ✅ | outbox_events 表有 delivered 记录 |
| User outbox 事件化 | ✅ | user_outbox 表有 follow-changed 记录 |
| Kafka fanout | ✅ | follow 后 Bob 帖子出现在 Alice home |
| Kafka timeline | ✅ | unfollow 后新帖不再出现 |

### Kafka 事件流验证

```sql
-- Feed Service outbox
SELECT id, topic, status, retry_count FROM outbox_events ORDER BY id DESC LIMIT 5;
-- 结果: post.created × 5, 全部 delivered

-- User Service outbox
SELECT id, topic, payload, status FROM user_outbox ORDER BY id DESC LIMIT 5;
-- 结果: user.follow-changed × 2, user.registered × 3, 全部 delivered
```

### E2E 流程验证

```powershell
# 1. 注册 alice & bob
# 2. Alice follows Bob → 200
# 3. Bob 发帖 ×3 → 200
# 4. Alice home timeline → 3 条 Bob 帖子 ✓
# 5. Alice unfollows Bob → 200
# 6. Bob 发新帖 → Alice home 不再显示 ✓
```
