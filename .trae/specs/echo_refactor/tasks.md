# Tasks

## 准备工作
- [x] Task 0: 从 master 拉取 echo_refactor 分支
  - [x] git checkout master
  - [x] git pull origin master
  - [x] git checkout -b echo_refactor

## 删除不需要的功能模块
- [x] Task 1: 删除 Agent 模块（整个 agent/ 目录）
  - [x] 删除 agent/react_engine.go
  - [x] 删除 agent/react_engine_test.go
  - [x] 删除 agent/tools.go
  - [x] 删除 agent/ 目录

- [x] Task 2: 删除记忆相关服务
  - [x] 删除 service/memory_service.go
  - [x] 删除 service/summarizer_service.go
  - [x] 删除 service/prompt_cache.go

- [x] Task 3: 删除 RAG 相关远程调用模块
  - [x] 删除 remote/vector_remote.go
  - [x] 删除 remote/request/ingest_request.go
  - [x] 删除 remote/request/ 目录
  - [x] 删除 remote/response/embedding_response.go
  - [x] 删除 remote/response/ 目录

- [x] Task 4: 删除不再需要的 DTO
  - [x] 删除 dto/chat_dto.go

## 精简模型层
- [x] Task 5: 精简 models/memory.go
  - [x] 删除 UserMemory 结构体
  - [x] 删除 ConversationSummary 结构体
  - [x] 删除 AgentConfig 结构体
  - [x] 保留 SessionMessage 结构体

## 精简仓储层
- [x] Task 6: 精简 repository/memory_repository.go
  - [x] 只保留 SaveSessionMessage、GetSessionMessages、DeleteSessionMessages 方法
  - [x] 删除其他所有记忆/摘要/Agent 相关方法

## 新增 Python 服务客户端
- [x] Task 7: 创建 remote/python_client.go
  - [x] 实现 PythonClient 结构体，封装对 Python 服务 /chat 接口的流式 HTTP 调用
  - [x] 支持 SSE 流式读取，逐帧回调

## 精简聊天服务
- [x] Task 8: 重写 service/chat_service.go
  - [x] 删除所有 Agent、RAG、记忆、摘要、前缀缓存相关代码
  - [x] 保留 ChatRequest、ChatResponse、StreamChunk 基本类型
  - [x] 实现精简版 ChatStream：保存用户消息 → 获取历史 → 调用 Python 服务流式接口 → 保存助手回复
  - [x] 实现 GetHistory：获取会话历史
  - [x] 实现 ClearSession：清理会话

## 精简聊天处理器
- [x] Task 9: 精简 handlers/chat_handler.go
  - [x] 删除所有记忆/摘要/Agent/缓存统计相关 handler
  - [x] 只保留 GetHistoryHandle 和 ClearSessionHandle

- [x] Task 10: 精简 handlers/chat_stream_handler.go
  - [x] 删除 WebSocket 相关代码（ChatHandleWS、handleChatMessage、WS 相关类型）
  - [x] 删除 tool_call / tool_result 事件处理
  - [x] 精简 ChatHandleSSE：只透传 Python 服务返回的流式内容
  - [x] 删除 agent 和 remote 包依赖

## 精简路由
- [x] Task 11: 精简 routes/router.go
  - [x] 删除 chat 路由中的 ws、summary、memory、agents、cache/stats 路由
  - [x] 简化 chat 路由注册，只保留 POST /api/chat、GET /api/chat/history、DELETE /api/chat/session
  - [x] 删除 agent 和 remote 包依赖

## 精简文件服务
- [x] Task 12: 精简 service/file_service.go
  - [x] 删除 RegisterFile 中对 vectorRemote.IngestFile 的调用
  - [x] 删除 vectorRemote 字段和相关依赖
  - [x] 文件注册仅写入数据库，去掉事务回滚（简化为单步插入）

## 精简数据库配置
- [x] Task 13: 精简 config/database.go
  - [x] autoMigrate 中移除 UserMemory、ConversationSummary、AgentConfig 表
  - [x] 只保留 File、SessionMessage、User 表

## 精简入口文件
- [x] Task 14: 精简 main.go
  - [x] 删除 agent 相关 import（如果有的话）

## 编译验证
- [x] Task 15: 编译验证
  - [x] 运行 `go build ./...` 确保无编译错误
  - [x] 运行 `go mod tidy` 清理无用依赖

## 生成重构报告
- [x] Task 16: 生成重构报告 REFACTOR_REPORT.md
  - [x] 第一部分：删除的功能（列出每个删除模块的原始功能描述）
  - [x] 第二部分：保留/修改的功能（前后对比说明）

## Git 提交与推送
- [x] Task 17: 提交代码并推送到 echo_refactor 分支
  - [x] git add + git commit
  - [x] git push origin echo_refactor

## 启动服务与自测
- [x] Task 18: 启动服务并自测
  - [x] 启动服务，处理可能的启动报错
  - [x] 测试用户注册/登录/注销/会话校验接口
  - [x] 测试文件上传令牌/文件注册接口
  - [x] 测试聊天 SSE 流式接口
  - [x] 测试历史记录接口
  - [x] 测试清理会话接口
  - [x] 关闭服务

# Task Dependencies
- Task 1~4 可并行执行（独立删除文件）
- Task 5 依赖 Task 1~4（确认模型不再被引用后修改）
- Task 6 依赖 Task 5
- Task 7 可独立执行
- Task 8 依赖 Task 1~6（删除旧代码后重写）
- Task 9~10 依赖 Task 8
- Task 11 依赖 Task 9~10
- Task 12 依赖 Task 3
- Task 13 依赖 Task 5
- Task 14 依赖 Task 1~13
- Task 15 依赖 Task 14
- Task 16 依赖 Task 15
- Task 17 依赖 Task 16
- Task 18 依赖 Task 17