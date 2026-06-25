# Tasks

## 准备工作
- [ ] Task 0: 从 master 拉取 echo_refactor 分支
  - [ ] git checkout master
  - [ ] git pull origin master
  - [ ] git checkout -b echo_refactor

## 删除不需要的功能模块
- [ ] Task 1: 删除 Agent 模块（整个 agent/ 目录）
  - [ ] 删除 agent/react_engine.go
  - [ ] 删除 agent/react_engine_test.go
  - [ ] 删除 agent/tools.go
  - [ ] 删除 agent/ 目录

- [ ] Task 2: 删除记忆相关服务
  - [ ] 删除 service/memory_service.go
  - [ ] 删除 service/summarizer_service.go
  - [ ] 删除 service/prompt_cache.go

- [ ] Task 3: 删除 RAG 相关远程调用模块
  - [ ] 删除 remote/vector_remote.go
  - [ ] 删除 remote/request/ingest_request.go
  - [ ] 删除 remote/request/ 目录
  - [ ] 删除 remote/response/embedding_response.go
  - [ ] 删除 remote/response/ 目录

- [ ] Task 4: 删除不再需要的 DTO
  - [ ] 删除 dto/chat_dto.go

## 精简模型层
- [ ] Task 5: 精简 models/memory.go
  - [ ] 删除 UserMemory 结构体
  - [ ] 删除 ConversationSummary 结构体
  - [ ] 删除 AgentConfig 结构体
  - [ ] 保留 SessionMessage 结构体

## 精简仓储层
- [ ] Task 6: 精简 repository/memory_repository.go
  - [ ] 只保留 SaveSessionMessage、GetSessionMessages、DeleteSessionMessages 方法
  - [ ] 删除其他所有记忆/摘要/Agent 相关方法

## 新增 Python 服务客户端
- [ ] Task 7: 创建 remote/python_client.go
  - [ ] 实现 PythonClient 结构体，封装对 Python 服务 /chat 接口的流式 HTTP 调用
  - [ ] 支持 SSE 流式读取，逐帧回调

## 精简聊天服务
- [ ] Task 8: 重写 service/chat_service.go
  - [ ] 删除所有 Agent、RAG、记忆、摘要、前缀缓存相关代码
  - [ ] 保留 ChatRequest、ChatResponse、StreamChunk 基本类型
  - [ ] 实现精简版 ChatStream：保存用户消息 → 获取历史 → 调用 Python 服务流式接口 → 保存助手回复
  - [ ] 实现 GetHistory：获取会话历史
  - [ ] 实现 ClearSession：清理会话

## 精简聊天处理器
- [ ] Task 9: 精简 handlers/chat_handler.go
  - [ ] 删除所有记忆/摘要/Agent/缓存统计相关 handler
  - [ ] 只保留 GetHistoryHandle 和 ClearSessionHandle

- [ ] Task 10: 精简 handlers/chat_stream_handler.go
  - [ ] 删除 WebSocket 相关代码（ChatHandleWS、handleChatMessage、WS 相关类型）
  - [ ] 删除 tool_call / tool_result 事件处理
  - [ ] 精简 ChatHandleSSE：只透传 Python 服务返回的流式内容
  - [ ] 删除 agent 和 remote 包依赖

## 精简路由
- [ ] Task 11: 精简 routes/router.go
  - [ ] 删除 chat 路由中的 ws、summary、memory、agents、cache/stats 路由
  - [ ] 简化 chat 路由注册，只保留 POST /api/chat、GET /api/chat/history、DELETE /api/chat/session
  - [ ] 删除 agent 和 remote 包依赖

## 精简文件服务
- [ ] Task 12: 精简 service/file_service.go
  - [ ] 删除 RegisterFile 中对 vectorRemote.IngestFile 的调用
  - [ ] 删除 vectorRemote 字段和相关依赖
  - [ ] 文件注册仅写入数据库，去掉事务回滚（简化为单步插入）

## 精简数据库配置
- [ ] Task 13: 精简 config/database.go
  - [ ] autoMigrate 中移除 UserMemory、ConversationSummary、AgentConfig 表
  - [ ] 只保留 File、SessionMessage、User 表

## 精简入口文件
- [ ] Task 14: 精简 main.go
  - [ ] 删除 agent 相关 import（如果有的话）

## 编译验证
- [ ] Task 15: 编译验证
  - [ ] 运行 `go build ./...` 确保无编译错误
  - [ ] 运行 `go mod tidy` 清理无用依赖

## 生成重构报告
- [ ] Task 16: 生成重构报告 REFACTOR_REPORT.md
  - [ ] 第一部分：删除的功能（列出每个删除模块的原始功能描述）
  - [ ] 第二部分：保留/修改的功能（前后对比说明）

## Git 提交与推送
- [ ] Task 17: 提交代码并推送到 echo_refactor 分支
  - [ ] git add + git commit
  - [ ] git push origin echo_refactor

## 启动服务与自测
- [ ] Task 18: 启动服务并自测
  - [ ] 启动服务，处理可能的启动报错
  - [ ] 测试用户注册/登录/注销/会话校验接口
  - [ ] 测试文件上传令牌/文件注册接口
  - [ ] 测试聊天 SSE 流式接口
  - [ ] 测试历史记录接口
  - [ ] 测试清理会话接口
  - [ ] 关闭服务

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