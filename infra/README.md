# infra — 本地基础设施（MySQL 8 + Redis 7）

「B」项目 P1 的本地依赖：MySQL 8（feed_db）+ Redis 7，通过 docker-compose 一键起停。

## 前置条件

- Docker Desktop 已启动（`docker info` 可正常输出）
- 端口 3306 / 6379 未被占用

## 快速开始

```powershell
cd infra

# 启动
docker compose up -d

# 查看状态（两个容器应均为 Up）
docker compose ps

# 停止（保留数据卷）
docker compose down

# 彻底重置（删除数据卷，下次 up 重新执行 init SQL）
docker compose down -v
```

首次启动会自动执行 `mysql/init/01-feed.sql`：建库 `feed_db`（5 张表）+ 写入 4 个种子用户（bob / alice / carol / dave）。

## 等待 MySQL 就绪

init SQL 需要几秒，就绪后再启动应用：

```powershell
# 轮询直到输出 mysqld is alive
docker exec b-mysql mysqladmin ping -uroot -proot123 --silent
```

## 启动应用服务

MySQL/Redis 就绪后，按**根目录 README** 依次启动 Feed Service（Go）、Gateway（Node）、前端 Shell / Feed Remote（见 `../README.md`「快速启动」）。

## 手动演示（curl 序列）

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

脚本覆盖：健康检查 → dev 登录 → 发帖 → 首页时间线 → 点赞 → 评论 → 跨用户全局扇出 → 删除鉴权/软删除，逐步骤输出 PASS/FAIL，全部通过退出码为 0。
