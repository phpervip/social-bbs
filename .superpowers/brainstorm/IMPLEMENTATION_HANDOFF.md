# B — 简化版 社交平台 实现交接文档（Plan B 纵向切片）

> 本文件是设计阶段的最终产物，实现阶段（新 opencode 终端）以此为唯一依据。
> 设计图全部保存在 `.superpowers/brainstorm/content/*.html`，可在 http://localhost:52341 预览。

## 1. 项目定位

- 简化版 社交平台（微博+私信+短视频全含，只做 20% 核心逻辑），Logo 为字母 **B**
- 目标：**学习/演示**项目，真实多语言微服务，本地 Kind (K8s in Docker) 部署
- 范围：全功能但简化；UI 极简主义（黑白中性底 + 咖啡棕点缀）

## 2. 技术栈

| 层 | 技术 |
|---|---|
| 前端 | React + Webpack + Module Federation（Shell + 3 Remote） |
| 网关 | Node.js API Gateway（REST 对外 + gRPC 转发 + WebSocket） |
| 服务 | User (Java Spring Boot) / Feed (Go) / Video (Go) / Notification / Search (Go) |
| 数据 | MySQL 8（分库）、Redis 7、Elasticsearch 8 + IK 分词器 |
| 消息 | Kafka（KRaft 单节点） |
| 对象存储 | MinIO 模拟 S3（生产换 AWS S3）+ Nginx HLS 静态服务 |
| 部署 | Kind 集群（1 control-plane + 2 worker）+ Ingress-NGINX + NodePort |

## 3. 全局架构决策（已批准，不得随意改动）

### 3.1 请求链路
`Browser → b.local(NodePort:80) → Ingress-Nginx → Gateway(8080) → gRPC Service(DNS) → MySQL/Redis/Kafka/MinIO → HLS 静态资源 → Browser 播放器`

### 3.2 统一规范
- 🔵 对外 HTTP/HTTPS，内网服务间统一 **gRPC**；前端请求一律收敛到 Gateway，不直连后端
- 🟠 Kafka 异步事件：Feed 动态 / 视频转码 / 用户变更 / MySQL→ES 同步
- WebSocket：`GET /ws/notifications` 完整走鉴权层，Gateway 升级后转发到 Notification Service
- 每服务三层架构：Handler → Service → Repository；gRPC 接口 + Kafka 事件 + 表结构 + Redis 策略见各设计图
- 部署：无状态微服务 Deployment ×2 副本（滚动更新 + HPA）；有状态组件 StatefulSet ×1（不启用 HPA）

### 3.3 前端 Module Federation
- Shell(Host) + User/Feed/Video 三个 Remote；Shell 统一注册路由（见 frontend-ui-design.html 路由表）
- 鉴权：Shell 统一持久化 JWT + 共享 API Client + 全局 EventBus 同步登录状态（**不用 React Context**）
- Remote 间禁止直接依赖，经 EventBus 通信；每个 Remote 外层 ErrorBoundary
- 品牌色：**咖啡棕 #8B5A2B**（brand）+ 焦糖 #C19A6B（brand-2），全局仅此一色系

### 3.4 关键服务决策摘要

**API Gateway**：五层中间件（前置过滤/鉴权安全/限流治理/路由转发/后置处理）；每服务独立熔断；登录注册跳过 JWT 但保留限流+IP 黑名单+CORS；Token 黑名单 Redis TTL=JWT 有效期；重试仅 GET 幂等接口。

**User Service**（Java）：users/follows/user_sessions 表；关注流程事务提交后（AFTER_COMMIT）+ 本地消息表兜底；缓存「先更新库再删缓存」+ TTL 兜底；ZRange 分页；(user_id, followee_id) 唯一索引幂等；只生产不消费 Kafka；演示环境物理外键、生产移除。

**Feed Service**（Go）：Timeline 混合 Push/Pull — 粉丝 ≤1000 写扩散 feed:home ZSet，粉丝 >1000 大 V 走 Pull 读时合并；发帖先响应客户端，Fanout 异步 Worker 执行；outbox_events 发件箱 + 定时补偿保障 Kafka 投递；帖子删除不批量清理 Timeline，前端读取后二次过滤；posts/post_likes/post_comments/outbox_events 表；点赞计数为近似值，最终以 MySQL 为准；关注变更：新增关注回填近期帖子、取关清理。

**Video Service**（Go）：S3 Multipart 分片上传（业务不拼接文件）；InitUpload 分布式锁 upload:init:lock；CompleteUpload 后发转码任务（topic: video:transcode-task）；FFmpeg Worker 分布式锁防重复消费、失败重试上限 3 次 + 死信 + 人工重试；定时 AbortMultipartUpload 清碎片；时效签名 URL + Referer 防盗链；playback 缓存 TTL 自动失效；videos/uploads/transcode_tasks 表；topic 语义：video:transcode-task=待转码指令（消费），video:transcoded=转码结果事件（生产）。

**Notification Service**：统一 Kafka 消费（user:follow-changed/post:created/post:liked/post:commented/video:transcoded）；biz_unique_id + DB 唯一索引 + Redis SETNX 幂等；消费限速 + 批量写入；WS 网关转发链路；离线持久入库、重连拉取；多端已读同步；未读数 MySQL 兜底重建；notifications 表 30 天归档清理。

**Search Service**（Go）：ES Sync Consumer（独立 consumer group: es-sync-group）消费 post:created/deleted/updated；biz_unique_id 幂等；写入失败重试 3 次 + dead_letter 表 + 人工重试 + 7 天清理；每 10min 增量扫描 MySQL（updated_at 游标）兜底；检索 multi_match BM25 + filter（author/时间/类型/视频）+ 排序（_score/created_at/like_count + function_score 时间衰减）+ from/size 分页（前端限 100 页，可升级 search_after）；三级降级 L1 Redis 缓存(5min TTL) → L2 ES → L3 MySQL FULLTEXT；ES 需 IK 分词器；**一致性边界：最终一致（秒级），强实时场景直接读 MySQL**。

## 4. 实现计划（已批准：计划 B 纵向切片）

| 阶段 | 内容 | 完成标志 |
|---|---|---|
| P1 | Shell + Feed Remote + Gateway + Feed Service + MySQL/Redis | 「发帖 → 首页时间线」端到端跑通 |
| P2 | User Service + 注册/登录/关注 + JWT 鉴权；Feed 回填 outbox 事件化 | 账号体系可用，发帖走 Kafka 事件 |
| P3 | Video Service + S3 分片 + FFmpeg 转码 + HLS 播放 | 视频上传→转码→播放闭环 |
| P4 | Kafka 事件化完善 + Notification + ES Search + 通知抽屉 | 通知实时推送 + 全文搜索可用 |

每阶段结束必须有一次完整演示；P2 需回填 Feed 的 outbox 改造（设计已预留，成本可控）。

## 5. 环境说明（Kind）

- ⚠️ **Kind 仅用于本地架构演示，禁止生产部署**；单宿主机无容灾；Local PV 不支持 Pod 跨节点迁移
- 中间件单实例为演示简化，生产需多副本（Kafka ≥3 / Redis 主从 / ES 多副本）
- FFmpeg Worker 靠分布式锁防重复消费
- 扩展规划：Prometheus+Grafana / Loki / NetworkPolicy（非本期范围）
- 本机 hosts 需添加：`127.0.0.1 b.local`
- MinIO 桶：videos / uploads / avatars

## 6. 设计图索引（.superpowers/brainstorm/content/）

- architecture-v2.html — 总架构
- mf-architecture-v2.html — 前端 MF
- gateway-design.html — API Gateway
- user-service.html — User Service
- feed-service.html — Feed + Timeline
- video-service.html — Video Service
- notification-service.html — Notification + Kafka 拓扑
- es-search-service.html — ES Sync + Search
- k8s-topology.html — K8s 部署拓扑
- frontend-ui-design.html — 前端 UI（咖啡棕品牌色）
- implementation-plan.html — A/B 计划对比（本计划依据）
