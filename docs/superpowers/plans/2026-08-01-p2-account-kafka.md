# P2 实施计划：账号体系 + 关注 + JWT 鉴权 + Kafka/outbox 事件化

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 账号体系（注册/登录/关注/取关 + 正式 JWT 鉴权）可用，Feed 发帖走 Kafka 事件（outbox → Kafka → fanout → 粉丝时间线）。

**Architecture:** 新增 Java 21 + Spring Boot 3 User Service（gRPC :9001，独立 user_db）签发 JWT 并生产 `user:*` 事件；新增 User Remote 微前端（:3002）承载注册/登录/个人主页/关注；Feed Service 改造 CreatePost 事务内写 outbox_events，双 worker（常驻 dispatcher + 定时补偿）投递 `post:created` 到 Kafka，Fanout 由进程内 channel 改为消费 Kafka 事件并向真实粉丝写扩散，同时消费 `user:follow-changed` 做关注回填/取关清理。Gateway 删除 dev 登录，增加 auth/user 路由与黑名单检查。

**Tech Stack:** Java 21 + Spring Boot 3 + gRPC(net.devh) + MyBatis-Plus + spring-data-redis + spring-kafka + jjwt + spring-security-crypto(BCrypt)；Go 1.26 + segmentio/kafka-go；Node Fastify；React 18 Webpack 5 MF；apache/kafka KRaft 单节点。

## Global Constraints

- 分支：从 `main` 切 `feat/p2-account-kafka`；最终推 main + 该分支
- 端口：Gateway 8080 · Feed gRPC 9000 · **User gRPC 9001** · MySQL 3306 · Redis 6379 · **Kafka 9092** · Shell 3000 · Feed Remote 3001 · **User Remote 3002**
- 数据库：`feed_db`（账号 `feed`/`feed123`，P1 不动表结构）+ 新增 **`user_db`**（账号 `user`/`user123`，经 `02-user.sql` 建库建表）
- **`proto/feed.proto` 已冻结，不得改动**；新增 `proto/user.proto`（冻结契约）
- JWT：HS256，`JWT_SECRET` 环境变量 **Gateway 与 User Service 共享**（dev 默认 `dev-secret`），TTL 24h（`JWT_TTL_SECONDS`）
- 事件 topic：`post:created`（Feed 生产，feed-fanout 组消费）· `user:follow-changed`（User 生产，feed-timeline 组消费）· `user:registered`（User 生产，P2 无人消费）
- Kafka 客户端（Go）：**`segmentio/kafka-go`**（纯 Go 无 cgo；禁止 confluent-kafka-go / sarama）
- Redis 键名与设计一字不差：`user:profile:{id}`(10min) / `user:followers:{id}`(ZSet 5min) / `user:following:{id}`(ZSet 5min) / `auth:blacklist:{jti}`(TTL=JWT 剩余)；Feed 既有 `feed:home:`/`post:detail:` 等不变
- 种子用户迁移：bob/alice/carol/dave 进 user_db，密码统一 `Password123!`（BCrypt）；**feed_db.users 表移除**，Feed 作者信息改走 User Service gRPC + Redis 缓存
- 品牌色 #8B5A2B / #C19A6B；代码英文注释命名，UI 文案中文
- Windows PowerShell 5.1（`&&` 不可用）；Go 用全路径 `C:\Program Files\Go\bin\go.exe`
- Maven：仓库提交 `mvnw`（Wrapper 自举）；JDK 21 Temurin 已装，JAVA_HOME 已配置
- 每任务完成后 `lsp_diagnostics` / 对应测试 / build 全绿；不 commit 除非任务步骤明确要求

---

## 1. 任务分解与依赖总览

| # | 任务 | 目录 | 依赖 | 类型 |
|---|---|---|---|---|
| T1 | 脚手架：切分支、proto/user.proto、user-service Maven 骨架 + mvnw、README 阶段说明 | 根 / proto / services/user-service | - | 控制器直做 |
| T2 | Infra：docker-compose + Kafka、02-user.sql、infra README 更新 | infra/ | T1 | 派发(quick) |
| T3 | User Service（Java/Spring Boot，gRPC + BCrypt + JWT + follow + Kafka 生产 + 本地消息表兜底） | services/user-service/ | T1 | 派发(deep) |
| T4 | Feed Service 改造：outbox 写入 + 双 worker + Kafka 消费（fanout 真实粉丝 + follow-changed 回填/清理）+ 作者信息改 User Service | services/feed-service/ | T1 | 派发(deep) |
| T5 | Gateway：user gRPC 客户端、auth/user 路由、删 dev.js、黑名单检查、cors | services/gateway/ | T1,T3(契约) | 派发(unspecified-high) |
| T6 | User Remote 新建（Auth + Profile） | frontend/user-remote/ | T1 | 派发(visual-engineering) |
| T7 | Shell 改造（remotes+user、路由、api-client 去 dev、event-bus）+ Feed Remote 作者链接 | frontend/shell/, frontend/feed-remote/ | T6 | 派发(visual-engineering) |
| T8 | 集成：compose up → 起全部服务 → e2e 扩展验证 → 浏览器验收 | - | T2-T7 | 控制器 + 修复派发 |
| T9 | 全分支评审（终审）+ 完成标志复验 | - | T8 | 派发 |

T2/T3/T4/T5/T6 在 T1 后**并行派发**；T7 依赖 T6 的组件暴露；T8 集成。

---

## 2. 契约：`proto/user.proto`（T1 冻结，T3/T5 两端生成 stub）

```proto
syntax = "proto3";
package user.v1;

option go_package = "social-bbs/user-service/proto/gen;userpb";

service UserService {
  rpc Register(RegisterRequest) returns (AuthResponse);
  rpc Login(LoginRequest) returns (AuthResponse);
  rpc Logout(LogoutRequest) returns (LogoutResponse);
  rpc GetProfile(GetProfileRequest) returns (GetProfileResponse);
  rpc UpdateProfile(UpdateProfileRequest) returns (UpdateProfileResponse);
  rpc Follow(FollowRequest) returns (FollowResponse);
  rpc Unfollow(UnfollowRequest) returns (UnfollowResponse);
  rpc GetFollowers(GetFollowersRequest) returns (FollowListResponse);
  rpc GetFollowing(GetFollowingRequest) returns (FollowListResponse);
}

message User {
  int64 id = 1;
  string username = 2;
  string display_name = 3;
  string email = 4;
  string bio = 5;
  string avatar_url = 6;
  int64 follower_count = 7;
  int64 following_count = 8;
  int64 created_at = 9; // unix ms
}

message RegisterRequest { string username = 1; string email = 2; string password = 3; string display_name = 4; }
message LoginRequest { string account = 1; string password = 2; } // account = username 或 email
message AuthResponse { string token = 1; int64 expires_in = 2; User user = 3; }
message LogoutRequest { string jti = 1; }
message LogoutResponse {}
message GetProfileRequest { int64 user_id = 1; }
message GetProfileResponse { User user = 1; }
message UpdateProfileRequest { int64 user_id = 1; string display_name = 2; string bio = 3; string avatar_url = 4; }
message UpdateProfileResponse { User user = 1; }
message FollowRequest { int64 follower_id = 1; int64 followee_id = 2; }
message FollowResponse {}
message UnfollowRequest { int64 follower_id = 1; int64 followee_id = 2; }
message UnfollowResponse {}
message GetFollowersRequest { int64 user_id = 1; int64 cursor = 2; int32 limit = 3; }
message GetFollowingRequest { int64 user_id = 1; int64 cursor = 2; int32 limit = 3; }
message FollowListResponse { repeated User users = 1; int64 next_cursor = 2; bool has_more = 3; }
```

错误码：`INVALID_ARGUMENT→400`、`ALREADY_EXISTS→409`、`UNAUTHENTICATED→401`、`NOT_FOUND→404`、`INTERNAL→500`（Gateway 映射用，与 feed.proto 契约一致）。

JWT payload：`{sub: String(user_id), username, displayName, jti(UUID), iat, exp}`，HS256。

---

## 3. T1 脚手架（控制器直做）

**Files:**
- Create: `proto/user.proto`（§2 全文）
- Create: `services/user-service/pom.xml`、`services/user-service/.mvn/wrapper/maven-wrapper.properties`、`mvnw`/`mvnw.cmd`
- Create: `services/user-service/src/main/resources/application.yml`（占位骨架）
- Modify: `README.md`（阶段说明 P2、组件表 +User Service/User Remote/Kafka）
- 动作：`git checkout -b feat/p2-account-kafka`（从 main）

**Interfaces:**
- Produces: `proto/user.proto`（冻结契约，T3 生成 Java stub、T5 复制到 gateway）、user-service Maven 骨架目录

- [ ] **Step 1: 切分支 + 建 proto**
  ```
  git checkout main; git pull; git checkout -b feat/p2-account-kafka
  ```
  写入 `proto/user.proto`（§2 全文，逐字）。`go vet` 不受影响；无 Go stub 生成。

- [ ] **Step 2: Maven 骨架 + Wrapper**
  - 用 spring initializr 手动写 `pom.xml`，关键依赖（版本用当前最新稳定，2026-08）：
    ```xml
    <parent>spring-boot-starter-parent 3.x</parent>
    <dependency> net.devh:grpc-server-spring-boot-starter (3.1.x)
    <dependency> com.baomidou:mybatis-plus-boot-starter (3.5.x)
    <dependency> org.springframework.boot:spring-boot-starter-data-redis
    <dependency> org.springframework.kafka:spring-kafka
    <dependency> io.jsonwebtoken:jjwt-api + jjwt-impl + jjwt-jackson (0.12.x)
    <dependency> org.springframework.security:spring-security-crypto
    <plugin> org.xolstice.maven.plugin:protobuf-maven-plugin (0.6.1) 配置 protoc + grpc-java codegen
    ```
  - **bootstrap Maven Wrapper（本机 Maven 确认不在 PATH）**：
    ```powershell
    # 下载 Maven 发行版到临时工具目录（一次性）
    $mvnVer = "3.9.9"
    $zip = "D:\Personal\Temp\opencode\apache-maven-$mvnVer-bin.zip"
    Invoke-WebRequest "https://archive.apache.org/dist/maven/maven-3/$mvnVer/binaries/apache-maven-$mvnVer-bin.zip" -OutFile $zip
    Expand-Archive $zip -DestinationPath "D:\Personal\Temp\opencode" -Force
    $MAVEN = "D:\Personal\Temp\opencode\apache-maven-$mvnVer\bin\mvn.cmd"
    # 在 user-service 目录生成 wrapper（生成后 mvnw/mvnw.cmd/.mvn 提交入库）
    & $MAVEN -N wrapper:wrapper -Dmaven=3.9.9
    ```
  - 验证：`.\mvnw -v` 输出 Maven 3.9.9（首次自动下载 wrapper jar；后续所有 Maven 命令一律用 `.\mvnw`，不依赖系统 Maven）

- [ ] **Step 3: 骨架目录 + application.yml 占位**
  ```
  services/user-service/src/main/java/social/bbs/user/
    UserServiceApplication.java  (@SpringBootApplication 空壳)
  src/main/resources/application.yml  (server 端口 9001 占位，其余 T3 填)
  src/test/java/social/bbs/user/ (占位测试目录)
  services/user-service/proto/user.proto (复制)
  ```
  `UserServiceApplication.java` 写最小可编译类。

- [ ] **Step 4: 验证编译**
  Run: `.\mvnw -q compile`（在 services/user-service）
  Expected: BUILD SUCCESS；protobuf 插件生成 `target/generated-sources` 中的 UserServiceGrpc 等类（此时 proto 已在）。

- [ ] **Step 5: 更新 README 阶段说明**
  阶段说明加 P2 一行 + 组件表加 User Service / User Remote / Kafka 三行（端口见 Global Constraints）。

- [ ] **Step 6: Commit**
  ```bash
  git add proto/user.proto services/user-service README.md
  git commit -m "feat(p2): scaffold user-service (Maven + grpc proto) and branch"
  ```

---

## 4. T2 Infra（派发 quick）

**Files:**
- Modify: `infra/docker-compose.yml`（+kafka 服务）
- Create: `infra/mysql/init/02-user.sql`（user_db schema + 种子）
- Modify: `infra/README.md`

**Interfaces:**
- Consumes: T1（目录骨架、user_db 设计）
- Produces: Kafka :9092、user_db（表 + 种子 4 用户）、MySQL 新账号 `user`/`user123` —— T3/T4 依赖

- [ ] **Step 1: docker-compose 加 kafka（KRaft 单节点）**
  ```yaml
  kafka:
    image: apache/kafka:3.7.0
    container_name: b-kafka
    ports: ["9092:9092"]
    environment:
      KAFKA_NODE_ID: 1
      KAFKA_PROCESS_ROLES: broker,controller
      KAFKA_LISTENERS: PLAINTEXT://:9092,CONTROLLER://:9093
      KAFKA_ADVERTISED_LISTENERS: PLAINTEXT://localhost:9092
      KAFKA_CONTROLLER_LISTENER_NAMES: CONTROLLER
      KAFKA_LISTENER_SECURITY_PROTOCOL_MAP: CONTROLLER:PLAINTEXT,PLAINTEXT:PLAINTEXT
      KAFKA_CONTROLLER_QUORUM_VOTERS: 1@localhost:9093
      KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR: 1
      KAFKA_TRANSACTION_STATE_LOG_REPLICATION_FACTOR: 1
      KAFKA_TRANSACTION_STATE_LOG_MIN_ISR: 1
      KAFKA_AUTO_CREATE_TOPICS_ENABLE: "true"
      KAFKA_LOG_DIRS: /tmp/kraft-combined-logs
    restart: unless-stopped
  ```
  验证：`docker compose up -d` 后 `docker compose exec kafka kafka-topics.sh --bootstrap-server localhost:9092 --list` 无报错（stderr 中文乱码用 `Select-String -NotMatch "Warning|No hook"` 过滤）。

- [ ] **Step 2: 02-user.sql（user_db）**
  ```sql
  -- 02-user.sql : user_db schema + seed users (P2)
  SET NAMES utf8mb4;
  CREATE DATABASE IF NOT EXISTS user_db CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
  USE user_db;

  CREATE TABLE IF NOT EXISTS users (
      id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
      username VARCHAR(64) NOT NULL UNIQUE,
      email VARCHAR(255) NOT NULL UNIQUE,
      password_hash VARCHAR(100) NOT NULL,
      display_name VARCHAR(64) NOT NULL DEFAULT '',
      bio VARCHAR(255) NOT NULL DEFAULT '',
      avatar_url VARCHAR(255) NOT NULL DEFAULT '',
      follower_count INT NOT NULL DEFAULT 0,
      following_count INT NOT NULL DEFAULT 0,
      created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
      updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3)
  ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

  CREATE TABLE IF NOT EXISTS follows (
      follower_id BIGINT UNSIGNED NOT NULL,
      followee_id BIGINT UNSIGNED NOT NULL,
      created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
      PRIMARY KEY (follower_id, followee_id),
      CONSTRAINT fk_follows_follower FOREIGN KEY (follower_id) REFERENCES users(id),
      CONSTRAINT fk_follows_followee FOREIGN KEY (followee_id) REFERENCES users(id)
  ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

  CREATE TABLE IF NOT EXISTS user_sessions (
      token_id VARCHAR(64) PRIMARY KEY,
      user_id BIGINT UNSIGNED NOT NULL,
      expires_at DATETIME(3) NOT NULL,
      revoked TINYINT(1) NOT NULL DEFAULT 0,
      INDEX idx_user_sessions_user_id (user_id),
      CONSTRAINT fk_user_sessions_user FOREIGN KEY (user_id) REFERENCES users(id)
  ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

  -- 密码统一为 Password123! 的 BCrypt 哈希（$2a$10$... 固定串，T3 用 BCrypt.matches 校验）
  -- 哈希生成方法（二选一，任一工具输出固定串填入下方）：
  --   Java: new BCryptPasswordEncoder().encode("Password123!")
  --   Python: python -c "import bcrypt; print(bcrypt.hashpw(b'Password123!', bcrypt.gensalt(10)).decode())"
  INSERT IGNORE INTO users (id, username, email, password_hash, display_name) VALUES
      (1,'bob','bob@b.dev','$2a$10$<固定哈希>','Bob咖啡师'),
      (2,'alice','alice@b.dev','$2a$10$<固定哈希>','Alice设计师'),
      (3,'carol','carol@b.dev','$2a$10$<固定哈希>','Carol摄影师'),
      (4,'dave','dave@b.dev','$2a$10$<固定哈希>','Dave开发者');

  -- 专用 MySQL 账号（User Service 用这个连，替代 root；02-user.sql 以 root 身份执行建库授权）
  CREATE USER IF NOT EXISTS 'user'@'%' IDENTIFIED BY 'user123';
  GRANT ALL PRIVILEGES ON user_db.* TO 'user'@'%';
  FLUSH PRIVILEGES;

- [ ] **Step 3: 验证**
  - `docker compose up -d` → 三个容器健康（mysql/redis/kafka）
  - `docker exec b-mysql mysql -uroot -proot123 -e "USE user_db; SHOW TABLES;"` 输出 3 表 + 种子 4 行
  - Kafka topic 列表可查

- [ ] **Step 4: infra README 更新**
  启动步骤加：Kafka 已在 compose；`mvnw` 起 User Service 命令；User Remote 起法。e2e 章节标记「P2 扩展见 T8」。

- [ ] **Step 5: Commit**
  ```bash
  git add infra/
  git commit -m "feat(p2): add kafka KRaft to compose + user_db schema/seed"
  ```

---

## 5. T3 User Service（派发 deep）

**Files:**
- Create: `services/user-service/src/main/java/social/bbs/user/`（package `social.bbs.user`）
  - `config/GrpcConfig.java`、`config/RedisConfig.java`、`config/JwtProperties.java`
  - `controller/UserGrpcService.java`（net.devh `@GrpcService` 实现 UserServiceGrpc.UserServiceImplBase）
  - `service/AuthService.java`、`service/UserService.java`、`service/FollowService.java`
  - `repository/UserMapper.java`、`FollowMapper.java`、`SessionMapper.java`（MyBatis-Plus）
  - `entity/UserEntity.java`、`FollowEntity.java`、`UserSessionEntity.java`
  - `kafka/UserEventPublisher.java`（KafkaTemplate）
  - `listener/FollowEventListener.java`（`@TransactionalEventListener(phase=AFTER_COMMIT)`）
  - `util/JwtUtil.java`、`util/UserOutboxService.java`（本地消息表兜底）
- Modify: `services/user-service/src/main/resources/application.yml`
- Test: `src/test/java/social/bbs/user/`（JUnit + Spring Boot Test）

**Interfaces:**
- Consumes: T1 骨架 + proto/user.proto（protobuf 插件生成 Java stub）、T2 user_db/Kafka
- Produces: gRPC :9001；`user:registered`/`user:follow-changed` 事件；Redis `user:profile:`/`user:followers:`/`user:following:`/`auth:blacklist:`；JWT（jti+session）—— T5 Gateway 消费

- [ ] **Step 1: 写失败测试（核心逻辑先行）**
  `AuthServiceTest.java`：
  ```java
  @Test void register_duplicateUsername_throwsAlreadyExists() { ... }
  @Test void login_wrongPassword_throwsUnauthenticated() { ... }
  @Test void login_byEmail_works() { ... }
  @Test void jwt_roundtrip_preservesClaims() { ... }
  ```
  `FollowServiceTest.java`：
  ```java
  @Test void follow_duplicate_isIdempotent() { ... } // 第二次 INSERT IGNORE 不抛错
  @Test void unfollow_notFollowing_isNoop() { ... }
  @Test void follow_self_rejected() { ... } // INVALID_ARGUMENT
  ```
  用 H2 内存库 + MyBatis 或 mock Mapper。Run: `.\mvnw test` → 预期 FAIL（类不存在）。

- [ ] **Step 2: 实体 + Mapper（MyBatis-Plus）**
  `UserEntity`（id/username/email/passwordHash/bio/avatarUrl/displayName/followerCount/followingCount/createdAt/updatedAt，`@TableName("users")`）；`FollowEntity`（联合主键 `@TableId` followerId+followeeId 复合或用 IdType 组合）；`UserSessionEntity`（tokenId 主键）。Mapper 接口 extends `BaseMapper<T>`；FollowMapper 加 `insertIgnore(...)`（`@Insert("INSERT IGNORE INTO follows ...")`）。

- [ ] **Step 3: AuthService + JwtUtil**
  - `JwtUtil`：jjwt 0.12 API，`HS256`，secret 来自 `JwtProperties`（env `JWT_SECRET`，默认 `dev-secret`）；`generate(sub,username,displayName)` 返回 `(token, jti, expiresAt)`；`verify(token)` 返回 claims map
  - `AuthService.register`：校验 username/email 非空 + 密码 ≥6 → 查重（唯一冲突→`ALREADY_EXISTS`）→ BCrypt encode → insert → 签发 JWT → 写 user_sessions（token_id=jti, expires_at）→ 发 `user:registered` → 返回 AuthResponse
  - `AuthService.login`：account 匹配 username 或 email → 查用户 → BCrypt.matches 校验（错→`UNAUTHENTICATED`）→ 签发 JWT + 写 session → 返回
  - `AuthService.logout(jti)`：置 session revoked → `auth:blacklist:{jti}` SETEX 剩余有效期

- [ ] **Step 4: UserService（GetProfile/UpdateProfile）**
  - GetProfile：Redis `user:profile:{id}` 命中直返；miss 查库回填（TTL 10min）；不存在→NOT_FOUND；返回含 follower/following count
  - UpdateProfile：改 display_name/bio/avatar_url → update → **删缓存** `user:profile:{id}` → 返回新资料

- [ ] **Step 5: FollowService + 事件**
  - Follow：follower_id==followee_id→INVALID_ARGUMENT；`INSERT IGNORE` follows；affected==0 视为已关注（幂等不报错）；tx 内 users.follower_count/following_count ±1（≥0 保护）→ COMMIT
  - FollowEventListener（AFTER_COMMIT）：更新 Redis `user:followers:{followee}`/`user:following:{follower}` ZSet（score=now ms）→ Kafka 发 `user:follow-changed`（payload `{follower_id,followee_id,action:"follow"|"unfollow",created_at}`）
  - 兜底：`UserOutboxService` 本地表（user_db 建 `user_outbox` 表，结构仿 feed outbox）记录事件；Kafka 发送失败时写表，定时重发（简化：启动定时器每 10s 扫 pending 重发，上限 3 次）
  - GetFollowers/GetFollowing：先 ZSet（5min TTL）缓存读取分页（ZRANGEBYSCORE + cursor）；miss 查库回填

- [ ] **Step 6: gRPC Controller 接线**
  `UserGrpcService` 实现各 RPC，Handler 仅参数校验+协议转换，调 Service；错误码映射（`io.grpc.Status`：ALREADY_EXISTS→ALREADY_EXISTS、NOT_FOUND、UNAUTHENTICATED、INVALID_ARGUMENT、INTERNAL）。application.yml 补全：grpc.server.port=9001、DB DSN、redis、kafka bootstrap、JWT 配置。

- [ ] **Step 7: 测试通过 + 编译**
  Run: `.\mvnw test` → 全绿；`.\mvnw -q package -DskipTests` → BUILD SUCCESS
  补集成测试：`FollowIntegrationTest`（真实 H2 + mock KafkaTemplate，验证 AFTER_COMMIT 事件只在提交后发、失败入 outbox 表）。

- [ ] **Step 8: Commit**
  ```bash
  git add services/user-service
  git commit -m "feat(p2): user-service (grpc register/login/follow + JWT + kafka events)"
  ```

---

## 6. T4 Feed Service 改造（派发 deep）

**Files:**
- Modify: `services/feed-service/internal/service/post_service.go`（CreatePost 写 outbox）
- Modify: `services/feed-service/internal/repository/interfaces.go`（OutboxRepo、PostRepo.LatestByAuthor/Authors、UserClient 接口）
- Modify: `services/feed-service/internal/repository/migrate.go` / `models.go`（outbox_events 模型已建，补 OutboxRow 读写）
- Modify: `services/feed-service/internal/worker/fanout.go`（StubFanoutMode → RealFollowersMode + Kafka 消费）
- Create: `internal/worker/outbox_dispatcher.go`、`internal/worker/compensation.go`、`internal/worker/kafka_consumer.go`、`internal/kafka/client.go`（segmentio/kafka-go）
- Create: `internal/repository/outbox_repo.go`、`internal/repository/user_client.go`
- Modify: `internal/service/timeline_service.go`（作者信息/重建兜底改关注者）
- Modify: `internal/config/config.go`（+Kafka/User 地址 env）
- Test: 对应 `_test.go`

**Interfaces:**
- Consumes: T1、T2（Kafka/user_db）、user.proto（Go stub：在 services/feed-service/proto/gen 生成 userpb）
- Produces: `post:created` 事件投递 Kafka；消费 `post:created`（fanout 真实粉丝）与 `user:follow-changed`（回填/清理）；Feed 时间线反映关注关系 —— T8 验证

- [ ] **Step 1: 写失败测试**
  - `outbox_repo_test.go`：CreateInTx 写 pending、ClaimPending 只取 pending、MarkDelivered/IncrementRetry/MarkFailed 状态流转（sqlmock）
  - `fanout_test.go`（改）：RealFollowersMode 读 `user:followers:{author}` ZSet → 只向粉丝 + 作者 feed:home ZADD；无粉丝时零写入；mock 通过
  - `kafka_consumer_test.go`：follow-changed follow→回填 LatestByAuthor、unfollow→ZREM（mock redis + sqlmock）

- [ ] **Step 2: OutboxRepo**
  `OutboxRepo` 接口 + sqlmock 实现：
  - `CreateInTx(ctx, tx, topic, payloadJSON)`：INSERT outbox_events status=pending
  - `ClaimPending(ctx, limit)`：`SELECT * WHERE status='pending' ORDER BY id LIMIT ?`
  - `MarkDelivered(ctx, id)` / `IncrementRetry(ctx, id)`（retry_count+1，≥3→failed）/ `MarkFailed(ctx, id)`

- [ ] **Step 3: CreatePost 写 outbox**
  `post_service.go`：`posts.Create` 的 tx 内**追加** `outbox.CreateInTx(tx, "post:created", {post_id,user_id,content,created_at})`；**删除** `s.fanout.Enqueue(...)` 与 `fanout` 字段注入。保持返回 Post 不阻塞。

- [ ] **Step 4: Kafka 客户端 + 双 worker**
  - `internal/kafka/client.go`：segmentio/kafka-go Writer（topic=post:created）+ Reader（group feed-fanout 消费 post:created；group feed-timeline 消费 user:follow-changed）；env：`FEED_KAFKA_ADDR`(localhost:9092) `FEED_USER_ADDR`(localhost:9001)
  - `outbox_dispatcher.go`：常驻 goroutine，循环 ClaimPending(50) → Kafka 发布 → 成功 MarkDelivered / 失败 IncrementRetry；`Compensation`：5s ticker 扫 pending（创建超 30s 未投递）+ failed 重投（上限 3）
  - 失败即不丢：发布 panic/错误 → IncrementRetry；dispatch 循环错误 sleep 1s 重试

- [ ] **Step 5: Fanout 真实粉丝**
  `fanout.go`：删除 StubFanoutMode 与全用户 ListIDs 路径；新增 `RealFollowersMode`：`UserClient.GetFollowerIDs(authorID)`（读 `user:followers:{author}` ZSet，miss→gRPC GetFollowers 回填）→ 每个粉丝 + 作者自己 ZADD feed:home + EXPIRE 7d + 500 上限；阈值常量 `BigVThreshold=1000` 保留 stub（fanout 前查粉丝数，>1000 本期仍走 Push，Pull 分支留 TODO 注释+常量，不实现）。
  Kafka 消费者（feed-fanout 组）接 post:created → 解 payload → 调 fanout。

- [ ] **Step 6: 消费 user:follow-changed**
  `kafka_consumer.go`：feed-timeline 组消费 → action=follow：`LatestByAuthor(followeeID, 0, 50)` → 回填 follower feed:home（ZADD+EXPIRE+上限）；action=unfollow：查 followee 帖子 id → 从 follower feed:home ZREM。

- [ ] **Step 7: 作者信息 + 重建兜底**
  - `user_client.go`：gRPC→User Service GetProfile（懒连接 + waitForReady），带 Redis 回填 `user:profile:{id}`；批量版（时间线一次取多作者）
  - `post_service.go`/`timeline_service.go`：Post 渲染作者 display_name/avatar 改走 `user:profile:{id}` MGET → miss 批量 gRPC；**删除 join feed_db.users 逻辑**
  - timeline 重建（cache miss）：`LatestByAuthors(followingIDs, 50)`（读 `user:following:{uid}` ZSet → miss 查 gRPC GetFollowing），替代全站 Latest
  - 删除 `repository/user_repo.go`（feed_db.users 表）相关依赖；CreatePost 的 user 存在校验改走 UserClient（原 users.GetByID 删除）

- [ ] **Step 8: 全测试 + vet + build**
  Run: `& "C:\Program Files\Go\bin\go.exe" vet ./...`；`go test ./...` 全绿（P1 19 个测试按改动适配）；`go build ./...`
  > 注意：`go.mod` 加 segmentio/kafka-go、google.golang.org/grpc 新依赖需 `go mod tidy`。

- [ ] **Step 9: Commit**
  ```bash
  git add services/feed-service
  git commit -m "feat(p2): feed outbox + kafka fanout to real followers + follow-changed backfill"
  ```

---

## 7. T5 Gateway（派发 unspecified-high）

**Files:**
- Create: `services/gateway/src/grpc/user.js`、`src/routes/auth.js`、`src/routes/user.js`、`src/proto/user.proto`（复制契约）
- Modify: `src/app.js`（注册 user 路由、删 devRoutes、公开前缀）、`src/middleware/auth.js`（黑名单）、`src/config.js`（+GW_USER_ADDR/cors 3002）、`src/middleware/rate.js`（follow 发布类）
- Delete: `src/routes/dev.js`
- Test: `test/`（node:test 扩展）

**Interfaces:**
- Consumes: T1、user.proto、T3 契约（gRPC 方法名）
- Produces: 对外 REST 注册/登录/登出/资料/关注接口；JWT 校验 + 黑名单 —— T7/T8 消费

- [ ] **Step 1: 写失败测试**
  `test/auth.test.js`：register 公开可访问（无 token 200）、login 错误密码 401、logout 后旧 token 请求 → 401（黑名单）、受保护路由无 token → 401、错误码映射（User ALREADY_EXISTS→409）。
  预期 FAIL（路由不存在）。

- [ ] **Step 2: grpc/user.js + config**
  仿 `grpc/feed.js`：懒连接 + waitForReady 重连 + breaker 包裹；导出 userClient。config.js：`userAddr: strEnv('GW_USER_ADDR','localhost:9001')`；corsOrigins push `http://localhost:3002`。

- [ ] **Step 3: routes/auth.js + routes/user.js**
  auth.js：`POST /api/auth/register|login|logout`（logout 从 `request.user.jti` 取 jti 调 Logout）。user.js：`GET /api/user/:id`、`PUT /api/user/profile`、`POST/DELETE /api/user/:id/follow`、`GET /api/user/:id/followers|following`。统一 `{code,message,data}` 信封。

- [ ] **Step 4: middleware/auth.js 改造**
  - 公开前缀：`['/api/auth/register','/api/auth/login','/healthz']`（删 `/api/dev`）
  - 校验通过后：`GET auth:blacklist:{decoded.jti}` 存在 → 401；`request.user.jti = decoded.jti`；保持 sub→id、username、displayName
  - `rate.js`：POST/DELETE follow 归入发布类档位（10r/min）

- [ ] **Step 5: app.js 接线 + 删 dev.js**
  注册 createAuthRoutes/createUserRoutes；移除 createDevRoutes import 与调用；删除 `routes/dev.js`。`/api/dev/users` 与 devLogin 下线。

- [ ] **Step 6: 测试 + lint**
  Run: `npm test` 全绿（P1 41 个测试按改动适配：dev 相关删除、公开前缀断言更新）；`npm run lint`（若有）干净。

- [ ] **Step 7: Commit**
  ```bash
  git add services/gateway
  git commit -m "feat(p2): gateway auth/user routes, blacklist check, remove dev login"
  ```

---

## 8. T6 User Remote（派发 visual-engineering）

**Files:**
- Create: `frontend/user-remote/`（package.json、webpack.config.js、babel.config.js、index.html、src/bootstrap.js/index.js/styles.css）
  - `src/Auth.jsx`（登录/注册双表单）
  - `src/Profile.jsx`（个人主页 + 关注/取关按钮 + 粉丝/关注列表 Tab）
  - `src/format.js`（相对时间，复用 Feed Remote 同款）
- Test: `npm run build` 过；`npm run dev` 独立可看

**Interfaces:**
- Consumes: T1；`@b/shared` 契约（api/bus/ui）
- Produces: MF exposes `./Auth`、`./Profile`（:3002 remoteEntry.js）—— T7 Shell 挂载

- [ ] **Step 1: 工程脚手架（完全仿 feed-remote）**
  `webpack.config.js`：`name:'user'`，`exposes:{ './Auth':'./src/Auth', './Profile':'./src/Profile' }`，`devServer.port:3002`，`headers:{'Access-Control-Allow-Origin':'*'}`；shared：react/react-dom/react-router-dom/axios `{singleton:true}` + `'@b/shared':{singleton:true,import:false}`（消费 Host 实例）；babel/style 同 Feed Remote。两个暴露组件**必须各自 `import './styles.css'`**（P1 教训 8ca6ae5）。

- [ ] **Step 2: Auth.jsx**
  - 双 Tab 登录/注册。登录：account + password → `api.login` → 存 token → `bus.emit('auth:login', token)` → `useNavigate()('/home')`
  - 注册：username/display_name/email/password → `api.register` → 同上
  - 成功前 loading；错误 `ui.Toast` 显示后端 message；未登录访问受保护路由由 Shell 守卫（P1 Protected.jsx 复用）

- [ ] **Step 3: Profile.jsx**
  - 读 `:id`（无 id 且已登录 → 当前用户）→ `api.getProfile(id)`
  - 头：avatar/display_name/@username/bio + follower/following 计数（`ui.Avatar` 等）
  - 非本人：关注/取关按钮（按 is_following 状态切换）→ `api.follow/unfollow` → 乐观更新计数 + Toast
  - Tab：粉丝列表/关注列表（`api.getFollowers/getFollowing` 游标分页，复用 ui.Skeleton/EmptyState）

- [ ] **Step 4: 独立验证 + 构建**
  Run: `npm run dev`（:3002）独立浏览器检查表单/页面；`npm run build` 成功（暴露 chunk 含 remoteEntry.js）。

- [ ] **Step 5: Commit**
  ```bash
  git add frontend/user-remote
  git commit -m "feat(p2): user-remote MF (Auth login/register + Profile with follow)"
  ```

---

## 9. T7 Shell + Feed Remote 改造（派发 visual-engineering）

**Files:**
- Modify: `frontend/shell/webpack.config.js`（remotes+user、cors 无关）、`src/App.jsx`（路由）、`src/layout/Layout.jsx`（「我的」启用）、`src/shared/api-client.js`（+user 方法、-devLogin/devUsers）、`src/shared/event-bus.js`（+profile:updated）
- Delete: `frontend/shell/src/pages/Login.jsx`
- Modify: `frontend/feed-remote/src/PostCard.jsx`（作者链接）
- Test: 三仓 `npm run build`

**Interfaces:**
- Consumes: T6（user remoteEntry）
- Produces: 完整前端体验（注册/登录/个人主页/关注按钮；发帖→时间线）—— T8 验证

- [ ] **Step 1: Shell webpack + 路由**
  `remotes` 加 `user:'user@http://localhost:3002/remoteEntry.js'`；App.jsx 路由：`/login`、`/register` → `User.Auth`（Suspense+ErrorBoundary）；`/profile/:id` → `User.Profile`；移除 Login.jsx 引用；Layout「我的」→ `/profile/:id`（当前用户 id，从本地 token 解析或 api.getProfile）。

- [ ] **Step 2: api-client.js 扩展**
  ```js
  register({username,email,password,display_name}) → POST /api/auth/register → 存 token → emit('auth:login')
  login({account,password}) → POST /api/auth/login → 同上
  logout() → POST /api/auth/logout → 清 token → emit('auth:logout')
  getProfile(id) / updateProfile(patch) / follow(id) / unfollow(id)
  getFollowers({id,cursor,limit}) / getFollowing({id,cursor,limit})
  ```
  **删除 devLogin/devUsers**；401 拦截逻辑保留（logout 时不触发重定向死循环——登出清 token 后拦截需豁免）。

- [ ] **Step 3: event-bus + 导航**
  `event-bus.js` 常量 +`profile:updated`；登录态由 auth:login/logout 驱动；Profile 更新资料后 emit。

- [ ] **Step 4: Feed Remote PostCard 作者链接**
  PostCard 作者名/头像包 `<Link to={'/profile/'+post.user_id}>`（react-router Link 可用，Shell 提供 Router）。

- [ ] **Step 5: 三仓 build 验证**
  Run: shell `npm run build`、feed-remote `npm run build`、user-remote `npm run build` 全成功。删除 Login.jsx 后确认无残留 import。

- [ ] **Step 6: Commit**
  ```bash
  git add frontend/shell frontend/feed-remote
  git commit -m "feat(p2): shell mounts user remote (auth/profile), drop dev login; feed author links"
  ```

---

## 10. T8 集成与端到端（控制器 + 修复派发）

**Files:**
- Modify: `infra/demo-e2e.ps1`（P2 扩展）
- Modify: `infra/README.md`（启动 6 服务步骤确认）
- 修复派发：各任务遗留问题

**Interfaces:**
- Consumes: T2-T7 全部
- Produces: 验收证据（e2e 全 PASS + 浏览器实测）

- [ ] **Step 1: 全栈启动**
  按 infra/README 顺序：`docker compose up -d`（mysql/redis/kafka）→ user-service（`.\mvnw spring-boot:run`）→ feed-service（go run）→ gateway（npm run dev）→ user-remote（:3002）→ shell（:3000）+ feed-remote（:3001）。确认 6 个进程端口监听。

- [ ] **Step 2: e2e 脚本扩展（P2 断言）**
  在既有 8 步基础上扩展：
  1. healthz PASS（沿用）
  2. **注册**新用户（随机用户名）→ 201/200 拿 token
  3. **登录** bob（Password123!）→ token
  4. bob **关注** 新用户（POST follow）
  5. 新用户**发帖** → bob 时间线**可见**该帖（fanout 经 Kafka）
  6. 未关注者（carol）时间线**不可见**该帖
  7. **取关** → bob 时间线该帖被清理（follow-changed 消费）
  8. 登出 → 旧 token 请求受保护接口 → 401（黑名单）
  9. `go vet/build`、`mvn test`、gateway `npm test`、前端三仓 build 全过（脚本尾部汇总）
  脚本用 PowerShell `Invoke-RestMethod` + `Select-String` 断言，输出 PASS/FAIL（沿用 P1 demo-e2e.ps1 结构）。等待 Kafka 传播：断言前 `Start-Sleep 1`。

- [ ] **Step 3: 浏览器验收**
  agent-browser：打开 :3000 → 注册 → 登录 → 发帖 → 时间线即时出现 → 打开新用户主页 → bob 关注 → 回首页看回填 → 取关看清理。`eval` 传 JS 需 base64（P2-HANDOFF §5 方法）。截图存 `D:\Personal\Temp\opencode\b-shots`，DOM innerText 验收。

- [ ] **Step 4: 修复循环**
  e2e/浏览器发现的问题 → 派发对应任务补丁（用 T3/T4/T5/T6 的 continuation session）→ 重跑直至全 PASS。

- [ ] **Step 5: Commit**
  ```bash
  git add infra/demo-e2e.ps1 infra/README.md
  git commit -m "test(p2): extend e2e for register/login/follow/kafka fanout/blacklist"
  ```

---

## 11. T9 终审（派发）

- [ ] **Step 1: 对照设计文档逐条复核**
  设计文档 `docs/superpowers/specs/2026-08-01-p2-account-kafka-design.md` §1-§10：完成标志、偏差记录 D-A1..A8 逐条核对实现与记录一致。
- [ ] **Step 2: 质量门禁**
  `go vet ./...`、`go build ./...`、`go test ./...`、`.\mvnw test`、gateway `npm test`、前端三仓 `npm run build`、e2e 全 PASS。
- [ ] **Step 3: 全分支评审**
  设计评审 + 代码评审（auth 安全、outbox 可靠性、fanout 幂等、Redis 键一致）。评审意见修复后合并 `main` 并推送。

---

## 12. 完成标志（验收标准，全部可测）

1. `docker compose up` 后 MySQL/Redis/**Kafka** 健康；user_db 建表 + 种子 4 用户（密码 Password123! 可登录）
2. 注册/登录/登出/关注/取关端到端可用（e2e + 浏览器实测）
3. **发帖走 Kafka**：新帖 → outbox pending → Kafka `post:created` → fanout → **粉丝**时间线出现；**非粉丝不可见**
4. 关注 → 回填作者近期帖子；取关 → 时间线清理（消费 `user:follow-changed`）
5. 登出后旧 token → 401（`auth:blacklist:{jti}` 生效）
6. `infra/demo-e2e.ps1` 全 PASS；`go vet/build/test`、`mvn test`、gateway `npm test`、前端三仓 build 全绿
7. dev 登录（/api/dev/*）全部下线；无 feed_db.users 残留引用
8. 偏差 D-A1..A8 记录于设计文档 §10 且评审知悉
