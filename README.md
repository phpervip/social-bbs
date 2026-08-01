# B — 简化版 社交平台（微服务学习项目）

Logo：**B** · 咖啡棕 #8B5A2B 品牌色 · 极简黑白中性底

架构设计：`.superpowers/brainstorm/IMPLEMENTATION_HANDOFF.md`（含全部设计图索引）
当前阶段：**P2 — 账号体系 + 关注 + JWT 鉴权 + Kafka/outbox 事件化**

## P2 组件

| 组件 | 技术 | 目录 | 端口 |
|---|---|---|---|
| Shell (MF Host) | React 18 + Webpack 5 MF | `frontend/shell/` | 3000 |
| Feed Remote (MF Remote) | React 18 + Webpack 5 MF | `frontend/feed-remote/` | 3001 |
| **User Remote (MF Remote)** | **React 18 + Webpack 5 MF** | **`frontend/user-remote/`** | **3002** |
| API Gateway | Node.js + Fastify | `services/gateway/` | 8080 |
| Feed Service | Go + gRPC + GORM | `services/feed-service/` | 9000 |
| **User Service** | **Java 21 + Spring Boot 3 + gRPC** | **`services/user-service/`** | **9001** |
| MySQL 8 / Redis 7 / Kafka (KRaft) | docker-compose | `infra/` | 3306 / 6379 / **9092** |

请求链路：`Browser → http://localhost:3000 → /api (proxy) → Gateway:8080 → gRPC → Feed Service:9000 / User Service:9001 → MySQL/Redis/Kafka`

## 快速启动（详见 `infra/README.md`）

```powershell
# 1. 基础设施（MySQL + Redis + Kafka）
cd infra; docker compose up -d

# 2. User Service（Java）
cd ../services/user-service; .\mvnw -q spring-boot:run

# 3. Feed Service（Go）
cd ../services/feed-service; go run ./cmd/server

# 4. Gateway（Node）
cd ../services/gateway; npm install; npm run dev

# 5. 前端（三个终端）
cd ../../frontend/shell; npm install; npm run dev          # :3000
cd ../../frontend/feed-remote; npm install; npm run dev    # :3001
cd ../../frontend/user-remote; npm install; npm run dev    # :3002

# 6. 浏览器打开 http://localhost:3000 → 注册/登录 → 发帖/关注
```

## 阶段说明（计划 B 纵向切片）

- P1 ✅ 发帖 → 首页时间线
- P2 🚧 账号体系 + 关注 + JWT 鉴权 + Kafka/outbox 事件化（本阶段）
- P3 Video Service + S3 分片 + FFmpeg HLS
- P4 Kafka 事件化 + Notification + ES Search
