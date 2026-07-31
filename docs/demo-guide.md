# B 社交平台 · P1 启动与演示手册

> 阶段：**P1 — 发帖 → 首页时间线 端到端闭环**
> 本文档面向第一次拉代码跑演示的人，Windows + PowerShell 5.1 实测通过。

## 1. 前置条件

| 项 | 要求 | 说明 |
|---|---|---|
| Docker Desktop | 已启动（`docker info` 能正常输出） | 承载 MySQL 8 + Redis 7 容器 |
| Node.js | 18+ | 跑 Gateway 与两个前端（MF） |
| Go | 1.26（本机实测 go1.26.5） | 跑 Feed Service |
| 端口 | 3306 / 6379 未被占用 | 被其他 MySQL/Redis 占用会导致 compose 起不来 |
| 端口 | 3000 / 3001 / 8080 / 9000 未被占用 | 应用端口冲突会让服务启动失败 |

## 2. 五步启动

> PowerShell 5.1 不支持 `&&`，同一窗口顺序执行用 `;` 或 `if ($?)`。推荐每步开一个终端窗口，方便看日志。

### 第 1 步：基础设施（MySQL + Redis）

```powershell
cd infra
docker compose up -d
docker compose ps        # 两个容器均应为 Up
```

首次启动会自动执行 `mysql/init/01-feed.sql`：建库 `feed_db`（5 张表）+ 写入 4 个种子用户（bob / alice / carol / dave）。init 需要几秒，就绪后再起应用：

```powershell
# 轮询直到输出 "mysqld is alive"
docker exec b-mysql mysqladmin ping -uroot -proot123 --silent
```

### 第 2 步：Feed Service（Go，端口 9000）

```powershell
cd services/feed-service
go run ./cmd/server
```

若 `go` 不在 PATH，用完整路径：

```powershell
cd services/feed-service
& "C:\Program Files\Go\bin\go.exe" run ./cmd/server
```

### 第 3 步：Gateway（Node，端口 8080）

```powershell
cd services/gateway
npm install
npm run dev
```

### 第 4 步：前端 Shell（MF Host，端口 3000）

```powershell
cd frontend/shell
npm install
npm run dev
```

### 第 5 步：前端 Feed Remote（端口 3001）

```powershell
cd frontend/feed-remote
npm install
npm run dev
```

> 顺序提醒：Shell 会从 :3001 加载 `remoteEntry.js`，**Feed Remote 要先于或与 Shell 同时启动**，否则宿主页面报 remoteEntry.js 404（见 §5）。

## 3. 浏览器演示脚本

> 📹 演示录屏（GIF，5 帧：登录 → 时间线 → 发帖 → 详情 → 评论）：[`docs/images/demo-reel.gif`](images/demo-reel.gif)

启动完成后打开 http://localhost:3000，按以下脚本走完整闭环：

| 步骤 | 操作 | 预期 |
|---|---|---|
| 1 | dev 登录选 **Alice** | 进入首页，右上角显示 Alice |
| 2 | 发一条帖子（如「大家好，我是 Alice」） | 发布成功 |
| 3 | 看首页时间线 | 刚发的帖子**即时出现**（Fanout 异步扇出到 feed:home） |
| 4 | 点帖子的点赞（♥） | like_count +1，心形高亮 |
| 5 | 点评论，输入内容并提交 | 评论区出现该评论，comment_count +1 |
| 6 | 删除刚发的帖子，确认 | 帖子从时间线消失（软删除，读时过滤） |
| 7 | 退出，dev 登录换 **Bob** | Alice 之前发的帖子出现在 Bob 时间线（**跨账号扇出**验证） |

要点：扇出是全局的，P1 无关注图，所有种子账号都能看到新帖。想快速自检可以跑自动化脚本替代手工操作：

```powershell
cd infra
./demo-e2e.ps1
```

脚本覆盖 healthz / 登录 / 发帖 / 时间线 / 点赞 / 评论 / 跨账号扇出 / 删除 8 个用例，全部通过退出码为 0。

## 4. 常见问题

**`go` 不在 PATH**
本机 Go 装在 `C:\Program Files\Go\bin\go.exe`。方案一：每次用完整路径调用（见 §2 第 2 步）；方案二：临时加入 PATH：

```powershell
$env:Path = "C:\Program Files\Go\bin;$env:Path"
```

**agent-browser 可用性**
浏览器验收依赖 `agent-browser` CLI（P1 实测可用）。注意两点：`eval` 传 JS 需先 base64 编码（PowerShell 无 heredoc）；`click @ref` 语法会报 "Missing arguments"，改用 `agent-browser find text "..." click`。若该 CLI 不可用，退回手工浏览器操作即可。

**端口冲突**
- 3306 / 6379 被本机已有 MySQL/Redis 占用时，`docker compose up` 会失败。停掉占用进程或改 `infra/docker-compose.yml` 的映射端口。
- 3000 / 3001 / 8080 / 9000 被占用时，对应服务启动即报错。释放端口后重试。

**remoteEntry.js 404**
Shell 加载 Feed Remote 的 `remoteEntry.js`（MF 插件已固定 `filename: 'remoteEntry.js'`）。404 的原因几乎都是 **Feed Remote（:3001）还没启动**。先启动 feed-remote，再刷新宿主页面。

**PowerShell 语法**
`&&` 不可用，连续命令用 `;`。docker CLI 的 stderr 会冒 "No hook installed" 中文乱码，属正常噪音，可用 `Select-String -NotMatch "Warning|No hook"` 过滤。

**想重置数据**
`docker compose down -v` 会删除数据卷，下次 `up` 重新执行 init SQL，恢复 4 个种子用户与空表。

## 5. 演示环境注意事项

- **dev 登录是 P1 临时方案**：当前用 `POST /api/dev/login`（body `{user_id}`）直接签发 JWT，另有 `GET /api/dev/users` 返回硬编码种子清单。没有真实密码与注册流程。**P2 会替换为 User Service 的正式注册/登录（正式 JWT 签发）**，届时 dev 端点移除，登录页改为注册/登录表单。
- dev 登录本身不校验 user 存在性（Feed Service 在 CreatePost 时校验，不存在返回 INVALID_ARGUMENT）。
- 帖子内容限制 ≤280 字符（UTF-8 rune 计数），评论 ≤500 字符。

---

相关文档：`docs/architecture.md`（架构说明）· `docs/acceptance.md`（P1 验收记录）· `infra/README.md`（基础设施与 curl 手动演示序列）
