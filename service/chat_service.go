package service

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"echo-core/models"
	"echo-core/remote"
	"echo-core/repository"
)

// ChatService 聊天服务（精简版）
type ChatService struct {
	memRepo      *repository.MemoryRepository
	pythonClient *remote.PythonClient
}

// NewChatService 创建聊天服务
func NewChatService(pythonClient *remote.PythonClient) *ChatService {
	memRepo := repository.NewMemoryRepository()

	// 自动迁移
	if err := memRepo.AutoMigrate(); err != nil {
		log.Printf("[ChatService] AutoMigrate失败 | error: %v", err)
	}

	log.Printf("[ChatService] 服务初始化完成")
	return &ChatService{
		memRepo:      memRepo,
		pythonClient: pythonClient,
	}
}

// ChatRequest 聊天请求
type ChatRequest struct {
	UserID    string `json:"userId"`
	SessionID string `json:"sessionId"`
	Message   string `json:"message"`
	Provider  string `json:"provider,omitempty"`
	Model     string `json:"model,omitempty"`
	BaseURL   string `json:"baseUrl,omitempty"`
	APIKey    string `json:"apiKey,omitempty"`
}

// StreamChunk 流式回调载荷
type StreamChunk struct {
	// Reply 当前累计的完整回复文本
	Reply string
	// Delta 本次回调新增的文本片段
	Delta string
	// Done 是否为最后一块（流结束）
	Done bool
	// Err 流过程中出现的错误（非空时 Done=true）
	Err error
}

// ChatStream 流式对话
func (s *ChatService) ChatStream(req ChatRequest, onChunk func(StreamChunk)) {
	log.Printf("[ChatService] ChatStream开始 | userId: %s | sessionId: %s | messageLen: %d", req.UserID, req.SessionID, len(req.Message))

	// 参数校验
	if req.UserID == "" || req.SessionID == "" || req.Message == "" {
		log.Printf("[ChatService] ChatStream参数不完整 | userId: %s | sessionId: %s | messageLen: %d", req.UserID, req.SessionID, len(req.Message))
		onChunk(StreamChunk{Done: true, Err: errors.New("userId, sessionId and message are required")})
		return
	}

	// 保存用户消息到数据库
	log.Printf("[ChatService] ChatStream保存用户消息 | sessionId: %s | userId: %s", req.SessionID, req.UserID)
	userMsg := &models.SessionMessage{
		SessionID: req.SessionID,
		UserID:    req.UserID,
		Role:      "user",
		Content:   req.Message,
		CreatedAt: time.Now(),
	}
	if saveErr := s.memRepo.SaveSessionMessage(userMsg); saveErr != nil {
		log.Printf("[ChatService] ChatStream保存用户消息失败: %v", saveErr)
	}

	// 获取历史消息
	log.Printf("[ChatService] ChatStream获取历史消息 | sessionId: %s | userId: %s", req.SessionID, req.UserID)
	history, err := s.memRepo.GetSessionMessages(req.SessionID, req.UserID, 50)
	if err != nil {
		log.Printf("[ChatService] ChatStream获取历史失败 | error: %v", err)
		onChunk(StreamChunk{Done: true, Err: fmt.Errorf("get history failed: %w", err)})
		return
	}
	log.Printf("[ChatService] ChatStream历史消息获取成功 | count: %d", len(history))

	// 转换历史消息为 PythonMessage 格式
	pythonHistory := make([]remote.PythonMessage, 0, len(history))
	for _, h := range history {
		pythonHistory = append(pythonHistory, remote.PythonMessage{
			Role:    h.Role,
			Content: h.Content,
		})
	}

	// 构建 Python 请求
	pyReq := remote.PythonChatRequest{
		UserID:    req.UserID,
		SessionID: req.SessionID,
		Message:   req.Message,
		History:   pythonHistory,
	}

	// 流式调用 Python 服务
	var fullReply strings.Builder

	log.Printf("[ChatService] ChatStream调用Python服务 | userId: %s | sessionId: %s", req.UserID, req.SessionID)
	err = s.pythonClient.ChatStream(pyReq, func(delta string) error {
		fullReply.WriteString(delta)
		onChunk(StreamChunk{
			Reply: fullReply.String(),
			Delta: delta,
		})
		return nil
	})

	if err != nil {
		log.Printf("[ChatService] ChatStream Python服务调用失败 | error: %v", err)
		onChunk(StreamChunk{Done: true, Err: err, Reply: fullReply.String()})
		return
	}

	// 保存助手回复到数据库
	reply := fullReply.String()
	log.Printf("[ChatService] ChatStream保存助手回复 | sessionId: %s | replyLen: %d", req.SessionID, len(reply))
	assistantMsg := &models.SessionMessage{
		SessionID: req.SessionID,
		UserID:    req.UserID,
		Role:      "assistant",
		Content:   reply,
		CreatedAt: time.Now(),
	}
	if saveErr := s.memRepo.SaveSessionMessage(assistantMsg); saveErr != nil {
		log.Printf("[ChatService] ChatStream保存助手回复失败: %v", saveErr)
	}

	log.Printf("[ChatService] ChatStream完成 | userId: %s | sessionId: %s | replyLen: %d", req.UserID, req.SessionID, len(reply))
	onChunk(StreamChunk{Done: true, Reply: reply})
}

// GetHistory 获取会话历史
func (s *ChatService) GetHistory(sessionID, userID string, limit int) ([]models.SessionMessage, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.memRepo.GetSessionMessages(sessionID, userID, limit)
}

// ClearSession 清理会话
func (s *ChatService) ClearSession(sessionID, userID string) error {
	log.Printf("[ChatService] ClearSession | sessionId: %s | userId: %s", sessionID, userID)
	return s.memRepo.DeleteSessionMessages(sessionID)
}