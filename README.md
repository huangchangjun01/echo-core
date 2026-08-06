# Echo Core

> Go 后端服务，负责用户鉴权、角色管理、文件存储、记忆管理、对话透传。AI 解析 / RAG / 向量召回等能力由外部 `echo-ai`（Python）服务承担。

---

## 一、功能模块

| 模块 | 职责 |
| --- | --- |
| 用户与会话 | 注册 / 登录 / 账号校验 / 会话查询 / 注销，bcrypt+salt 存密码，进程内 SessionStore（24h 滑动） |
| 角色管理 | CRUD 角色（至少保留 1 个默认角色），所有记忆/对话按角色隔离 |
| 文件存储 | 七牛云上传 Token 生成 + 文件元数据登记 + 记忆管理（列表 / 改描述 / 纯文本 / 二进制下载） |
| 回忆记忆 | 多模态源文件 + AI 解析生成的 md = 一条"记忆主题"。申请 memoryId / 主题唯一性 / 记忆内上传 token / Save / Update（按需重解析）/ 列表 / 详情 / 删源文件 / 删主题 |
| 对话透传 | `POST /api/chat` 透传到 Python `/chat`；支持同步 JSON 与 SSE 流式两种形态 |
| 公共 | 健康探针 `/health`、RequestID、CORS、AccessLog、鉴权中间件 |

---

## 二、技术栈

| 类别 | 选型 |
| --- | --- |
| Web 框架 | Gin `v1.12.0` |
| ORM | GORM `v1.31.1` + MySQL Driver `v1.6.0` |
| 对象存储 | 七牛云 Go SDK `v7.26.4` |
| 工具库 | `godotenv`（配置）、`google/uuid`（key/memoryId 命名） |
| 外部服务 | `echo-ai`（Python，`/chat`、`/ingest_file`、`/memory/*`） |

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
│   └── router.go                 # 聚合 user / role / file / chat / memory 五个子路由
│
├── handlers/                     # HTTP 处理器
│   ├── user_handler.go           #   注册 / 登录 / 会话
│   ├── role_handler.go           #   角色 CRUD
│   ├── file_handler.go           #   上传 Token / 登记 / 记忆管理列表 / 改描述 / 纯文本 / 下载
│   ├── recall_memory_handler.go  #   回忆记忆（8 业务 + 4 内部接口）
│   ├── chat_stream_handler.go    #   对话同步 + SSE 流式
│   └── health_handler.go         #   /health 探针
│
├── service/                      # 业务服务层
│   ├── user_service.go
│   ├── role_service.go
│   ├── file_service.go           #   包含 ListMemoryFiles / DownloadFile / CreateTextMemory
│   ├── qiniu_service.go          #   七牛对象删除（DeleteObject / DeleteByPrefix）
│   ├── recall_memory_service.go  #   回忆记忆主流程：Save / Update / Delete / 状态机
│   ├── chat_service.go
│   ├── recall_filetype_test.go   #   fileType 权威归一回归测试
│   └── request/                  #   服务层入参 DTO
│       ├── user_request.go
│       ├── file_request.go
│       └── memory_request.go     #   回忆记忆入参
│
├── repository/                   # 仓储层（GORM 封装）
│   ├── user_repository.go
│   ├── role_repository.go
│   ├── file_repository.go
│   └── recall_memory_repository.go
│
├── models/                       # 数据库模型
│   ├── user.go
│   ├── role.go
│   ├── file.go
│   ├── recall_memory.go          #   RecallMemory / RecallSourceFile
│   └── (历史) user_memory / conversation_summary / agent_config 已下线
│
├── dto/                          # 对外 DTO（请求/响应）
│   ├── user_dto.go
│   ├── role_dto.go
│   ├── file_dto.go
│   ├── chat_dto.go
│   ├── ingest_dto.go
│   ├── health_dto.go
│   └── memory_dto.go             #   /memory/* 接口出入参
│
├── remote/                       # 外部服务 HTTP 客户端
│   ├── python_chat_client.go     #   透传 Python /chat（同步 + SSE）
│   ├── python_ingest_client.go   #   透传 Python /ingest_file
│   └── python_memory_client.go   #   透传 Python /memory/*（parse / reparse / source/delete / delete）
│
├── middleware/
│   ├── request_id.go             # 注入 X-Request-Id
│   ├── auth.go                   # 会话校验（Header/Cookie/Body 三路取 sid）
│   ├── cors.go
│   └── access_log.go             # 统一访问日志
│
└── utils/
    ├── logger.go                 # LogWith / LogWithCtx / LogStartup / LogAccess
    ├── session.go                # SessionStore 接口 + 内存实现 + 单例
    ├── session_store.go          # 单例初始化 / 优雅关停
    ├── password.go               # bcrypt 封装
    └── system_config.go          # GetEnv
```

---

## 四、路由清单

### 公开路由

| Method | Path | 鉴权 | 说明 |
| --- | --- | --- | --- |
| GET | `/health` | 否 | 存活探针，返回 `{status, version}` |
| POST | `/api/auth/register` | 否 | 注册账号 |
| POST | `/api/auth/checkAccount` | 否 | 账号占用校验 |
| POST | `/api/auth/login` | 否 | 登录，返回 `sessionId` |
| POST | `/api/auth/check` | 否 | 校验会话有效性 |
| POST | `/api/auth/logout` | 否 | 注销会话 |

### 角色（需鉴权）

| Method | Path | 鉴权 | 说明 |
| --- | --- | --- | --- |
| POST | `/api/role` | 是 | 创建角色 |
| GET | `/api/role` | 是 | 列出当前用户的角色（无角色自动建默认） |
| PUT | `/api/role/:id` | 是 | 修改角色 |
| DELETE | `/api/role/:id` | 是 | 删除角色（至少保留 1 个） |

### 文件（需鉴权）

| Method | Path | 鉴权 | 说明 |
| --- | --- | --- | --- |
| POST | `/api/file/token` | 是 | 获取七牛云上传 Token（通用目录） |
| POST | `/api/file/register` | 是 | 登记文件元数据（事务 + 触发 RAG 入库） |
| GET | `/api/file/list` | 是 | 记忆管理：按 roleId/fileType 列出当前用户的文件 |
| PUT | `/api/file/:id/desc` | 是 | 记忆管理：修改文件描述（同步触发 RAG 重新入库） |
| POST | `/api/file/text` | 是 | 记忆管理：新建纯文本记忆（desc 直接入 RAG） |
| GET | `/api/file/:id/download` | 是 | 记忆管理：下载文件二进制流（走七牛 SDK 源站 API） |

### 回忆记忆（需鉴权）

| Method | Path | 鉴权 | 说明 |
| --- | --- | --- | --- |
| POST | `/api/memory/apply` | 是 | 申请 memoryId（去横线 UUID） |
| POST | `/api/memory/check-topic` | 是 | 校验 (userId, roleId, topic) 是否已存在 |
| POST | `/api/memory/upload-token` | 是 | 记忆目录内上传 token（强制落在 `memory/{userId}/{roleId}/{memoryId}/`） |
| POST | `/api/memory/save` | 是 | 保存记忆：主题 + 批量源文件落库，事务提交后异步触发 `/memory/parse` |
| POST | `/api/memory/update` | 是 | 编辑记忆：服务端以"DB 旧值 vs 请求新值"为权威 diff，按需触发 `/memory/reparse` |
| GET | `/api/memory/list` | 是 | 记忆列表（按 roleId 过滤） |
| GET | `/api/memory/detail` | 是 | 记忆详情（含源文件下载链接 + md 内容缓存 + md 下载链接） |
| DELETE | `/api/memory/file` | 是 | 删除单个源文件（硬删记录 + 异步删对象 + 通知 AI 更新 md/向量） |
| DELETE | `/api/memory/theme` | 是 | 删除整个主题（硬删记录 + 异步删目录 + 通知 AI 删向量） |

### 回忆记忆 · 内部接口（echo-ai 回调，无 session，靠 `X-Internal-Token` 校验）

| Method | Path | 鉴权 | 说明 |
| --- | --- | --- | --- |
| POST | `/api/memory/md-url` | Token | 兼容保留（已废弃，返回"deprecated"） |
| POST | `/api/memory/md-content` | Token | echo-ai 直接拉 md 正文（绕开 CDN 421，从 `md_content` 缓存读） |
| POST | `/api/memory/md-content/save` | Token | echo-ai 把解析后的 md 写回 DB（缓存以供后续使用） |
| POST | `/api/memory/internal/parse-status` | Token | echo-ai 解析完成/失败时回调，写 `parse_status`（2/3） |

### 对话（需鉴权）

| Method | Path | 鉴权 | 说明 |
| --- | --- | --- | --- |
| POST | `/api/chat` | 是 | 对话（`stream=false` 同步 JSON / `stream=true` SSE） |

**Session 传递**：中间件按优先级 `Header X-Session-Id` → `Cookie session_id` → 请求体 `sessionId` 取值。

---

## 五、数据表

启动时由 GORM 自动迁移，共 5 张表。

| 表名 | 说明 |
| --- | --- |
| `user` | 账号、密码哈希、盐值、昵称、邮箱、状态、最近登录信息 |
| `role` | 角色（用户维度，至少保留 1 个默认） |
| `file` | 文件名、七牛 key、userId、roleId、类型、状态、描述、config JSON |
| `recall_memory` | 回忆记忆主题：memoryId（去横线 UUID）、userId、roleId、topic、主观描述、md_key、md_content 缓存、parse_status / edit_status / status |
| `recall_source_file` | 回忆记忆的源文件：fileKey、fileName、fileType（1文本/2图片/3视频/4音频） |

`recall_memory` 关键字段：
- `memory_id`：`uniqueIndex`（去横线 UUID）
- `(user_id, role_id)`：`idx_recall_user_role` 索引
- `parse_status`：0 待解析 / 1 解析中 / 2 完成 / 3 失败
- `edit_status`：0 空闲可编辑 / 1 AI 正在写入 md（编辑锁）
- `status`：1 可用 / 2 已删除（软删）

`recall_source_file` 通过 `memory_id` 关联到主题，多模态可同时挂多个源文件。

> 历史版本曾存在 `session_message` / `user_memory` / `conversation_summary` / `agent_config` 等表，已随 Agent/记忆/摘要模块一起下线，模型文件已删除，AutoMigrate 不再注册。

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

### 3. 文件登记 + 记忆管理

```
Client ──POST /api/file/token──────▶ FileService.GetUploadToken
                                          │ key = bizType/YYYYMMDD/uuid.ext
                                          ▼
                                    { token, uploadURL, key, domain }

Client ──直传──────────────────────────────────────────────▶ 七牛 OSS

Client ──POST /api/file/register──▶ FileService.RegisterFile (Tx)
                                          │ ① Insert file row
                                          │ ② Commit
                                          │ ③ 生成可访问 URL
                                          │ ④ POST Python /ingest_file（失败仅记日志）
                                          ▼
                                    { id, userId, key, status, url, ingestion? }

Client ──GET /api/file/list?roleId=&fileType=──▶ FileService.ListMemoryFiles
Client ──PUT /api/file/:id/desc──▶ FileService.UpdateDesc （触发重新入库）
Client ──POST /api/file/text──▶ FileService.CreateTextMemory （纯文本入 RAG）
Client ──GET /api/file/:id/download──▶ FileService.DownloadFile （七牛 SDK 源站 API）
```

### 4. 回忆记忆（核心流程）

```
Client ──POST /api/memory/apply──────────────────────▶ RecallMemoryService.ApplyMemoryId
                                                            ▼
                                                      { memoryId }

Client ──POST /api/memory/check-topic {roleId,topic}─▶ CheckTopicExists
                                                            ▼
                                                      { exists }

Client ──POST /api/memory/upload-token─────────────▶ GetMemoryUploadToken
  │  源文件：key = memory/{userId}/{roleId}/{memoryId}/{uuid}{ext} (InsertOnly)
  │  md 文件(isMd)：key = memory/{userId}/{roleId}/{memoryId}/{memoryId}.md（允许覆盖）
  ▼
{ token, uploadURL, key, domain }

Client ──直传─────────────────────────────────────────────────────▶ 七牛 OSS

Client ──POST /api/memory/save─────────────────────▶ RecallMemoryService.SaveMemory (Tx)
                                                            │ ① 主题唯一性强校验
                                                            │ ② Insert recall_memory
                                                            │ ③ BatchCreateSourceFile
                                                            │ ④ Commit
                                                            │ ⑤ 异步 fire-and-forget：
                                                            │     - UpdateParseStatus → RUNNING
                                                            │     - POST Python /memory/parse（3 次指数退避重试）
                                                            │     - 失败 → FAILED(3)
                                                            ▼
                                                      { memoryId, ... }

                  ┌──────────── echo-ai 异步 ────────────┐
                  │  parse_memory() 完成后                │
                  │  - mark_done(parse_status=2) 直写 DB │
                  │  - 或回调 POST /api/memory/internal/parse-status
                  │  - 把 md 上传对象存储 + POST /api/memory/md-content/save 缓存到 DB
                  └────────────────────────────────────────┘

Client ──POST /api/memory/update {memoryId, topic, subjectiveDesc,
                                  sourceFiles[], needReparse}──▶ UpdateMemory (Tx)
                                                            │ 1. 服务端"权威 diff"：
                                                            │    topicChanged / descChanged / filesChangedAuthoritative
                                                            │ 2. 即使 needReparse=true，权威无改动则不触发 AI
                                                            │ 3. 事务内更新主题/描述 + 源文件 diff
                                                            │ 4. 仅在 authoritativeNeedReparse 时，
                                                            │    异步 POST Python /memory/reparse（带 removedFileKeys）
                                                            ▼
                                                      { 最新记忆项 }
```

### 5. 回忆记忆 · 删除链路

```
DELETE /api/memory/file {memoryId, fileKey}
  │
  ▼ RecallMemoryService.DeleteSourceFile (Tx)
      │ ① HardDeleteSourceWithTx
      │ ② Commit
      │ ③ 异步：
      │   - DeleteObject(fileKey)         # 七牛对象删除
      │   - POST Python /memory/source/delete
      ▼

DELETE /api/memory/theme {memoryId}
  │
  ▼ RecallMemoryService.DeleteMemoryTheme (Tx)
      │ ① HardDeleteAllSourceWithTx
      │ ② HardDeleteWithTx
      │ ③ Commit
      │ ④ 异步：
      │   - DeleteByPrefix(memory/{userId}/{roleId}/{memoryId}/)
      │   - POST Python /memory/delete
      ▼
```

### 6. 登录与会话

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
| `QINIU_ACCESS_KEY` | 是* | - | 七牛云 AccessKey（文件/记忆功能需要） |
| `QINIU_SECRET_KEY` | 是* | - | 七牛云 SecretKey |
| `QINIU_BUCKET_NAME` | 是* | - | 七牛云 存储空间 |
| `QINIU_DOMAIN` | 是* | - | 七牛云 访问域名 |
| `ECHO_AI_REMOTE_BASE_URL` | 否 | `http://localhost:8000` | Python 服务地址 |
| `ECHO_CORE_INTERNAL_TOKEN` | 否 | - | 内部接口（`/api/memory/*` 内部）共享 Token；非空时校验 `X-Internal-Token`，空则开放（仅内网） |
| `JWT_SECRET` | 否 | - | 预留（当前未使用） |
| `LLM_*` | 否 | - | 预留（当前对话/记忆解析完全由 Python 处理） |

> 标 `*` 的变量只在调用文件/记忆相关接口时才需要，缺失时 `FileService` / `QiniuService` 会返回明确错误。

---

## 八、本地启动

```bash
# 1. 安装依赖
go mod tidy

# 2. 配置 .env（参考第七节）

# 3. 确认外部依赖
#    - MySQL 可连
#    - Python 服务 echo-ai 已启动（默认 :8000），并实现了 /chat / /ingest_file / /memory/*

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

单元测试：

```bash
go test ./service -run 'TestResolveSourceFileType|TestInferFileTypeByExt'
```

---

## 九、日志约定

所有业务日志统一通过 `utils.LogWith(ctx, "<Component>", "msg | key=value")` 或 `utils.LogWithCtx(ctx, "<Component>", "msg | key=value")` 输出，格式：

```
[rid=<requestId> uid=<userId>] [<Component>] msg | key=value
```

- `rid` 由 `middleware/request_id.go` 注入，缺失时显示为 `empty`
- `uid` 由 `middleware/auth.go` 注入
- 整个请求链路（Handler → Service → Remote）按 `rid` 串接，便于 grep 排障

调试 Python 真实返回时，重点看 `[PythonChatClient.ChatStreamEvents]` 分组的 `SSE 帧 #N type=<...>` 行；回忆记忆链路则看 `[RecallMemoryService]` / `[PythonMemoryClient/memory/parse]`。

---

## 十、典型排障

| 现象 | 关注日志 | 根因 / 处理 |
| --- | --- | --- |
| 列表里某条记忆永远 "待解析" | `[RecallMemoryService.triggerParseAsync] 置 RUNNING 失败` | DB 更新失败；检查 MySQL 连接 |
| 列表里某条记忆 "解析失败" | `[PythonMemoryClient/memory/parse] HTTP 请求失败` / `Python 返回非 200` | echo-ai 不可达；检查 `ECHO_AI_REMOTE_BASE_URL` 与 Python 进程 |
| md 详情返回 "md 内容未缓存（早期数据）" | `[Recall] MdContent from db` 命中但 `MdContent==""` | 旧记录未走 `md-content/save` 缓存；可让 echo-ai 重新解析补齐 |
| 浏览器 fetch 文件出现 NetworkError | `[FileService.GetPublicURL]` 域名可达性 | 七牛 CDN 域名可能下线，改用 `/api/file/:id/download`（直走源站 API） |
| 编辑记忆后 md 退化成 "MP4 容器元数据" | `[RecallMemoryService] fileType 纠正` 应有命中 | 前端把 mp4 错标为文本；服务端已用 `resolveSourceFileType` 兜底（见 `service/recall_filetype_test.go`） |