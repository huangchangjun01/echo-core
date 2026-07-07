package service

import (
	"context"
	"echo-core/dto"
	"echo-core/remote"
	"echo-core/utils"
	"errors"
	"strings"
)

// ChatService 聊天服务
// 当前实现：纯透传到 Python 服务完成对话。
// 多种记忆、摘要、Agent、RAG 等能力均由 Python 服务侧承担，本服务仅负责：
//  1. 入参校验
//  2. 调用 Python 同步 / 流式接口
//  3. 把结果（同步 JSON / 流式事件）透传给上层 handler
type ChatService struct {
	pythonClient *remote.PythonChatClient
}

// NewChatService 构造 ChatService
func NewChatService() *ChatService {
	utils.LogWithCtx(context.Background(), "ChatService", "初始化（Python 透传模式）")
	return &ChatService{
		pythonClient: remote.NewPythonChatClient(),
	}
}

// validateRequest 通用入参校验
func validateRequest(req dto.ChatRequest) error {
	if strings.TrimSpace(req.UserID) == "" {
		return errors.New("userId is required")
	}
	if strings.TrimSpace(req.Message) == "" {
		return errors.New("message is required")
	}
	return nil
}

// ChatSync 同步调用 Python 完成对话
func (s *ChatService) ChatSync(ctx context.Context, req dto.ChatRequest) (*dto.ChatSyncResponse, error) {
	utils.LogWithCtx(ctx, "ChatService", "ChatSync 入参 | userId=%s sessionId=%s stream=%v msgLen=%d",
		req.UserID, req.SessionID, req.Stream, len(req.Message))
	if err := validateRequest(req); err != nil {
		utils.LogWithCtx(ctx, "ChatService", "ChatSync 参数校验失败 | userId=%s err=%v", req.UserID, err)
		return nil, err
	}
	return s.pythonClient.ChatSync(ctx, remote.PythonChatRequest{
		UserID:    req.UserID,
		SessionID: req.SessionID,
		Message:   req.Message,
		Stream:    false,
	})
}

// ChatStream 流式调用 Python 完成对话
// handler 在每收到一条 ChatEvent 时被回调一次；流结束或出错时由 pythonClient 负责关闭。
func (s *ChatService) ChatStream(ctx context.Context, req dto.ChatRequest, handler func(dto.ChatEvent) error) error {
	utils.LogWithCtx(ctx, "ChatService", "ChatStream 入参 | userId=%s sessionId=%s msgLen=%d",
		req.UserID, req.SessionID, len(req.Message))
	if err := validateRequest(req); err != nil {
		utils.LogWithCtx(ctx, "ChatService", "ChatStream 参数校验失败 | userId=%s err=%v", req.UserID, err)
		return err
	}
	return s.pythonClient.ChatStreamEvents(ctx,
		remote.PythonChatRequest{
			UserID:    req.UserID,
			SessionID: req.SessionID,
			Message:   req.Message,
			Stream:    true,
		},
		handler,
	)
}
