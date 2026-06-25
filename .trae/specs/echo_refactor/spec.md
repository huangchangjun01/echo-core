# Echo Core 服务重构 Spec

## Why
当前系统业务划分不合理，包含了多层级记忆、ReAct/Agent 编排、RAG 检索等多个复杂功能，导致系统臃肿。需要精简为核心聊天服务，只保留用户管理、文件存储和精简后的聊天接口。

## What Changes
- **删除** 多层级记忆功能（记忆抽取、合并、去重）
- **删除** React 引擎和提示词相关功能（Agent 编排、ReAct 循环、工具调用）
- **删除** Agent 相关功能（MultiAgentOrchestrator、路由决策）
- **删除** RAG 相关功能（向量检索、知识库搜索、文件入库向量化）
- **删除** 摘要生成功能（对话摘要、增量摘要、滑动窗口）
- **删除** 前缀缓存功能（PromptCache）
- **保留** 用户管理、用户认证和会话相关功能
- **保留** 文件存储相关功能（七牛云上传、文件注册，去掉 ingest_file 向量化调用）
- **精简** 聊天接口：仅保留 SSE 流式聊天 + 历史记录，直接调用 Python 服务完成对话
- **新增** Python 服务客户端（流式调用）
- **生成** 重构报告保存到根目录

## Impact
- Affected specs: 无（首次重构）
- Affected code: 见下方详细文件列表

---

## REMOVED Requirements

### Requirement: 多层级记忆功能
**Reason**: 业务不需要跨会话长期记忆、记忆抽取合并、去重等复杂功能
**Migration**: 删除 `service/memory_service.go`、`models/memory.go` 中 UserMemory 和 ConversationSummary 模型、`repository/memory_repository.go` 中记忆相关方法

### Requirement: ReAct 引擎和提示词
**Reason**: 业务简化为直接调用 Python 服务，不再需要 Agent 编排和 ReAct 循环
**Migration**: 删除 `agent/` 整个目录、`service/prompt_cache.go`、`service/summarizer_service.go`

### Requirement: Agent 编排
**Reason**: 不再需要多 Agent 路由、工具调用、search Agent 等功能
**Migration**: 删除 `agent/` 整个目录，精简 `service/chat_service.go` 中 Agent 相关代码

### Requirement: RAG 检索
**Reason**: 不再需要向量检索、知识库搜索、文件向量化入库
**Migration**: 删除 `remote/vector_remote.go`、`remote/request/`、`remote/response/`，精简 `service/file_service.go` 中 ingest_file 调用

---

## MODIFIED Requirements

### Requirement: 聊天接口
系统 SHALL 提供精简后的聊天接口，仅保留 SSE 流式聊天和会话历史记录。

#### 接口变更
- `POST /api/chat` — SSE 流式聊天（保留，但内部逻辑改为直接调用 Python 服务）
- `GET /api/chat/history` — 获取会话历史（保留）
- `DELETE /api/chat/session` — 清理会话（保留）
- 删除：`GET /api/chat/ws`、`GET /api/chat/summary`、`GET /api/chat/memory`、`POST /api/chat/memory`、`GET /api/chat/memory/all`、`DELETE /api/chat/memory`、`GET /api/chat/agents`、`GET /api/chat/cache/stats`

#### Scenario: SSE 流式聊天
- **WHEN** 用户发送 POST /api/chat 请求，携带 userId、sessionId、message
- **THEN** 系统调用 Python 服务 `/chat` 接口（流式），将 Python 返回的 SSE 流逐帧透传给客户端

#### Scenario: 获取历史记录
- **WHEN** 用户发送 GET /api/chat/history 请求
- **THEN** 系统返回该会话的历史消息列表

### Requirement: 文件存储
系统 SHALL 保留文件上传令牌和文件注册功能，但移除文件注册时对 Python ingest_file 的调用。

#### Scenario: 文件注册
- **WHEN** 用户调用 POST /api/file/register
- **THEN** 系统仅将文件信息写入数据库，不再调用 Python 向量化入库接口

---

## ADDED Requirements

### Requirement: Python 服务客户端
系统 SHALL 提供 `remote/python_client.go`，封装对 Python 服务的流式 HTTP 调用。

#### Scenario: 流式调用 Python 服务
- **WHEN** chat_service 需要调用 Python 服务
- **THEN** 通过 HTTP POST 到 `{ECHO_AI_REMOTE_BASE_URL}/chat`，使用 SSE 流式读取响应，逐帧回调