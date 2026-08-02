# infra — 本地基础设施（MySQL 8 + Redis 7 + Kafka 3）

「B」项目 P2 的本地依赖：MySQL 8（feed_db + user_db）+ Redis 7 + Kafka 3（KRaft 单节点），通过 docker-compose 一键起停。

## 前置条件

- Docker Desktop 已启动（`docker info` 可正常输出）
- 端口 3306 / 6379 / 9092 未被占用
- Node.js 18+ / Go 1.22+（本机实测 Go 1.26.5）。若 `go` 不在 PATH（如仅存在于 `C:\Program Files\Go\bin\go.exe`），请在启动 Feed Service 前手动加入 PATH 或用完整路径调用。

## 快速开始

```powershell
cd infra

# 启动
docker compose up -d

# 查看状态（三个容器应均为 Up）
docker compose ps

# 停止（保留数据卷）
docker compose down

# 彻底重置（删除数据卷，下次 up 重新执行 init SQL）
docker compose down -v
```

首次启动会自动执行：
- `mysql/init/01-feed.sql`：建库 `feed_db`（5 张表）+ 写入 4 个演示用户
- `mysql/init/02-user.sql`：建库 `user_db`（4 张表）+ 写入 4 个种子用户（bob / alice / carol / dave，密码统一 `Password123!`）+ 创建 `user`/`user123` 专用账号

## 等待 MySQL 就绪

init SQL 需要几秒，就绪后再启动应用：

```powershell
# 轮询直到输出 mysqld is alive
docker exec b-mysql mysqladmin ping -uroot -proot123 --silent
```

## Kafka 健康检查

```powershell
# 查看 topic 列表（auto.create.topics.enable=true，无需手动建 topic）
docker compose exec kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --list
```

## 启动应用服务

MySQL/Redis/Kafka 就绪后，按**根目录 README** 依次启动：

1. User Service（Java）：`cd ../services/user-service; .\mvnw spring-boot:run`
2. Feed Service（Go）：`cd ../services/feed-service; go run ./cmd/server`
3. Gateway（Node）：`cd ../services/gateway; npm run dev`
4. 前端：
   - Shell（:3000）
   - Feed Remote（:3001）
   - User Remote（:3002）

详见 `../README.md`「快速启动」。

## 手动演示（curl 序列）

> 以下命令基于 P1 的 `/api/dev/login`，P2 改为 User Service 签发 JWT。完整的新版演示（注册 / 登录 / 关注 / 发帖 / 时间线）将在 Gateway T5 完成后更新到此文档。

以下请求都打到 Gateway `http://localhost:8080`（需先启动 Feed Service + Gateway）。

```powershell
# 1. 健康检查
curl.exe -s http://localhost:8080/healthz

# 2. 登录（dev 账号 alice, user_id=2），拿到 token
$login = curl.exe -s -X POST http://localhost:8080/api/dev/login -H "Content-Type: application/json" -d '{\"user_id\":2}'
# 从返回 data.token 取 JWT，存入 $token

# 3. 发帖
curl.exe -s -X POST http://localhost:8080/api/feed/post `
  -H "Content-Type: application/json" -H "Authorization: Bearer $token" `
  -d '{\"content\":\"hello B demo\"}'

# 4. 首页时间线
curl.exe -s "http://localhost:8080/api/feed/home?cursor=0&limit=20" -H "Authorization: Bearer $token"

# 5. 点赞（post_id 换成上一步返回的 id）
curl.exe -s -X POST http://localhost:8080/api/feed/like `
  -H "Content-Type: application/json" -H "Authorization: Bearer $token" `
  -d '{\"post_id\":1}'

# 6. 评论
curl.exe -s -X POST http://localhost:8080/api/feed/comment `
  -H "Content-Type: application/json" -H "Authorization: Bearer $token" `
  -d '{\"post_id\":1,\"content\":\"nice!\"}'

# 7. 帖子详情 / 评论列表
curl.exe -s http://localhost:8080/api/feed/post/1 -H "Authorization: Bearer $token"
curl.exe -s http://localhost:8080/api/feed/post/1/comments -H "Authorization: Bearer $token"

# 8. 删除（仅作者）
curl.exe -s -X DELETE http://localhost:8080/api/feed/post/1 -H "Authorization: Bearer $token"
```

## 自动化验证

```powershell
cd infra
./demo-e2e.ps1
```

脚本覆盖：健康检查 → 登录 → 发帖 → 首页时间线 → 点赞 → 评论 → 跨用户全局扇出 → 删除鉴权/软删除，逐步骤输出 PASS/FAIL，全部通过退出码为 0。

P2 扩展（注册 / 真实关注 / Kafka fanout / 黑名单）将在 T8 联调阶段更新到 `demo-e2e.ps1`。
