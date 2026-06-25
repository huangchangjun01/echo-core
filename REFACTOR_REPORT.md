# Echo Core 重构报告

## 概述

本次重构目的：简化业务架构，删除不必要的复杂功能，精简为只保留核心聊天服务。

重构分支：`echo_refactor`

---

## 第一部分：删除的功能

### 1. Agent 模块（整个 `agent/` 目录）

**删除前功能**：
- `react_engine.go`：实现了 ReAct 引擎，支持多轮工具调用循环，处理 Agent 编排
- `tools.go`：定义了工具集，包括默认工具（天气、计算、时间）和 RAG 搜索工具
- `react_engine_test.go`：单元测试代码
- 支持 MultiAgent 编排：根据用户问题路由到不同 Agent（default / search）
- 支持 RAG 知识库检索、工具调用、ReAct 流式执行

**删除原因**：业务简化为直接调用 Python 服务，不再需要 Agent 编排和工具调用逻辑

**删除文件**：
- `agent/react_engine.go`
- `agent/react_engine_test.go`
- `agent/tools.go`
- `agent/` 目录

---

### 2. 多层级记忆服务

**删除前功能**：
- `memory_service.go`：用户长期记忆服务，负责记忆加载、上下文构建、异步抽取、合并去重
  - 支持从对话中抽取用户偏好、事实、知识
  - 支持语义去重合并，避免脏数据膨胀
  - 跨会话注入到 system prompt
- `summarizer_service.go`：对话摘要服务，增量摘要 + 滑动窗口
  - 增量摘要：旧摘要 + 新增消息 → 新摘要
  - 滑动窗口：只保留最近 N 条消息 + 摘要
  - 控制 token 增长，避免上下文爆炸
- `prompt_cache.go`：提示词前缀缓存
  - 缓存拼装好的前缀字符串，减少重复计算
  - 支持 LLM 上游前缀缓存命中，节省 token 费用

**删除原因**：业务不需要跨会话长期记忆、摘要压缩、前缀缓存等复杂功能

**删除文件**：
- `service/memory_service.go`
- `service/summarizer_service.go`
- `service/prompt_cache.go`

---

### 3. RAG 相关远程调用

**删除前功能**：
- `vector_remote.go`：调用 Python 服务获取向量嵌入（文本/图片）、文件向量化入库
- `remote/request/ingest_request.go`：定义文件入库请求结构
- `remote/response/embedding_response.go`：定义向量响应结构

**删除原因**：不再需要 RAG 检索、向量化入库功能

**删除文件**：
- `remote/vector_remote.go`
- `remote/request/` 目录
- `remote/response/` 目录

---

### 4. 其他删除

**dto 层**：
- `dto/chat_dto.go`：删除，因为请求响应结构统一由 service 层定义

**models 层清理**：
- 删除 `UserMemory`：用户长期记忆模型
- 删除 `ConversationSummary`：对话摘要模型
- 删除 `AgentConfig`：Agent 配置模型
- 仅保留 `SessionMessage`：会话消息模型

**repository 层清理**：
- 删除 `memory_repository.go` 中所有记忆、摘要相关方法
- 仅保留 `SaveSessionMessage`、`GetSessionMessages`、`DeleteSessionMessages` 三个方法

---

## 第二部分：保留/修改的功能

### 1. 用户管理、用户认证和会话

**修改前**：
- 用户注册、登录、注销、会话校验
- 基于 bcrypt 加盐密码哈希
- 基于内存会话存储，5 分钟自动清理过期会话
- 支持 Header/Cookie/Body 三种方式提取 sessionId

**修改后**：
- 完全保留上述所有功能
- 代码无任何修改

**文件**：
- `service/user_service.go` —— 完全保留
- `handlers/user_handler.go` —— 完全保留
- `models/user.go` —— 完全保留
- `repository/user_repository.go` —— 完全保留
- `middleware/auth.go` —— 完全保留
- `utils/password.go` —— 完全保留
- `utils/session.go` —— 完全保留
- `utils/session_store.go` —— 完全保留

---

### 2. 文件存储

**修改前**：
- 提供 `POST /api/file/token` 获取七牛云上传 token
- 提供 `POST /api/file/register` 注册文件信息
  - 开启数据库事务
  - 插入文件记录到数据库
  - 调用 Python `ingest_file` 接口做向量化入库
  - 失败则回滚事务

**修改后**：
- 保留获取上传 token 功能，完全不变
- 注册文件功能简化：
  - 移除事务（单步插入不需要事务）
  - 移除对 Python `ingest_file` 接口的调用
  - 移除事务回滚逻辑
  - 仅直接插入文件记录到数据库
- 保留 `models/file.go` 完整结构不变
- 保留 `repository/file_repository.go` 完整功能不变

**修改对比**：

| 对比项 | 修改前 | 修改后 |
|--------|--------|--------|
| 向量化入库 | ✅ 支持 | ❌ 删除 |
| 事务 | ✅ 支持 | ❌ 删除 |
| 文件元数据持久化 | ✅ 支持 | ✅ 保留 |
| 七牛 token 获取 | ✅ 支持 | ✅ 保留 |

**文件**：
- `service/file_service.go` —— 修改
- `handlers/file_handler.go` —— 完全保留
- `models/file.go` —— 完全保留
- `repository/file_repository.go` —— 完全保留

---

### 3. 聊天接口（精简后）

**修改前**：
- `POST /api/chat`：SSE 流式聊天
- `GET /api/chat/ws`：WebSocket 流式聊天
- `GET /api/chat/history`：获取历史
- `GET /api/chat/summary`：获取摘要
- `GET /api/chat/memory`：获取用户记忆
- `POST /api/chat/memory`：保存用户记忆
- `GET /api/chat/memory/all`：列出全部记忆
- `DELETE /api/chat/memory`：删除用户记忆
- `GET /api/chat/agents`：获取 Agent 列表
- `DELETE /api/chat/session`：清理会话
- `GET /api/chat/cache/stats`：缓存统计

**修改后**：
- ✅ `POST /api/chat`：保留 SSE 流式聊天，内部逻辑重写
- ✅ `GET /api/chat/history`：保留获取历史
- ✅ `DELETE /api/chat/session`：保留清理会话
- ❌ 删除 `GET /api/chat/ws`：WebSocket 聊天
- ❌ 删除 `GET /api/chat/summary`：获取摘要
- ❌ 删除 `GET /api/chat/memory`：获取用户记忆
- ❌ 删除 `POST /api/chat/memory`：保存用户记忆
- ❌ 删除 `GET /api/chat/memory/all`：列出全部记忆
- ❌ 删除 `DELETE /api/chat/memory`：删除用户记忆
- ❌ 删除 `GET /api/chat/agents`：获取 Agent 列表
- ❌ 删除 `GET /api/chat/cache/stats`：缓存统计

**内部逻辑变化**：

| 对比项 | 修改前 | 修改后 |
|--------|--------|--------|
| Agent 编排 | ✅ ReAct + 工具调用 | ❌ 删除 |
| 长期记忆注入 | ✅ 支持 | ❌ 删除 |
| 摘要压缩 | ✅ 增量摘要 + 滑动窗口 | ❌ 删除 |
| Prompt 缓存 | ✅ 支持 | ❌ 删除 |
| 直接调用 Python | ❌ 不支持 | ✅ 支持 |
| SSE 流式透传 | ✅ 支持 | ✅ 保留（简化） |
| 历史保存 | ✅ 保存到 DB | ✅ 保留 |
| 工具调用事件 | ✅ tool_call/tool_result 透传 | ❌ 删除 |

**新增文件**：
- `remote/python_client.go`：新增 Python 服务客户端，支持流式 SSE 调用

**重写文件**：
- `service/chat_service.go`：完全重写，精简为直接调用 Python
- `handlers/chat_handler.go`：删除无关 handler，仅保留 2 个接口
- `handlers/chat_stream_handler.go`：删除 WebSocket 和工具调用，仅保留 SSE
- `routes/router.go`：删除无用路由，精简 chat 路由

**路由变化**：
```
/api/chat
  POST / → SSE 流式聊天（保留）
  GET /history → 获取历史（保留）
  DELETE /session → 清理会话（保留）
  （其它路由全部删除）
```

---

### 4. 数据库配置

**修改前**：
自动迁移 6 张表：
- `file`
- `session_message`
- `user_memory`
- `conversation_summary`
- `agent_config`
- `user`

**修改后**：
自动迁移 3 张表：
- `file` ✅ 保留
- `session_message` ✅ 保留
- `user` ✅ 保留
- `user_memory` ❌ 删除
- `conversation_summary` ❌ 删除
- `agent_config` ❌ 删除

**文件**：`config/database.go` —— 修改

---

### 5. 入口文件

**修改前**：`main.go` 无需修改，因为原本就没有导入已删除的包

**修改后**：保持不变

---

## 统计

| 统计项 | 数量 |
|--------|------|
| 删除文件 | 10 + 2 个目录 = 12 个 |
| 修改文件 | 7 个 |
| 新增文件 | 1 个 |
| 保留不变文件 | 10 个 |

**删除文件列表**：
1. `agent/react_engine.go`
2. `agent/react_engine_test.go`
3. `agent/tools.go`
4. `service/memory_service.go`
5. `service/summarizer_service.go`
6. `service/prompt_cache.go`
7. `remote/vector_remote.go`
8. `remote/request/ingest_request.go`
9. `remote/response/embedding_response.go`
10. `dto/chat_dto.go`

**新增文件**：
1. `remote/python_client.go`

**修改文件**：
1. `models/memory.go`
2. `repository/memory_repository.go`
3. `service/chat_service.go`
4. `service/file_service.go`
5. `handlers/chat_handler.go`
6. `handlers/chat_stream_handler.go`
7. `routes/router.go`
8. `config/database.go`

---

## 验证

- ✅ `go build ./...` 编译通过
- ✅ `go mod tidy` 依赖清理完成
- ✅ 代码符合项目既有编码规范

---

## 下一步

代码已提交并推送到 `echo_refactor` 分支，等待测试。
