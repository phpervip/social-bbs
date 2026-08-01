# P2 设计文档：账号体系 + 关注 + JWT 鉴权 + Kafka/outbox 事件化

> 阶段：P2（承接 P1，交接入口见 `P2-HANDOFF.md`）
> 依据：`.superpowers/brainstorm/IMPLEMENTATION_HANDOFF.md`（已批准设计总纲）+ 设计图（user-service.html / feed-service.html / gateway-design.html / mf-architecture-v2.html）
> 状态：**设计定稿**，经脑暴拍板 4 个开放决策点。下一步写实施计划（对齐 P1 `plan.md` 结构）。

---

## 1. 背景与范围

P1「发帖 → 首页时间线」已端到端交付。P2 完成两件大事：

1. **账号体系**：注册 / 登录 / 关注 / 取关 + 正式 JWT 鉴权（替换 P1 的 dev-only 登录）
2. **Feed 回填 outbox + Kafka 事件化**：发帖走 Kafka 事件（替换 P1 进程内 Fanout）

**完成标志**（来自 IMPLEMENTATION_HANDOFF §4）：账号体系可用，发帖走 Kafka 事件。

**范围边界**（P2 不做）：
- 不迁移 Kind/K8s（D1 留 P2+）
- 不做大 V Pull 模式（阈值 stub 保留，P4 启用）
- 不消费 `user:registered` 事件（新用户懒重建已覆盖）
- 个人主页不做帖子列表（feed.proto 冻结，需新 RPC，留 P4）
- 不做点赞精确计数校准 / ES 搜索（D6/D7 留 P4）

---

## 2. 决策记录（脑暴拍板，2026-08-01）

| # | 决策点 | 结论 | 理由 |
|---|---|---|---|
| D-A1 | User Service 技术栈 | **Java 21 + Spring Boot 3** | 忠实已批准设计图（user-service.html 明写 Java），演示第二种主流微服务语言 |
| D-A2 | 前端形态 | **新建 User Remote（:3002）全量** | 最贴近设计稿 Shell+3 Remote；承载注册/登录/个人主页/关注；Shell 删 dev 登录 |
| D-A3 | Kafka/outbox 深度 | **outbox + 双 worker**（常驻 dispatcher + 定时补偿） | 可靠性优先；P1 已建 outbox_events 表应利用；直发 Kafka 丢可靠性 |
| D-A4 | 关注对 Timeline | **真实粉丝 Push + 关注回填/取关清理**；保持 Push-only | 本期无大 V 场景；Pull 分支（阈值 stub）留 P4 接入真实粉丝数后启用 |

**附加边界决策**（脑暴中识别）：
- D-A5：关注/取关按钮放**个人主页**（User Remote 内）。Feed PostCard 作者名 → 链接跳 `/profile/:id`。遵守 MF「Remote 间禁止直接依赖」。
- D-A6：JWT 由 User Service 签发（HS256，`JWT_SECRET` 环境变量），**Gateway 只校验**（中间件已真实实现，仅换签发源 + 加黑名单检查）。

---

## 3. 总体架构

### 3.1 组件与端口

| 组件 | 技术 | 目录 | 端口 | 状态 |
|---|---|---|---|---|
| Shell (MF Host) | React 18 + Webpack 5 MF | `frontend/shell/` | 3000 | 改 |
| Feed Remote | React 18 + Webpack 5 MF | `frontend/feed-remote/` | 3001 | 改 |
| **User Remote** | React 18 + Webpack 5 MF | `frontend/user-remote/` | **3002** | **新** |
| API Gateway | Node + Fastify + gRPC | `services/gateway/` | 8080 | 改 |
| Feed Service | Go 1.26 + gRPC + GORM | `services/feed-service/` | 9000 | 改 |
| **User Service** | **Java 21 + Spring Boot 3** | `services/user-service/` | **9001 (gRPC)** | **新** |
| MySQL 8 / Redis 7 | docker-compose | `infra/` | 3306 / 6379 | 改（+user_db） |
| **Kafka** | **apache/kafka KRaft 单节点** | `infra/` | **9092** | **新** |

请求链路：`Browser → :3000 → /api (proxy) → Gateway:8080 → gRPC → Feed:9000 / User:9001 → MySQL/Redis/Kafka`

### 3.2 事件拓扑（P2 定稿）

| topic | 生产方 | 消费方/组 | P2 动作 |
|---|---|---|---|
| `post:created` | Feed（outbox→Kafka） | Feed 自身 fanout 组 `feed-fanout` | 真实粉丝写扩散 |
| `user:follow-changed` | User Service | Feed timeline 组 `feed-timeline` | 关注回填 / 取关清理 |
| `user:registered` | User Service | —（P2 不消费） | 只生产，懒重建覆盖 |

---

## 4. User Service 规格（Java 21 + Spring Boot 3）

### 4.1 工程与依赖

- 目录 `services/user-service/`；Maven 工程，仓库提交 `mvnw`（Maven Wrapper 自举，本机无需预装 Maven；JDK 21 Temurin 已确认安装）
- 依赖：`spring-boot-starter`、`grpc-server-spring-boot-starter`（net.devh）、`mybatis-plus-boot-starter`、`spring-boot-starter-data-redis`、`spring-kafka`、`jjwt`（或 `java-jwt`）、`spring-security-crypto`（仅 BCrypt，不引入完整 Spring Security）
- 三层：Controller(gRPC) → Service → Repository；与设计图一致
- 配置 env：`USER_GRPC_PORT`(:9001) / `USER_DB_DSN`(user_db) / `USER_REDIS_ADDR` / `KAFKA_BOOTSTRAP`(localhost:9092) / `JWT_SECRET`(与 Gateway 共享) / `JWT_TTL_SECONDS`(24h)

### 4.2 gRPC 契约（`proto/user.proto`，新增冻结契约）

服务 `user.v1.UserService`：

| RPC | 说明 |
|---|---|
| Register | username/email/password/display_name → 建用户 + 签发 JWT + 写 session |
| Login | username 或 email + password → 校验 BCrypt → 签发 JWT + 写 session |
| Logout | jti → 置 session revoked + 写 Redis 黑名单 |
| GetProfile | 返回用户资料（含 follower_count/following_count；post_count 由 Feed 侧展示，不在 User Service 计算） |
| UpdateProfile | bio/avatar_url/display_name |
| Follow | 幂等关注（联合唯一索引 + INSERT IGNORE 语义） |
| Unfollow | 幂等取关 |
| GetFollowers | 粉丝列表（游标分页） |
| GetFollowing | 关注列表（游标分页） |

错误码沿用契约约定：`INVALID_ARGUMENT→400`、`ALREADY_EXISTS→409`、`UNAUTHENTICATED→401`、`NOT_FOUND→404`、`INTERNAL→500`。

### 4.3 数据库 user_db

```sql
users:        id PK · username VARCHAR(64) UNIQUE · email VARCHAR(255) UNIQUE
              · password_hash VARCHAR(100) · bio VARCHAR(255) DEFAULT ''
              · avatar_url VARCHAR(255) DEFAULT '' · follower_count INT DEFAULT 0
              · following_count INT DEFAULT 0 · created_at DATETIME(3) · updated_at DATETIME(3)
follows:      follower_id BIGINT + followee_id BIGINT 联合 PK · FK→users.id
              · created_at DATETIME(3)   -- 联合唯一 → 关注天然幂等
user_sessions: token_id VARCHAR(64) PK(=JWT jti) · user_id BIGINT · expires_at DATETIME(3)
              · revoked TINYINT(1) DEFAULT 0   -- 多端登录 / 主动下线
```

- 种子用户（迁移自 feed_db 种子，密码统一 BCrypt(`Password123!`)）：bob / alice / carol / dave
- 演示环境物理外键；注释注明生产移除

### 4.4 Redis 键（User Service 维护）

| key | 类型 | TTL | 说明 |
|---|---|---|---|
| `user:profile:{id}` | String JSON | 10min | 热点资料；更新库后 DEL |
| `user:followers:{id}` | ZSet (score=follow created_at ms) | 5min | 粉丝列表；ZRange 分页 |
| `user:following:{id}` | ZSet | 5min | 关注列表；ZRange 分页 |
| `auth:blacklist:{jti}` | String | = JWT 剩余有效期 | 登出黑名单；与 Gateway 共享 |

缓存一致性：**更新数据库 → 删除缓存**，TTL 兜底。

### 4.5 关注流程（事务 + 事件）

```
FollowService: tx { 写 follows + 更新 follower_count/following_count } → COMMIT
→ @TransactionalEventListener(AFTER_COMMIT) → 更新 Redis ZSet → 发 Kafka user:follow-changed
→ 失败 → 本地消息表（user_outbox）兜底补偿重发
```

### 4.6 密码与 JWT

- 密码：BCrypt（`BCryptPasswordEncoder`，strength 10）
- JWT：HS256 签发，payload `{sub: String(user_id), username, displayName, jti, iat, exp}`；`jti` = UUID，同步写 `user_sessions`
- 登录/注册成功返回 `{token, expires_in, user:{...}}`

---

## 5. Feed Service 改造（Go）

### 5.1 CreatePost 事务改造

- `internal/service/post_service.go`：`posts.Create` 的 tx 内**同时插入 outbox_events**（`topic=post:created`，payload `{post_id, user_id, content, created_at}`，`status='pending'`）
- **移除** `fanout.Enqueue` 调用（fanout.go 进程内 channel 消费者 → 改为 Kafka 消费者）

### 5.2 outbox 投递双 worker（`internal/worker/` 改造）

| worker | 行为 |
|---|---|
| **Dispatcher（常驻 goroutine）** | 轮询 `outbox WHERE status='pending' ORDER BY id LIMIT N` → Kafka 发布 `post:created` → ack 后置 `delivered`；失败 → `retry_count+1`，≥3 → `failed` |
| **Compensation（定时 5s ticker）** | 扫描超时未投递的 pending / failed → 重投（上限 3 次，超限保持 failed 记日志） |

- Kafka 客户端：**`segmentio/kafka-go`**（纯 Go、无 cgo，Windows 零摩擦）—— 否决 confluent-kafka-go（需 cgo/librdkafka）

### 5.3 Fanout 改造（真实粉丝 Push）

- `StubFanoutMode`（恒 PUSH 全用户）→ **`RealFollowersMode`**：
  - 消费 `post:created`（consumer group `feed-fanout`）
  - 读作者粉丝列表 `user:followers:{author_id}` ZSet（User Service 维护）
  - ZADD post_id → 每个粉丝 + 作者自己的 `feed:home:{uid}`
  - **本期 Push-only**：Fanout 无条件推送给所有真实粉丝，不做阈值分支；大 V Pull 分支（粉丝数 > 1000 走 `feed:inbox` 读时合并）逻辑留 P4，仅保留阈值常量 stub
- `feed:inbox:{author_id}` 键路径保留不启用

### 5.4 消费 user:follow-changed（`internal/worker/` 新增 consumer）

- **Follow**：拉 followee 近期帖子（新增 `LatestByAuthor` 查询，近期 50 条）→ 回填进 follower `feed:home`
- **Unfollow**：从 follower `feed:home` ZREM 掉 followee 的帖子
- payload：`{follower_id, followee_id, action: follow|unfollow, created_at}`

### 5.5 作者信息来源切换（D3 衔接）

- **移除 `feed_db.users` 种子表**；Feed 不再 join 本地 users 表
- 时间线/帖子渲染作者信息：`MGET user:profile:{id}`（User Service 维护的 Redis 缓存）→ miss 时批量 gRPC `GetProfile` 回填
- 时间线**重建兜底**（cache miss）：改为拉取**关注者的**近期帖子（`user:following:{uid}` ZSet + `LatestByAuthors`），而非 P1 的全站最新 50 条

### 5.6 需要的 repository 新增

- `OutboxRepo`：`CreateInTx` / `ClaimPending(limit)` / `MarkDelivered` / `IncrementRetry` / `MarkFailed`
- `PostRepo.LatestByAuthor(authorID, cursor, limit)` / `LatestByAuthors(ids, limit)`
- `UserClient`（gRPC → User Service）：`GetProfile(id)`（带 Redis 缓存回填）、批量版

---

## 6. Gateway 改造（Node/Fastify）

### 6.1 路由集（替换 dev 登录）

| Method | Path | gRPC 目标 | 鉴权 |
|---|---|---|---|
| POST | /api/auth/register | User.Register | 公开 |
| POST | /api/auth/login | User.Login | 公开 |
| POST | /api/auth/logout | User.Logout | 受保护（读 request.user.jti） |
| GET | /api/user/:id | User.GetProfile | 受保护 |
| PUT | /api/user/profile | User.UpdateProfile | 受保护 |
| POST | /api/user/:id/follow | User.Follow | 受保护（发布类限流） |
| DELETE | /api/user/:id/follow | User.Unfollow | 受保护（发布类限流） |
| GET | /api/user/:id/followers | User.GetFollowers | 受保护 |
| GET | /api/user/:id/following | User.GetFollowing | 受保护 |
| GET | /api/feed/* | Feed（现有） | 受保护（不变） |
| GET | /healthz | - | 公开 |

- **删除** `routes/dev.js`（dev 登录/用户清单）；`middleware/auth.js` 公开前缀 `['/api/dev','/healthz']` → `['/api/auth/register','/api/auth/login','/healthz']`
- JWT 校验后**新增黑名单检查**：`GET auth:blacklist:{jti}` 存在 → 401
- `config.js`：+`GW_USER_ADDR=localhost:9001`；corsOrigins + `http://localhost:3002`

### 6.2 新增 gRPC 客户端

- `src/grpc/user.js`（仿 feed.js：懒连接 + waitForReady 重连 + breaker 包裹）
- `src/routes/auth.js`、`src/routes/user.js`

---

## 7. 前端改造

### 7.1 User Remote（新建，`frontend/user-remote/`，:3002）

- webpack：`name:'user'`，`exposes: { './Auth': ..., './Profile': ... }`，`devServer.port: 3002`，CORS 头 `*`
- shared 同 Feed Remote：react/react-dom/react-router-dom/axios `{singleton:true}` + `'@b/shared': { singleton:true, import:false }`
- **组件**：
  - `Auth`：登录/注册双表单（用户名或邮箱 + 密码；注册含 display_name/email）；成功 → `api.login/register` → 存 token → `bus.emit('auth:login')` → 跳 `/home`
  - `Profile`：个人主页 — 头像/display_name/@username/bio + follower/following 计数 + 关注/取关按钮（非本人）+ 粉丝/关注列表 Tab
- 全部经 `@b/shared` 的 `{api, bus, ui}`

### 7.2 Shell（改）

- webpack `remotes` + `user: 'user@http://localhost:3002/remoteEntry.js'`
- 路由：`/login /register → User.Auth`；`/profile/:id → User.Profile`；「我的」导航启用 → `/profile/:id`（当前用户）
- `src/pages/Login.jsx` 移除（或改为 User.Auth 挂载点）
- `shared/api-client.js`：+`register/login/logout/getProfile/updateProfile/follow/unfollow/getFollowers/getFollowing`；**移除 devLogin/devUsers**；401 拦截逻辑保留
- `shared/event-bus.js`：+`profile:updated` 事件

### 7.3 Feed Remote（改）

- PostCard 作者名/头像 → `<Link to="/profile/:id">`（跨 Remote 只跳路由，不直接依赖）
- 时间线语义不变（关注者流）

---

## 8. Infra 改造

- `infra/docker-compose.yml`：+`kafka`（apache/kafka 镜像，KRaft 单节点：`KAFKA_PROCESS_ROLES=broker,controller`、`KAFKA_NODE_ID=1`、`KAFKA_CONTROLLER_QUORUM_VOTERS=1@kafka:9093`、listeners PLAINTEXT:9092 + CONTROLLER:9093；端口 9092）
- `infra/mysql/init/02-user.sql`：user_db schema + 种子 4 用户（BCrypt `Password123!`）
- `infra/README.md`：更新启动步骤（compose up 含 Kafka → user-service → feed-service → gateway → shell → user-remote → feed-remote）
- `infra/demo-e2e.ps1`：扩展（见 §9）

---

## 9. 测试与验收

### 9.1 单元/集成测试

| 面 | 覆盖 |
|---|---|
| User Service (JUnit) | 注册重名 409、登录密码错误 401、BCrypt 哈希、Follow 幂等（重复关注不炸）、JWT 签发/校验往返 |
| Feed (Go) | outbox 写入（sqlmock）、dispatcher 投递+重试上限、compensation 扫描、RealFollowers fanout（mock follower ZSet）、follow-changed 回填/清理（sqlmock + redis mock）、LatestByAuthor |
| Gateway (node:test) | 公开前缀变更、黑名单 401、错误码映射（含 user 服务）、新路由转发 |
| 前端 | 三仓 `npm run build` 全过 |

### 9.2 端到端（`infra/demo-e2e.ps1` 扩展）

1. `docker compose up` 后 MySQL/Redis/Kafka 健康
2. 注册新用户 → 登录 → 发帖
3. **非粉丝**查时间线 → **不见**该帖；**关注作者后** → 时间线出现（回填）
4. 作者再发一帖 → 粉丝时间线即时出现（Kafka fanout）
5. 取关 → 时间线清理该作者帖子
6. 登出 → 用旧 token 请求 → 401（黑名单生效）
7. `go vet` / `go build` / `mvn test` / gateway `npm test` / 前端三仓 build 全过

### 9.3 完成标志

1. 注册/登录/登出/关注/取关端到端可用（浏览器实测）
2. **发帖走 Kafka 事件**：outbox → Kafka `post:created` → fanout → 粉丝时间线；非粉丝不可见
3. 关注回填近期帖子、取关清理时间线
4. 全部测试通过 + e2e 全 PASS
5. 设计偏差记录于本文档 §2/§10，评审知悉

---

## 10. 偏差记录（对照 IMPLEMENTATION_HANDOFF 已批准设计）

| # | 设计原案 | P2 实际 | 原因/衔接 |
|---|---|---|---|
| D-A1 | User Service Java Spring Boot | **与设计一致**（选用 Java） | 忠实设计图 |
| D-A2 | Shell+3 Remote | **新增 User Remote**（设计第 2 个 Remote） | 一致 |
| D-A3 | outbox + 定时补偿 | **双 worker**（常驻 dispatcher + 补偿） | 更可靠，延迟更低 |
| D-A4 | 混合 Push/Pull | **Push-only + 阈值 stub** | 无大 V 场景；P4 启用 Pull |
| D-A5 | 关注按钮位置未定 | 个人主页内（User Remote） | MF 禁止跨 Remote 直接依赖 |
| D-A6 | 个人主页含帖子列表 | **本期不含** | feed.proto 冻结；P4 新增 RPC 后补 |
| D-A7 | user:registered 消费初始化时间线 | **本期不消费** | 新用户懒重建已覆盖；P4 Notification 再用 |
| D-A8 | feed_db.users 种子表 | **移除**，作者信息走 User Service gRPC + Redis 缓存 | D3 正式衔接 |

---

## 11. 参考

- `P2-HANDOFF.md`（交接入口）
- `.superpowers/brainstorm/IMPLEMENTATION_HANDOFF.md` §3.4 / §4
- `.superpowers/brainstorm/content/user-service.html` / `feed-service.html` / `gateway-design.html` / `mf-architecture-v2.html`
- `.superpowers/sdd/p1-feed-loop/plan.md`（P1 规格，outbox_events 表结构 §3.1，Redis 键 §3.2）
