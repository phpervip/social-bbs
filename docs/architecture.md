# B 社交平台 · P1 架构说明

> 对应阶段：**P1 — 发帖 → 首页时间线 端到端闭环**（已交付）
> 依据：根 `README.md`、`.superpowers/brainstorm/IMPLEMENTATION_HANDOFF.md`（已批准设计总纲）、`.superpowers/sdd/p1-feed-loop/plan.md`（唯一需求源）
> 品牌色：咖啡棕 `#8B5A2B`（brand）· 焦糖 `#C19A6B`（brand-2），黑白中性底

## 1. 项目定位

「B」是一个简化版社交平台微服务学习项目。全量设计是微博 + 私信 + 短视频的混合体，本项目只做其中 20% 的核心逻辑，按纵向切片分阶段交付。P1 只交付一条最关键的链路：**发帖 → 首页时间线**。

目标定位是学习与演示：真实的多语言微服务（React / Node / Go / MySQL / Redis），代码结构清晰、可本地一键跑通，UI 走极简主义。每阶段结束必须有一次完整演示。

阶段规划：P1 发帖闭环（本阶段）→ P2 User Service + 鉴权 + 关注（回填 outbox + Kafka）→ P3 Video Service + S3 分片 + FFmpeg HLS → P4 Kafka 事件化 + Notification + ES Search。

## 2. 技术栈

| 层 | 组件 | 技术 | 目录 | 端口 |
|---|---|---|---|---|
| 前端 | Shell（MF Host） | React 18 + Webpack 5 Module Federation + react-router-dom v6 + axios | `frontend/shell/` | 3000 |
| 前端 | Feed Remote（MF Remote） | React 18 + Webpack 5 Module Federation | `frontend/feed-remote/` | 3001 |
| 网关 | API Gateway | Node.js + Fastify + @grpc/grpc-js + @grpc/proto-loader + ioredis + jsonwebtoken | `services/gateway/` | 8080 |
| 服务 | Feed Service | Go 1.26 + gRPC + GORM + go-redis/v9 | `services/feed-service/` | 9000 |
| 数据 | MySQL 8 / Redis 7 | docker-compose（`infra/mysql/init/01-feed.sql` 自动建库） | `infra/` | 3306 / 6379 |
| 契约 | feed.proto | proto3，`feed.v1.FeedService`，已冻结 | `proto/feed.proto` | - |

Feed Service gRPC 方法（`proto/feed.proto`）：`CreatePost / GetPost / GetHomeTimeline / DeletePost / LikePost / UnlikePost / AddComment / GetComments / Search`。Gateway 与 Feed Service 各自从同一份 proto 生成 stubs。

## 3. 请求链路

一条帖子从浏览器到数据库的完整路径：

```
Browser
  │  打开 http://localhost:3000
  ▼
Shell :3000（MF Host，React 18）
  │  devServer proxy：/api → http://localhost:8080
  ▼
API Gateway :8080（Node + Fastify）
  │  五层中间件：前置 → 鉴权(JWT) → 限流 → 转发(熔断) → 后置
  │  @grpc/grpc-js 懒连接 + 断线重连（waitForReady）
  ▼
Feed Service :9000（Go 1.26 + gRPC）
  │  Handler → Service → Repository（GORM + go-redis/v9）
  ├───────► MySQL 8 :3306（feed_db，5 张表）
  └───────► Redis 7 :6379（feed:home / post:detail / post:likes / feed:lock）
```

前端请求一律收敛到 Gateway，不直连任何后端服务。服务间通信统一走 gRPC（内网），对外只暴露 REST/HTTP。

## 4. 组件职责

| 组件 | 职责 |
|---|---|
| Shell（MF Host） | 统一宿主与路由；dev 登录页；持久化 JWT；共享 API Client；`/api` 代理到 Gateway:8080 |
| Feed Remote | 时间线列表、发帖表单、点赞、评论等 Feed 相关 UI（独立 MF Remote，宿主按需加载） |
| API Gateway | REST 对外；JWT 校验（HS256，`JWT_SECRET`）；Redis 固定窗口限流；REST → gRPC 转发；每服务独立熔断器；统一响应 `{code, message, data}` |
| Feed Service | 帖子 CRUD、软删除、点赞/评论、时间线读写缓存、进程内 Fanout Worker、游标分页 |
| infra | docker-compose 起 MySQL 8 + Redis 7；首次启动自动执行 init SQL 建库 + 种子用户 |

Gateway 五层中间件（顺序即设计）：前置（request-id、日志、CORS、body 限制）→ 鉴权（公开路由表 `/api/dev/*`、`/healthz` 跳过）→ 限流（匿名 30r/min、已登录 100r/min、发布类 10r/min）→ 转发（REST→gRPC 映射 + 熔断）→ 后置（统一响应与错误码映射）。

## 5. 数据模型（feed_db，5 张表）

初始化 SQL 见 `infra/mysql/init/01-feed.sql`（首次容器启动自动执行）。

| 表 | 关键字段 | 说明 |
|---|---|---|
| `users` | id PK · username UNIQUE · display_name · avatar_url · created_at | P1 无 User Service，内置 4 个种子用户：bob(1)/alice(2)/carol(3)/dave(4) |
| `posts` | id PK · user_id · content TEXT · media_url · like_count · comment_count · created_at · deleted_at | 软删除通过 `deleted_at`（NULL = 未删除） |
| `post_likes` | (post_id, user_id) 联合 PK · created_at | 物理外键 → posts.id（演示环境允许）；联合 PK 保证点赞幂等 |
| `post_comments` | id PK · post_id · user_id · content VARCHAR(500) · created_at | 按 post_id / created_at 索引 |
| `outbox_events` | id PK · topic · payload JSON · status ENUM(pending/delivered/failed) · retry_count · created_at | **P1 只建表不写入**，P2 回填启用（Kafka 投递发件箱） |

## 6. Redis 键设计

| 键 | 类型 | 行为 |
|---|---|---|
| `feed:home:{user_id}` | ZSet | score = created_at(ms)，member = post_id；TTL 7d 滑动续期；上限 500 条，超限 ZREMRANGEBYRANK 淘汰最旧 |
| `post:detail:{id}` | String（JSON） | TTL 30min；帖子更新/删除后 DEL |
| `post:likes:{id}` | String（计数） | TTL 30min |
| `feed:lock:{user_id}` | SETNX | TTL 5s；时间线重建锁，拿不到锁则直接回源 MySQL 不重建 |

时间线读取路径：`feed:home:{uid}` → ZREVRANGEBYSCORE（游标）→ MGET `post:detail` 缓存，miss 的批量回源 MySQL，**已删除/不存在的帖子读时直接丢弃**（服务端过滤）。ZSet 未命中（不存在或空）时重建：抢 `feed:lock` → 取 MySQL 最近 50 条全站帖子回填 + EXPIRE 7d → 返回。

## 7. 关键机制

**Fanout 扇出（发帖异步扩散）**
发帖先同步响应客户端，Fanout 由进程内异步 Worker 执行：channel 队列（cap 1024）+ 单消费者 goroutine，消费 `(post_id, user_id, created_at)`。P1 无关注图，阈值 stub 恒 PUSH，即对 users 表**所有**用户（含作者自己）ZADD 进各自 `feed:home`，每次 ZADD 后 EXPIRE 7d，上限 500 超限淘汰；队列满时同步兜底执行。**P2 改造为 outbox_events 回填 + Kafka 投递**。

**软删除**
`DeletePost` 仅作者可删（否则 PERMISSION_DENIED），GORM 软删除置 `deleted_at`，删 `post:detail` 缓存但**不清时间线**（前端读取时服务端二次过滤）。

**游标分页**
统一用 `CursorPage{cursor, limit}`：cursor = 上页最后一条 created_at（unix ms），0 = 第一页（新→旧）；limit 默认 20、最大 50。时间线 / 评论 / 搜索共用。

**like_count 精确计数**
`posts.like_count` 在事务内同步更新（Like 用 insert ignore 幂等，Unlike 保证 ≥0 不回负），成功后刷新 `post:likes` 缓存并 DEL `post:detail`。设计允许近似值，P1 直接做精确。

## 8. 设计偏差（D1-D7）与 P2 衔接

以下为 P1 相对全量设计的简化决策，全部记录在 `plan.md §1.1`，P2 衔接点见 `P2-HANDOFF.md §2.1`：

| # | P1 简化 | P2 动作 |
|---|---|---|
| D1 | compose 部署（MySQL/Redis 容器，应用本地进程） | P2+ 上 Kind 时迁移 StatefulSet/Deployment（非本期必须） |
| D2 | 无 Kafka，Fanout 用进程内 goroutine + channel | **回填 `outbox_events` + Kafka 投递**（P2 重点） |
| D3 | 无 User Service，feed_db 内置 users 种子表 | User Service 替换种子表（独立 user_db） |
| D4 | Gateway dev-only JWT（HS256 临时签发） | 换 User Service 签发（正式注册/登录） |
| D5 | Timeline Push-only 全局扇出（阈值 stub 恒 PUSH） | 接入真实粉丝数后启用混合模式（粉丝 ≤1000 Push / 大 V Pull） |
| D6 | 点赞精确计数（设计允许近似） | 留到 P4 异步回写校准 |
| D7 | MySQL LIKE 搜索（设计 L3 降级层） | 留到 P4 换 ES |

P2 范围（来自已批准的 `IMPLEMENTATION_HANDOFF.md §4`）：**User Service + 注册/登录/关注 + JWT 鉴权；Feed 回填 outbox 事件化**。完成标志：账号体系可用，发帖走 Kafka 事件。

---

相关文档：`docs/acceptance.md`（P1 验收记录）· `docs/demo-guide.md`（启动与演示手册）· 根 `README.md` · `P2-HANDOFF.md`（P1 → P2 交接入口）
