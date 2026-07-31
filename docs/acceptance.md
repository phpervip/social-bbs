# B 社交平台 · P1 验收记录

> 阶段：**P1 — 发帖 → 首页时间线 端到端闭环**
> 交付：远程 `https://github.com/phpervip/social-bbs`，`main` 与 `feat/p1-feed-loop` 两分支均指向 **`8ca6ae5`**（见 `P2-HANDOFF.md §1.1`）
> 本文档记录验收标准、验收证据与集成阶段踩坑清单。

## 1. 验收标准（4 条）

| # | 标准 | 判定方式 | 结果 |
|---|---|---|---|
| 1 | 端到端自动化验证通过 | `infra/demo-e2e.ps1` 全部用例 PASS | ✅ 8/8 PASS |
| 2 | 浏览器手工流程跑通核心闭环 | 登录 → 发帖 → 时间线 → 点赞 → 评论 → 删除 | ✅ 通过 |
| 3 | 构建与测试全绿 | Go vet/build/test + Gateway 单测 + 前端双仓 build | ✅ 通过 |
| 4 | 设计偏差完整记录并有 P2 衔接 | `plan.md §1.1` 的 D1-D7 全部登记 | ✅ 已登记 |

## 2. 验收证据

### 2.1 端到端 e2e：8/8 PASS

运行方式：

```powershell
cd infra
./demo-e2e.ps1
```

脚本按顺序覆盖 8 个场景，逐步骤输出 PASS/FAIL，全部通过退出码为 0：

| # | 用例 | 覆盖点 |
|---|---|---|
| 1 | healthz | Gateway 健康检查 `{code:0, message:"ok"}` |
| 2 | 登录 | `POST /api/dev/login` 签发 JWT（dev-only，P1 临时方案） |
| 3 | 发帖 | `POST /api/feed/post` 创建帖子 |
| 4 | 时间线 | `GET /api/feed/home` 返回刚发的帖子 |
| 5 | 点赞 | `POST /api/feed/like`，like_count 精确 +1 |
| 6 | 评论 | `POST /api/feed/comment`，comment_count +1 |
| 7 | 跨账号扇出 | 其他种子账号时间线也能看到该帖（全局 Fanout） |
| 8 | 删除 | 仅作者可删（DELETE 鉴权），软删除后时间线读时过滤 |

### 2.2 浏览器实测流程

以 dev 账号 Alice 登录走完完整闭环：

1. 打开 http://localhost:3000
2. dev 登录 Alice
3. 发一条帖子 → 首页时间线**即时出现**（Fanout 异步写入 `feed:home`）
4. 点赞（♥）→ 计数 +1
5. 发表评论 → 评论区出现
6. 删除帖子 → 弹确认 → 时间线不再显示
7. 换 Bob 登录 → 验证 Alice 的帖子出现在 Bob 时间线（跨账号扇出）

浏览器验收使用 `agent-browser` CLI（`eval` 传 JS 需 base64，PowerShell 无 heredoc；`click @ref` 语法会报 "Missing arguments"，改用 `agent-browser find text "..." click`）。

### 2.3 构建与测试

| 项 | 命令 | 结果 |
|---|---|---|
| Go 静态检查 | `go vet ./...` | PASS |
| Go 编译 | `go build ./...` | PASS |
| Go 单元测试 | `go test ./...` | **19/19 PASS** |
| Gateway 单测 | `npm test`（node:test） | **41/41 PASS** |
| 前端 Shell build | `npm run build`（frontend/shell） | PASS |
| 前端 Feed Remote build | `npm run build`（frontend/feed-remote） | PASS |

Go 不在 PATH，使用完整路径 `C:\Program Files\Go\bin\go.exe`（go1.26.5）执行。

### 2.4 终审

T8 终审 4/4 完成标志 PASS（报告在 `D:\Personal\Temp\opencode\t8-review.md`，非仓库内）。

## 3. 集成阶段踩坑清单（T7：5+1 处修复）

集成阶段共修复 6 处问题，全部记录在 git 历史与 `P2-HANDOFF.md §1.4`，P2 不得重犯：

| commit | 问题 | 修复 |
|---|---|---|
| `66b50e4` | Gateway gRPC `deadline` 在模块加载时求值，运行后必然过期 → 503 | 改函数每次调用求值 |
| `66b50e4` | GORM 把 `PostRow` 复数化成 `post_rows`，读写表分离 | 加 `TableName() → "posts"` |
| `66b50e4` | MF shared 缺 `eager: true` → "Shared module not available" | 补 eager |
| `66b50e4` | `ui.js` 用 React 钩子未 import → "React is not defined" | 补 import |
| `66b50e4` | MF 插件未指定 `filename` → 容器 chunk 落 `feed.js`，宿主请求 remoteEntry.js 404 | `filename: 'remoteEntry.js'` |
| `8ca6ae5` | **三个 MF 暴露组件没 import `styles.css`** → 所有控件浏览器默认尺寸 | 各组件 `import './styles.css'` |

> ⚠️ 重要教训：MF Remote 的每个暴露组件必须自带 `import './styles.css'`（style-loader 幂等），否则样式只在独立 dev 入口生效、宿主里全部退化。

## 4. 偏差与遗留

- 设计偏差 D1-D7 全部记录在 `plan.md §1.1`，P2 衔接点见 `P2-HANDOFF.md §2.1`（本阶段 D6 点赞精确计数、D7 MySQL LIKE 搜索留到 P4）。
- P1 的 dev 登录（`POST /api/dev/login` + `GET /api/dev/users`）是临时方案，**P2 由 User Service 正式注册/登录替换**。
- `outbox_events` 表已建好（id/topic/payload/status/retry_count/created_at），P2 回填启用。

---

相关文档：`docs/architecture.md`（架构说明）· `docs/demo-guide.md`（启动与演示手册）· `P2-HANDOFF.md`（P1 → P2 交接入口）
