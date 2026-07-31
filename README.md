# B — 简化版 社交平台（微服务学习项目）

Logo：**B** · 咖啡棕 #8B5A2B 品牌色 · 极简黑白中性底

架构设计：`.superpowers/brainstorm/IMPLEMENTATION_HANDOFF.md`（含全部设计图索引）
当前阶段：**P1 — 发帖 → 首页时间线 端到端闭环**（Shell + Feed Remote + Gateway + Feed Service + MySQL/Redis）

## P1 组件

| 组件 | 技术 | 目录 | 端口 |
|---|---|---|---|
| Shell (MF Host) | React 18 + Webpack 5 MF | `frontend/shell/` | 3000 |
| Feed Remote (MF Remote) | React 18 + Webpack 5 MF | `frontend/feed-remote/` | 3001 |
| API Gateway | Node.js + Fastify | `services/gateway/` | 8080 |
| Feed Service | Go + gRPC + GORM | `services/feed-service/` | 9000 |
| MySQL 8 / Redis 7 | docker-compose | `infra/` | 3306 / 6379 |

请求链路：`Browser → http://localhost:3000 → /api (proxy) → Gateway:8080 → gRPC → Feed Service:9000 → MySQL/Redis`

## 快速启动（详见 `infra/README.md`）

```powershell
# 1. 基础设施（MySQL + Redis）
cd infra; docker compose up -d

# 2. Feed Service（Go）
cd ../services/feed-service; go run ./cmd/server

# 3. Gateway（Node）
cd ../services/gateway; npm install; npm run dev

# 4. 前端（两个终端）
cd ../../frontend/shell; npm install; npm run dev      # :3000
cd ../../frontend/feed-remote; npm install; npm run dev # :3001

# 5. 浏览器打开 http://localhost:3000 → dev 登录 → 发帖
```

## 阶段说明（计划 B 纵向切片）

- P1 ✅ 发帖 → 首页时间线（本阶段）
- P2 User Service + 鉴权 + 关注（回填 outbox + Kafka）
- P3 Video Service + S3 分片 + FFmpeg HLS
- P4 Kafka 事件化 + Notification + ES Search
