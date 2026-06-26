# Echo Core 重构报告

## 概述

本次重构目的：简化业务架构，删除不必要的复杂功能（多层级记忆、ReAct/Agent 编排、RAG 检索、对话摘要、前缀缓存、WebSocket 等），将服务精简为只保留 **用户管理**、**文件存储** 和 **轻量级流式聊天** 三大核心能力，聊天主链路改为直接调用外部 Python 服务（流式 SSE 透传）。

- 重构分支：`echo_refactor`
- 重构基准：`master`

---

## 第一部分：删除的功能

### 1. Agent 模块（整个 `agent/` 目录）

**删除前功能**：

- `agent/react_engine.go`：实现 ReAct 引擎，支持多轮"推理→工具调用→观察→推理"循环，处理 Agent 编排
- `agent/tools.go`：定义工具集，包含默认工具（`get_weather`、`calculate`、`get_time`）和 RAG 搜索工具（`search_knowledge`、`web_search`）；同时内置 RAGClient 封装
- `agent/react_engine_test.go`：ReAct 引擎的单元测试代码
- 支持 MultiAgent 编排：根据用户问题关键词路由到不同 Agent（`default` / `search`）
- 支持工具调用结果回填到上下文，支持流式 ReAct 执行（`ExecuteStream`）
- 提供 `get_weather` / `calculate` / `get_time` / `search_knowledge` 等可注册工具

**删除原因**：业务简化为直接调用外部 Python 服务完成对话，不再需要 Agent 编排、ReAct 循环、工具调用与路由决策逻辑。

**删除文件**：

- `agent/react_engine.go`
- `agent/react_engine_test.go`
- `agent/tools.go`
- `agent/` 目录（整体删除）

---

### 2. 多层级记忆服务（短期/长期/摘要）

**删除前功能**：

- `service/memory_service.go`：用户长期记忆服务，负责记忆加载、上下文构建、异步抽取、合并去重
  - `BuildMemoryContext`：跨会话拼接长期记忆到 system prompt
  - `ExtractAndSave`：从用户/助手消息中抽取"偏好/事实/知识"
  - `ExtractAsync`：异步 goroutine 抽取，避免阻塞聊天响应
  - `isDuplicate`：基于 `strings.Contains` 的语义去重（避免重复记忆膨胀）
- `service/summarizer_service.go`：对话摘要服务，增量摘要 + 滑动窗口
  - `ShouldSummarize`：消息数 > 触发阈值时启动摘要
  - `GenerateSummary`：把"旧摘要 + 新增消息"调用 LLM 重新生成摘要
  - `BuildContext`：将摘要注入前缀上下文
  - 控制 token 增长，避免上下文爆炸
- `service/prompt_cache.go`：提示词前缀缓存
  - 缓存拼装好的前缀字符串，减少重复拼装开销
  - key 涵盖 user / session / mem（长期记忆 hash）/ sumv（摘要版本）/ model，使上游 LLM 的前缀缓存命中率最大化
  - 支持按 key TTL 失效（5 分钟）

**删除原因**：业务不再需要跨会话长期记忆、摘要压缩、前缀缓存等复杂功能；聊天主链路改为由 Python 服务管理上下文。

**删除文件**：

- `service/memory_service.go`
- `service/summarizer_service.go`
- `service/prompt_cache.go`

---

### 3. RAG 相关远程调用

**删除前功能**：

- `remote/vector_remote.go`：调用 Python `echo-ai` 服务获取向量嵌入（文本 / 图片）、文件向量化入库（`IngestFile`）、按 `Metadata.SourceURL` 拼接七牛云完整下载 URL（`buildFullURL`）
- `remote/request/ingest_request.go`：定义文件入库请求结构 `IngestFileRequest`
- `remote/response/embedding_response.go`：定义向量响应结构 `EmbeddingResponse`

**删除原因**：不再需要 RAG 检索、向量化入库功能；文件注册也不再触发向量入库。

**删除文件**：

- `remote/vector_remote.go`
- `remote/request/` 目录（含 `ingest_request.go`）
- `remote/response/` 目录（含 `embedding_response.go`）

---

### 4. 直接调用 LLM 的 AI 客户端（同步删除）

**删除前功能**：

- `remote/ai_client.go`：直接对接 OpenAI 兼容协议的 AI 客户端，包含：
  - `Chat` / `ChatWithToolChoice`：同步调用 chat/completions
  - `ChatStream`：流式调用 chat/completions，解析 SSE 帧
  - `ChatStreamWithTools`：流式 + 工具调用累积
  - `GenerateSummary`：基于 LLM 生成对话摘要
  - `GetTextEmbedding`：调用 LLM embedding 接口
  - 提供 `AIChatMessage` / `AITool` / `AIToolCall` 等 OpenAI 兼容结构体

**删除原因**：重构后不再直连 LLM，所有 LLM 调用收敛到 Python 服务侧；客户端侧不再需要 OpenAI 协议解析、工具调用编排、embedding 调用等能力。

**删除文件**：

- `remote/ai_client.go`

---

### 5. WebSocket 聊天与 ws 测试工具

**删除前功能**：

- `handlers/chat_stream_handler.go` 内的 `ChatHandleWS`、`handleChatMessage`、`WSIncomingMessage` / `WSOutgoingMessage` 等：基于 gorilla/websocket 的全双工聊天
- `routes/router.go` 内 `GET /api/chat/ws` 路由
- `cmd/wstest/main.go`：命令行 WebSocket 调试客户端（连接 `ws://host/api/chat/ws`，发 ping/chat，统计 delta 数）

**删除原因**：本次重构只保留 SSE 流式聊天接口，WebSocket 协议层被移除；对应的调试工具和依赖一并清理。

**删除文件**：

- `cmd/wstest/main.go`（含 `cmd/wstest/` 目录）
- `cmd/`、`test/`、`web/` 三个空目录

**依赖清理**：`go mod tidy` 后 `github.com/gorilla/websocket` 从 `go.mod` 移除。

---

### 6. 不再需要的 DTO / Model / Repository 方法

**`dto/chat_dto.go`**：删除。原先的 chat 协议 DTO 收敛到 `service/chat_service.go` 内（`ChatRequest`、`StreamChunk`），避免在 dto 层留下无引用的结构。

**`models/memory.go` 精简**：

| 原模型            | 删除原因                     | 处理       |
| ----------------- | ---------------------------- | ---------- |
| `UserMemory`      | 多层级长期记忆功能被删除     | 删除       |
| `ConversationSummary` | 对话摘要功能被删除       | 删除       |
| `AgentConfig`     | Agent 编排被删除             | 删除       |
| `SessionMessage`  | 短期会话消息仍需保留         | **保留**   |

**`repository/memory_repository.go` 精简**：

- 删除：长期记忆 / 摘要 / Agent 相关的所有方法
- 保留：
  - `SaveSessionMessage(msg)` —— 写入会话消息
  - `GetSessionMessages(sessionID, userID, limit)` —— 读取会话历史
  - `DeleteSessionMessages(sessionID)` —— 清理会话

---

### 7. 其它精简点

- `service/file_service.go::RegisterFile`：不再调用 `vectorRemote.IngestFile`，不再使用 DB 事务（简化为单步插入），移除 `VectorRemote` 字段
- `config/database.go::autoMigrate`：仅保留 `File / SessionMessage / User` 三张表
- `routes/router.go`：仅保留 `POST /api/chat`、`GET /api/chat/history`、`DELETE /api/chat/session` 三个 chat 路由
- `handlers/chat_handler.go`：仅保留 `GetHistoryHandle` 和 `ClearSessionHandle`

---

## 第二部分：保留 / 修改的功能（含修改前后对比）

### 1. 用户管理、用户认证和会话（**完全保留**）

| 文件                       | 修改前                   | 修改后 |
| -------------------------- | ------------------------ | ------ |
| `service/user_service.go`  | 注册 / 登录 / 注销 / 校验 | 同左   |
| `handlers/user_handler.go` | 上述 4 个 HTTP handler   | 同左   |
| `models/user.go`           | User 实体 + IsEnabled    | 同左   |
| `repository/user_repository.go` | 用户 CRUD + UpdateLastLogin / UpdatePassword / UpdateStatus | 同左 |
| `middleware/auth.go`       | RequireSession / OptionalSession / Header+Cookie+Body 提取 sid | 同左 |
| `utils/password.go`        | bcrypt + salt 加盐哈希    | 同左   |
| `utils/session.go`         | Session + MemorySessionStore（含 gcLoop）| 同左 |
| `utils/session_store.go`   | 全局 SessionStore 单例    | 同左   |
| `dto/user_dto.go`          | 注册 / 登录 / 校验 DTO    | 同左   |

**对比说明**：登录流程、密码加盐哈希策略、会话有效期（24h）、滑动过期（Touch）、三种 sid 提取方式（Header / Cookie / Body）全部不变；本次重构未触动用户域任何一行代码。

---

### 2. 文件存储（**保留主体 + 精简事务与向量化入库**）

**修改前**：

- `POST /api/file/token` —— 获取七牛云上传 token
- `POST /api/file/register` —— 注册文件信息：
  1. `tx := DB.Begin()` 开启数据库事务
  2. `tx.Create(file)` 在事务中插入 file 行
  3. `POST {ECHO_AI_REMOTE_BASE_URL}/ingest_file` 调用 Python 向量化入库
  4. 成功 → `tx.Commit()`；失败 → `tx.Rollback()`

**修改后**：

- `POST /api/file/token` —— **完全保留**，未改动
- `POST /api/file/register` —— 精简为单步插入：
  1. 校验入参
  2. `db.Create(file)` 直接插入文件记录（无事务）
  3. 不再调用 Python `ingest_file`，不再做向量化入库

**对比表**：

| 对比项              | 修改前       | 修改后     |
| ------------------- | ------------ | ---------- |
| 七牛 token 获取     | ✅ 支持      | ✅ 保留    |
| 文件元数据持久化    | ✅ 支持      | ✅ 保留    |
| Python 向量化入库   | ✅ 调用      | ❌ 删除    |
| DB 事务包裹         | ✅ 使用事务  | ❌ 删除    |
| 失败回滚            | ✅ 支持      | ❌ 删除    |
| `vectorRemote` 字段 | ✅ 注入      | ❌ 删除    |

**涉及文件**：

- `service/file_service.go` —— 修改（去除事务与 ingest 调用）
- `handlers/file_handler.go` —— 完全保留
- `models/file.go` —— 完全保留
- `repository/file_repository.go` —— 完全保留

---

### 3. 聊天接口（**精简后只保留 SSE 流式 + 历史 + 清会话**）

**修改前的接口列表**：

- `POST /api/chat` —— SSE 流式聊天
- `GET /api/chat/ws` —— WebSocket 流式聊天（删除）
- `GET /api/chat/history` —— 获取会话历史
- `GET /api/chat/summary` —— 获取对话摘要（删除）
- `GET /api/chat/memory` —— 获取用户长期记忆（删除）
- `POST /api/chat/memory` —— 保存用户长期记忆（删除）
- `GET /api/chat/memory/all` —— 列出全部记忆（删除）
- `DELETE /api/chat/memory` —— 删除用户记忆（删除）
- `GET /api/chat/agents` —— 获取 Agent 列表（删除）
- `GET /api/chat/cache/stats` —— 前缀缓存统计（删除）
- `DELETE /api/chat/session` —— 清理会话

**修改后的接口列表**：

- ✅ `POST /api/chat` —— SSE 流式聊天（**保留，逻辑重写**）
- ✅ `GET /api/chat/history` —— 获取历史（保留）
- ✅ `DELETE /api/chat/session` —— 清理会话（保留）

**主链路逻辑变化（修改前 → 修改后）**：

| 对比项              | 修改前                                            | 修改后                                  |
| ------------------- | ------------------------------------------------- | --------------------------------------- |
| Agent 编排          | ✅ MultiAgentOrchestrator + ReAct 循环            | ❌ 删除                                 |
| 工具调用            | ✅ tool_call / tool_result 透传                    | ❌ 删除                                 |
| 长期记忆注入        | ✅ BuildMemoryContext 注入 system                  | ❌ 删除                                 |
| 摘要压缩            | ✅ 增量摘要 + 滑动窗口                              | ❌ 删除                                 |
| 前缀缓存            | ✅ PromptCache 命中上游缓存                        | ❌ 删除                                 |
| 直连 LLM            | ✅ AIClient 调用 OpenAI 兼容协议                    | ❌ 删除                                 |
| 直连 Python 服务    | ❌ 不支持                                          | ✅ 新增（流式 SSE 透传）                  |
| 持久化会话消息      | ✅ 保存到 session_message                          | ✅ 保留                                 |
| SSE 协议            | ✅ text/event-stream                              | ✅ 保留（仅 start/delta/finish/error）   |
| 历史查询            | ✅ GetSessionMessages                             | ✅ 保留                                 |
| 清理会话            | ✅ DeleteSessionMessages                          | ✅ 保留                                 |

**新增文件**：

- `remote/python_client.go` —— Python 服务客户端：
  - `PythonClient.ChatStream(req, handler)`：POST `{baseURL}/v1/chat/completions`，按 SSE 逐帧回调 `token` 片段
  - `PythonClient.ChatCompletions(req)`：非流式调试用入口
  - 协议：Echo-AI 的 SSE 帧格式为 `data: {"token":"片段"}`，与 OpenAI `delta.content` 不同，已单独定义 `PythonChatStreamChunk`

**重写文件**：

- `service/chat_service.go`：
  - **修改前**：1100+ 行，包含 Agent 路由、ReAct 执行、记忆抽取编排、摘要触发、前缀缓存、工具调用结果回填、错误重试等复杂流程
  - **修改后**：精简为 ~160 行，仅包含
    1. 校验 `userId / sessionId / message`
    2. 保存用户消息到 `session_message`
    3. 拉取最近 50 条历史
    4. 构造 OpenAI 兼容 messages，调用 `pythonClient.ChatStream`
    5. 流式回调过程中累计 `fullReply`，最终落库 `assistant` 消息
  - 暴露：`ChatStream` / `GetHistory` / `ClearSession`
- `handlers/chat_stream_handler.go`：
  - **修改前**：包含 SSE + WebSocket + 工具事件透传等 ~210 行
  - **修改后**：仅保留 `ChatHandleSSE` + `writeSSEEvent` 共 ~110 行，事件序列简化为 `start → delta*N → finish / error`
- `handlers/chat_handler.go`：
  - **修改前**：包含 memory / summary / agents / cache/stats 等 ~250 行
  - **修改后**：仅保留 `GetHistoryHandle` 和 `ClearSessionHandle`
- `routes/router.go`：删除 chat 路由组下的 `ws`、`summary`、`memory*`、`agents`、`cache/stats` 路由

**对比示例（修改前 / 修改后 chat_service.go 调用栈）**：

```
修改前：
  ChatStream → MultiAgentOrchestrator.RouteAgent
           → ReActEngine.ExecuteStream
                ↳ ChatStreamWithTools (OpenAI 兼容 SSE + tool_calls 累积)
                ↳ Tool Handler 执行 → tool 消息回填
                ↳ 直至 finish_reason = "stop"
           → MemoryService.ExtractAsync (goroutine)
           → SaveSessionMessage(user + assistant)

修改后：
  ChatStream → SaveSessionMessage(user)
           → GetSessionMessages(limit=50)
           → pythonClient.ChatStream(pyReq, handler)
                ↳ 按行解析 SSE 帧 {"token":"..."}，逐 token 回调
           → SaveSessionMessage(assistant)
```

---

### 4. 数据库配置（**精简自动迁移**）

| 原迁移表            | 修改前 | 修改后     |
| ------------------- | ------ | ---------- |
| `file`              | ✅     | ✅ 保留    |
| `session_message`   | ✅     | ✅ 保留    |
| `user`              | ✅     | ✅ 保留    |
| `user_memory`       | ✅     | ❌ 删除    |
| `conversation_summary` | ✅  | ❌ 删除    |
| `agent_config`      | ✅     | ❌ 删除    |

**文件**：`config/database.go::autoMigrate` —— 仅迁移 `File / SessionMessage / User` 三张表。

---

### 5. 入口文件 `main.go`

**修改前**：标准 `gin.Default()` + `SetupRoutes(r)` + 监听 `APP_PORT`。

**修改后**：保持不变（启动早期仍调用 `utils.InitSessionStore(0)` + `defer StopSessionStore()`，保证 SessionStore 单例与优雅关停）。

---

## 删除 / 修改 / 保留 汇总

| 类别       | 数量 | 说明                                                            |
| ---------- | ---- | --------------------------------------------------------------- |
| 删除文件   | 14   | agent/ (3) + service 记忆相关 (3) + remote RAG (3) + remote/ai_client.go + dto/chat_dto.go + cmd/wstest/main.go + 空目录 3 个 |
| 修改文件   | 7    | service/chat_service.go, service/file_service.go, handlers/chat_handler.go, handlers/chat_stream_handler.go, routes/router.go, models/memory.go, repository/memory_repository.go, config/database.go |
| 新增文件   | 1    | remote/python_client.go                                          |
| 保留不变   | 13+  | 用户域 / 文件存储基础 / 工具 / 中间件 / SessionStore 全部不动    |

**删除文件清单**：

1. `agent/react_engine.go`
2. `agent/react_engine_test.go`
3. `agent/tools.go`
4. `service/memory_service.go`
5. `service/summarizer_service.go`
6. `service/prompt_cache.go`
7. `remote/vector_remote.go`
8. `remote/request/ingest_request.go`
9. `remote/response/embedding_response.go`
10. `remote/ai_client.go`
11. `dto/chat_dto.go`
12. `cmd/wstest/main.go`
13. 空目录 `cmd/`、`test/`、`web/`（清理）

**新增文件清单**：

1. `remote/python_client.go`

**修改文件清单**：

1. `models/memory.go`
2. `repository/memory_repository.go`
3. `service/chat_service.go`
4. `service/file_service.go`
5. `handlers/chat_handler.go`
6. `handlers/chat_stream_handler.go`
7. `routes/router.go`
8. `config/database.go`

**依赖变化**：`go mod tidy` 后移除 `github.com/gorilla/websocket v1.5.3`。

---

## 验证

- ✅ `go build ./...` 编译通过
- ✅ `go mod tidy` 依赖清理完成（gorilla/websocket 已移除）
- ✅ 所有保留代码遵循既有编码规范（清晰分层、`log.Printf` 流水日志、参数校验、错误处理）
- ✅ 启动服务后用户 / 文件 / 聊天三大接口自测通过（详见发布记录）

---

## 下一步

代码已提交并推送到 `echo_refactor` 分支；本次精简后服务的核心链路已收敛到「用户域 → 聊天域 → Python 服务域」三层，依赖面与维护成本显著降低。