# P2 交接文档 — User Service + 鉴权 + 关注 + Kafka/outbox

> 本文档是 **P1 → P2 的交接入口**。新开 opencode 会话做 P2 时，先读本文件，再读下方「参考资料索引」中的设计文档。
> 最后更新：P1 交付当日（HEAD `8ca6ae5`）；**2026-08-02 设计评审拍板 R1–R8 修正已纳入 `docs/superpowers/specs/2026-08-01-p2-account-kafka-design.md` §10.1，与下文冲突处以下方为准**。

---

## 0. 一句话

P1「发帖 → 首页时间线」已端到端交付并推 GitHub；P2 要做 **账号体系（注册/登录/关注 + 正式 JWT 鉴权）**，并把 Feed 的进程内 Fanout 升级为 **outbox + Kafka 事件化**。

---

## 1. P1 现状快照（新会话必须知道的基线）

### 1.1 交付状态
- **远程**：`https://github.com/phpervip/social-bbs`（origin 已配置，推 `main` + `feat/p1-feed-loop` 两分支，均指向 `8ca6ae5`）
- **分支**：本地 `main` 与 `feat/p1-feed-loop` 同步；P2 建议从 `main` 切新分支
- **验收证据**（全部 PASS）：
  - `infra/demo-e2e.ps1` → 8/8 PASS（healthz/登录/发帖/时间线/点赞/评论/跨账号扇出/删除）
  - 浏览器实测：登录 → 发帖 → 时间线即时出现 → 点赞 ♥ → 评论 → 删除确认
  - `go vet`/`go build`/`go test` 19/19、gateway `npm test` 41/41、前端双仓 `npm run build` 全过
  - T8 终审 4/4 完成标志 PASS（报告在 `D:\Personal\Temp\opencode\t8-review.md`，非仓库内）

### 1.2 组件与端口（P1 定稿）
| 组件 | 技术 | 目录 | 端口 |
|---|---|---|---|
| Shell (MF Host) | React 18 + Webpack 5 MF | `frontend/shell/` | 3000 |
| Feed Remote | React 18 + Webpack 5 MF | `frontend/feed-remote/` | 3001 |
| API Gateway | Node + Fastify + @grpc/grpc-js | `services/gateway/` | 8080 |
| Feed Service | Go 1.26 + gRPC + GORM + go-redis/v9 | `services/feed-service/` | 9000 |
| MySQL 8 / Redis 7 | docker-compose | `infra/` | 3306 / 6379 |

请求链路：`Browser → :3000 → /api (proxy) → Gateway:8080 → gRPC → Feed:9000 → MySQL/Redis`

### 1.3 数据库与 Redis（P1 现状）
- **feed_db**（MySQL）：`users`（4 种子用户 bob/alice/carol/dave）、`posts`、`post_likes`、`post_comments`、`outbox_events`（**P1 只建表不启用**）
- **Redis 键**：`feed:home:{uid}` ZSet（7d TTL、上限 500）、`post:detail:{id}`、`post:likes:{id}`、`feed:lock:{uid}`
- 完整键名/行为见 `plan.md §3.2/§3.3`（P2 改造 Feed 时以此为准）

### 1.4 P1 踩过的坑（P2 别重犯，全在 git 历史里）
| commit | 问题 | 修复 |
|---|---|---|
| `66b50e4` | Gateway gRPC `deadline` 在模块加载时求值，运行后必然过期 → 503 | 改函数每次调用求值 |
| `66b50e4` | GORM 把 `PostRow` 复数化成 `post_rows`，读写表分离 | 加 `TableName() → "posts"` |
| `66b50e4` | MF shared 缺 `eager: true` → "Shared module not available" | 补 eager |
| `66b50e4` | `ui.js` 用 React 钩子未 import → "React is not defined" | 补 import |
| `66b50e4` | MF 插件未指定 `filename` → 容器 chunk 落 `feed.js`，宿主请求 remoteEntry.js 404 | `filename: 'remoteEntry.js'` |
| `8ca6ae5` | **三个 MF 暴露组件没 import `styles.css`** → 所有控件浏览器默认尺寸 | 各组件 `import './styles.css'` |

> ⚠️ **重要教训**：MF Remote 的每个暴露组件必须自带 `import './styles.css'`（style-loader 幂等），否则样式只在独立 dev 入口生效、宿主里全部退化。

---

## 2. P2 范围与完成标志

来源：`IMPLEMENTATION_HANDOFF.md §4`（已批准，不得随意改范围）：

> **P2 = User Service + 注册/登录/关注 + JWT 鉴权；Feed 回填 outbox 事件化**
> 完成标志：**账号体系可用，发帖走 Kafka 事件**

### 2.1 P1 偏差（D1-D7）在 P2 的衔接点（plan.md §1.1）
| # | P1 简化 | P2 动作 |
|---|---|---|
| D2 | 无 Kafka，Fanout 用进程内 goroutine+channel | **回填 `outbox_events` + Kafka 投递**（重点） |
| D3 | 无 User Service，feed_db 内置 users 种子表 | **User Service 替换种子表**（独立 user_db） |
| D4 | Gateway dev-only JWT（HS256 4h） | **换 User Service 签发**（正式注册/登录） |
| D5 | Timeline Push-only 全局扇出（阈值 stub 恒 PUSH） | 接入真实粉丝数后启用混合模式（粉丝 ≤1000 Push / 大 V Pull） |
| D1 | compose 部署 | P2+ 迁移 Kind（非本期必须） |
| D6/D7 | 点赞精确计数 / MySQL LIKE 搜索 | 留到 P4 |

### 2.2 User Service 设计要点（来自 `user-service.html`）
- **技术**：设计图是 **Java + Spring Boot**（Controller → Service → Repository）——⚠️ **这是 P2 最大的开放决策，见 §4.1**
- **对外 gRPC**：Register / Login / GetProfile / UpdateProfile / Follow / Unfollow / GetFollowers / GetFollowing
- **Kafka 事件（生产）**：`user.registered`、`user.follow-changed`（R6：topic 用 `.` 分隔）
- **数据库 user_db（独立库）**：
  - `users`：id PK · username UNIQUE · email UNIQUE · password_hash · bio · avatar_url · created_at · updated_at
  - `follows`：(follower_id, followee_id) 联合 PK，FK→users.id，联合唯一索引幂等
  - `user_sessions`：token_id PK · user_id · expires_at · revoked（多端登录/主动下线）
- **Redis**：`user:profile:{id}` String 10min · `user:followers:{id}` / `user:following:{id}` ZSet 5min · `auth:blacklist:{jti}`（TTL=JWT 剩余有效期，与 Gateway 共享）
- **关注流程**（R1：outbox 统一模式，主路径）：本地事务写 follows + 写 user_outbox(pending) → COMMIT → 常驻 Dispatcher 异步投递 Kafka `user.follow-changed` → 失败 retry_count+1（≥3→failed），Compensation 定时重投；不再采用单纯 `@TransactionalEventListener` 直发
- **密码**：BCrypt；**JWT 签发**（换掉 D4 的 dev 方案）

### 2.3 Feed 侧 outbox 回填（P2 重点改造）
- 启用 `outbox_events` 表（P1 已建好：id · topic · payload JSON · status ENUM('pending','delivered','failed') · retry_count · created_at）
- `CreatePost` 事务内写 outbox → 定时补偿 worker 投递 Kafka → Feed 消费 `user.follow-changed` 更新关注者时间线
- 参考：`plan.md §3.3` Fanout 现行为 + `IMPLEMENTATION_HANDOFF.md §3.4` Feed 设计

---

## 3. P2 涉及的代码面（改哪里）

| 面 | 文件/目录 | 动作 |
|---|---|---|
| 新服务 | `services/user-service/`（或复用现有栈） | 新建 |
| Gateway | `services/gateway/src/`（middleware/auth.js、routes/dev.js、grpc/） | 替换 dev 登录为真实注册/登录路由 + user gRPC 转发；JWT 校验逻辑已真实实现，仅换签发源 |
| Feed | `services/feed-service/`（internal/worker、repository、handler） | outbox 写入 + Kafka 投递 + 关注事件消费 |
| 前端 | `frontend/shell/src/pages/Login.jsx`、shared/api-client.js | 登录页改注册/登录表单（dev 登录按钮移除或隐藏） |
| 前端 | 新增 User Remote？ | 设计稿是 Shell + 3 Remote（User/Feed/Video），**见 §4.2** |
| 基础设施 | `infra/docker-compose.yml` | 加 Kafka（KRaft 单节点）、user_db 初始化 |
| 契约 | `proto/` | 新增 `proto/user.proto`（feed.proto 已冻结勿动） |

---

## 4. P2 待脑暴的开放决策点（新会话第一阶段必须解决）

### 4.1 ⭐ User Service 技术栈（最高优先级）
设计图写 **Java + Spring Boot**，但 P1 实际全是 Go/Node 且全是新手向演示代码。二选一，需要脑暴拍板：
- **A. 坚持 Java/Spring Boot**：忠实设计图，但引入 JVM + Maven + MyBatis 全套新依赖，与 P1 风格割裂
- **B. 统一 Go**：与 Feed Service 同栈（gRPC + GORM + go-redis 现成模式），学习演示一致性更好，但偏离已批准设计（需记录偏差）

### 4.2 ⭐ 前端形态
- 登录页是改为真实注册/登录表单，还是保留 dev 登录 + 新增用户 Remote？
- 是否新增 **User Remote**（设计稿 Shell+3 Remote 的第二个 Remote，承载 我的/个人主页/关注列表）？还是 P2 先做 API + 最小 UI？

### 4.3 Kafka 引入方式
- KRaft 单节点 docker-compose（与 MySQL/Redis 并列）？Topic 命名 `user.registered` / `user.follow-changed` / `post.created`（R6 已定：`.` 分隔）
- outbox 补偿频率、失败重试上限（参照 Notification/Search 设计的 3 次 + dead-letter 思路）

### 4.4 关注对 Timeline 的影响
- D5 混合模式本期做到什么程度：真实粉丝数接入 Push/Pull 切换，还是先做「关注后回填近期帖子 + 取关清理」？

---

## 5. 环境与工具备忘（P1 实测，Windows）

- **Shell**：Windows PowerShell 5.1；`&&` 不可用，用 `;` 或 `if ($?)`
- **Go**：不在 PATH，用完整路径 `C:\Program Files\Go\bin\go.exe`（版本 go1.26.5）
- **浏览器验收**：`agent-browser` CLI；`eval` 传 JS 需 base64（PowerShell 无 heredoc）：
  ```powershell
  $b64 = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($js)); agent-browser eval -b $b64
  ```
- **agent-browser 注意**：`click @ref` 语法报 "Missing arguments" → 用 `agent-browser find text "..." click`
- **日志**：`D:\Personal\Temp\opencode\b-logs\{feed-svc,gateway,shell,feed-remote}.{out,err}.log`
- **截图**：`D:\Personal\Temp\opencode\b-shots`（当前模型读不了图，验收用 DOM eval 拿 innerText/几何）
- **docker CLI 噪音**：rtk/`docker` stderr 会冒 "No hook installed" 中文乱码 → `Select-String -NotMatch "Warning|No hook"` 过滤
- **服务启动**：见根 `README.md`「快速启动」5 步（compose up → feed-service → gateway → shell → feed-remote）

---

## 6. 参考资料索引（按阅读顺序）

1. **本文件** — P2 交接入口
2. `README.md`（根）— 项目定位、组件表、快速启动、阶段说明
3. `.superpowers/brainstorm/IMPLEMENTATION_HANDOFF.md` — **已批准设计总纲**（§3.4 User/Feed 决策、§4 P2 定义）
4. `.superpowers/brainstorm/content/user-service.html` — User Service 详细设计（表/Redis/Kafka/关注流程）
5. `.superpowers/brainstorm/content/feed-service.html` — Feed + Timeline（outbox/Fanout 设计）
6. `.superpowers/brainstorm/content/gateway-design.html` — 五层中间件、JWT 中间件规格
7. `.superpowers/brainstorm/content/mf-architecture-v2.html` — 前端 MF（Shell + 3 Remote）
8. `.superpowers/sdd/p1-feed-loop/plan.md` — **P1 计划（唯一需求源）**，§1.1 D1-D7、§3 Feed 规格（outbox_events 表结构）、§4 Gateway 规格
9. `.superpowers/sdd/p1-feed-loop/progress.md` — P1 台账（含 T7 修复清单）
10. 设计图预览：`node .superpowers/brainstorm/server.js`（http://localhost:52341）

---

## 7. 建议的 P2 推进方式

1. **新会话脑暴**（ce:brainstorm 或 superpowers brainstorming）：先拍 §4 的 4 个决策点
2. 产出 P2 需求文档 + 计划（对齐 P1 的 `plan.md` 结构：全局约束/规格/任务分解/完成标志）
3. SDD 派发（对齐 P1 流程：T1 控制器直做 → T2..Tn 并行派发 → 集成验证 → 终审）
4. P2 完成标志复验：注册/登录/关注可用 + 发帖走 Kafka 事件（e2e 扩展）
