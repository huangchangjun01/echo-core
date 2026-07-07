# Echo Core

> Go 后端服务，负责用户鉴权、文件存储、对话透传。AI/记忆/RAG 等能力由外部 `echo-ai`（Python）服务承担。

---

## 一、功能模块

| 模块 | 职责 |
| --- | --- |
| 用户与会话 | 注册 / 登录 / 账号校验 / 会话查询 / 注销，bcrypt+salt 存密码，进程内 SessionStore（24h 滑动） |
| 文件存储 | 七牛云上传 Token 生成 + 文件元数据登记；登记后异步触发 Python `/ingest_file` 入 RAG |
| 对话透传 | `POST /api/chat` 透传到 Python `/chat`；支持同步 JSON 与 SSE 流式两种形态 |
| 公共 | 健康探针 `/health`、RequestID、CORS、AccessLog、鉴权中间件 |

---

## 二、技术栈

| 类别 | 选型 |
| --- | --- |
| Web 框架 | Gin `v1.12.0` |
| ORM | GORM `v1.31.1` + MySQL Driver `v1.6.0` |
| 对象存储 | 七牛云 Go SDK `v7.26.4` |
| 工具库 | `godotenv`（配置）、`google/uuid`（key 命名） |
| 外部服务 | `echo-ai`（Python，`/chat`、`/ingest_file`） |

---

## 三、目录结构

```
echo-core/
├── main.go                       # 入口：加载 .env、初始化 DB、注册路由、启动 Gin
├── go.mod / go.sum
├── .env                          # 本地环境变量
│
├── config/                       # 全局配置
│   └── database.go               # MySQL 初始化 + 自动迁移
│
├── routes/
│   └── router.go                 # 聚合 user / file / chat 三个子路由
│
├── handlers/                     # HTTP 处理器
│   ├── user_handler.go           #   注册 / 登录 / 会话
│   ├── file_handler.go           #   上传 Token / 文件登记
│   ├── chat_stream_handler.go    #   对话同步 + SSE 流式
│   └── health_handler.go         #   /health 探针
│
├── service/                      # 业务服务层
│   ├── user_service.go
│   ├── file_service.go
│   ├── chat_service.go
│   └── request/                  #   服务层入参 DTO
│
├── repository/                   # 仓储层（GORM 封装）
│   ├── user_repository.go
│   └── file_repository.go
│
├── models/                       # 数据库模型
│   ├── user.go
│   └── file.go
│
├── dto/                          # 对外 DTO（请求/响应）
│   ├── user_dto.go
│   ├── chat_dto.go
│   ├── ingest_dto.go
│   └── health_dto.go
│
├── remote/                       # 外部服务 HTTP 客户端
│   ├── python_chat_client.go     #   透传 Python /chat（同步 + SSE）
│   └── python_ingest_client.go   #   透传 Python /ingest_file
│
├── middleware/
│   ├── request_id.go             # 注入 X-Request-Id
│   ├── auth.go                   # 会话校验（Header/Cookie/Body 三路取 sid）
│   ├── cors.go
│   └── access_log.go             # 统一访问日志
│
└── utils/
    ├── logger.go                 # LogWithCtx / LogStartup / LogAccess
    ├── session.go                # SessionStore 接口 + 内存实现 + 单例
    ├── session_store.go          # 单例初始化 / 优雅关停
    ├── password.go               # bcrypt 封装
    └── system_config.go          # GetEnv
```

---

## 四、路由清单

| Method | Path | 鉴权 | 说明 |
| --- | --- | --- | --- |
| GET | `/health` | 否 | 存活探针，返回 `{status, version}` |
| POST | `/api/auth/register` | 否 | 注册账号 |
| POST | `/api/auth/checkAccount` | 否 | 账号占用校验 |
| POST | `/api/auth/login` | 否 | 登录，返回 `sessionId` |
| POST | `/api/auth/check` | 否 | 校验会话有效性 |
| POST | `/api/auth/logout` | 否 | 注销会话 |
| POST | `/api/file/token` | 是 | 获取七牛云上传 Token |
| POST | `/api/file/register` | 是 | 登记文件元数据（事务 + 触发 RAG 入库） |
| POST | `/api/chat` | 是 | 对话（`stream=false` 同步 JSON / `stream=true` SSE） |

**Session 传递**：中间件按优先级 `Header X-Session-Id` → `Cookie session_id` → 请求体 `sessionId` 取值。

---

## 五、数据表

只有 2 张表，启动时由 GORM 自动迁移。

| 表名 | 说明 |
| --- | --- |
| `user` | 账号、密码哈希、盐值、昵称、邮箱、状态、最近登录信息 |
| `file` | 文件名、七牛 key、userId、类型、状态、config JSON |

> 历史版本曾存在 `session_message` / `user_memory` / `conversation_summary` / `agent_config` 等表，已随 Agent/记忆/摘要 模块一起下线，模型文件已删除，AutoMigrate 不再注册。

---

## 六、核心链路

### 1. 对话（同步）

```
Client ──POST /api/chat {stream=false}──▶ ChatHandler
  │  中间件校验 sid → 注入 userId → 覆盖请求体里的 userId（防冒用）
  ▼
ChatService.ChatSync ──POST──▶ Python /chat
  ◀──────── ChatSyncResponse{ reply, events[], latencyMs }
ChatHandler ── 200 JSON ──▶ Client
```

### 2. 对话（SSE 流式）

```
Client ──POST /api/chat {stream=true}──▶ ChatHandler
  │  写响应头 text/event-stream + 立刻 flush 一条 ": connected" 注释帧
  ▼
ChatService.ChatStream ──POST (Accept: text/event-stream)──▶ Python /chat
  │
  │   每帧 data: {...} 携带 ChatEvent，6 类事件由 Python 协议透传：
  │     context / tool / prefix / delta / done / memory_extracted
  │
  ▼
writeSSEEvent(seq, ev) ── event:<type>\nid:<seq>\ndata:<json>\n\n ──▶ Client
  │
  ▼
流结束 / 出错 → 关闭连接
```

### 3. 文件登记

```
Client ──POST /api/file/token──────▶ FileService.GetUploadToken
                                          │ 生成 key = bizType/YYYYMMDD/uuid.ext
                                          ▼
                                    { token, uploadURL, key }

Client ──直传──────────────────────────────────────────────▶ 七牛 OSS

Client ──POST /api/file/register──▶ FileService.RegisterFile (Tx)
                                          │ ① Insert file row
                                          │ ② Commit
                                          │ ③ 生成私有访问 URL (24h)
                                          │ ④ POST Python /ingest_file（失败仅记日志，不影响主返回）
                                          ▼
                                    { id, userId, key, status, ingestion? }
```

### 4. 登录与会话

```
Client ──POST /api/auth/login──▶ UserService.Login
                                    │ ① GetByUsername → 不区分"账号不存在/密码错"
                                    │ ② IsEnabled
                                    │ ③ VerifyPassword (bcrypt+salt)
                                    │ ④ SessionStore.Create (24h)
                                    │ ⑤ UpdateLastLogin
                                    ▼
                              { sessionId, expireAt, user }

后续请求带 sid 即可，RequireSession 中间件命中后自动注入 userId。
```

---

## 七、环境变量

复制 `.env` 后按需修改。

| 变量 | 必填 | 默认 | 说明 |
| --- | --- | --- | --- |
| `APP_PORT` | 否 | `8080` | HTTP 监听端口 |
| `APP_MODE` | 否 | `debug` | Gin 模式（debug/release/test） |
| `DB_HOST` | 是 | - | MySQL Host |
| `DB_PORT` | 否 | `3306` | MySQL 端口 |
| `DB_USER` / `DB_PASSWORD` / `DB_NAME` | 是 | - | 数据库连接信息 |
| `QINIU_ACCESS_KEY` | 是* | - | 七牛云 AccessKey（文件功能需要） |
| `QINIU_SECRET_KEY` | 是* | - | 七牛云 SecretKey |
| `QINIU_BUCKET_NAME` | 是* | - | 七牛云 存储空间 |
| `QINIU_DOMAIN` | 是* | - | 七牛云 访问域名 |
| `ECHO_AI_REMOTE_BASE_URL` | 否 | `http://localhost:8000` | Python 服务地址 |
| `JWT_SECRET` | 否 | - | 预留（当前未使用） |
| `LLM_*` | 否 | - | 预留（当前对话完全由 Python 处理） |

> 标 `*` 的变量只在调用文件上传相关接口时才需要，缺失时 `FileService` 会返回明确错误。

---

## 八、本地启动

```bash
# 1. 安装依赖
go mod tidy

# 2. 配置 .env（参考第七节）

# 3. 确认外部依赖
#    - MySQL 可连
#    - Python 服务 echo-ai 已启动（默认 :8000），并实现了 /chat 与 /ingest_file

# 4. 启动
go run main.go
# 或运行产物
./echo-core.exe
```

启动横幅会打印关键配置（端口、DB、Python baseURL），便于核对环境。

```
==== [config] env=debug port=8080 db=root@tcp(...:3306)/xxx pythonBase=http://localhost:8000 qiniu=see QINIU_* envs ====
==== [db]      host=... port=3306 name=xxx user=root slowThreshold=1s logLevel=info ====
==== [server]  listen=:8080 version=echo-core ====
```

---

## 九、日志约定

所有业务日志统一通过 `utils.LogWithCtx(ctx, "<Component>", "msg | key=value")` 输出，格式：

```
[rid=<requestId> uid=<userId>] [<Component>] msg | key=value
```

- `rid` 由 `middleware/request_id.go` 注入，缺失时显示为 `empty`
- `uid` 由 `middleware/auth.go` 注入
- 整个请求链路（Handler → Service → Remote）按 `rid` 串接，便于 grep 排障

调试 Python 真实返回时，重点看 `[PythonChatClient.ChatStreamEvents]` 分组的 `SSE 帧 #N type=<...>` 行。