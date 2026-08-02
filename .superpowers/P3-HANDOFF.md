# P3 交接文档

## 当前状态

- **分支**: `main`（P2 已合并）
- **所有服务运行中**: Gateway :8080, Feed Service :9000, User Service :9001, MySQL :3306, Redis :6379, Kafka :9092

## P2 完成内容

| 功能 | 状态 |
|---|---|
| 注册/登录/JWT 鉴权 | ✅ |
| 关注/取关 + fanout | ✅ |
| Kafka outbox 事件化 | ✅ |
| Feed Service (Go) + User Service (Java) | ✅ |
| Gateway (Node.js + Fastify) | ✅ |
| 前端 Module Federation (Shell + User Remote + Feed Remote) | ✅ |

## P3 目标

**Video Service + S3 分片 + FFmpeg 转码 + HLS 播放**

根据 `.superpowers/brainstorm/IMPLEMENTATION_HANDOFF.md` 设计：

| 组件 | 技术 |
|---|---|
| Video Service | Go + gRPC + GORM |
| 对象存储 | MinIO (模拟 S3) |
| 转码 | FFmpeg HLS 切片 |
| 表 | videos / uploads / transcode_tasks |

### P3 核心流程

```
用户上传视频 → InitUpload (分布式锁) → 分片上传 S3 → CompleteUpload 
→ 发 Kafka 转码任务 → FFmpeg Worker 消费 → HLS 切片 → 写入 MinIO 
→ 更新视频状态 → 前端播放
```

### P3 需要新增的组件

1. **Video Service (Go)** — 独立服务，端口 9002
2. **MinIO** — docker-compose 新增，模拟 S3
3. **FFmpeg Worker** — Video Service 内的 goroutine，消费 Kafka
4. **前端 Video Remote** — Module Federation，端口 3003
5. **Gateway 路由** — 新增 `/api/video/*` 路由

## 设计文档位置

- 完整设计: `.superpowers/brainstorm/IMPLEMENTATION_HANDOFF.md`
- 设计图: `.superpowers/brainstorm/content/video-service.html`
- 总架构: `.superpowers/brainstorm/content/architecture-v2.html`

## 启动 P3 的提示词

```
继续 P3 实现。先读取 .superpowers/brainstorm/IMPLEMENTATION_HANDOFF.md 了解设计，
然后读取 .superpowers/brainstorm/content/video-service.html 查看 Video Service 设计图。

P3 目标：Video Service + S3 分片 + FFmpeg 转码 + HLS 播放

当前环境：
- infra/docker-compose.yml 需要新增 MinIO
- services/video-service/ 需要新建（Go + gRPC）
- services/gateway/src/routes/video.js 需要新增
- frontend/video-remote/ 需要新建（React MF）
- Kafka topic: video:transcode-task, video:transcoded

请先制定实现计划，然后开始 T1 脚手架搭建。
```
