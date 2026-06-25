package handlers

import (
	"echo-core/middleware"
	"echo-core/service"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ChatHandler 聊天处理器
type ChatHandler struct {
	svc *service.ChatService
}

// NewChatHandler 创建聊天处理器
func NewChatHandler(svc *service.ChatService) *ChatHandler {
	return &ChatHandler{svc: svc}
}

// GetHistoryHandle 获取历史消息
// GET /api/chat/history?session_id=xxx&user_id=xxx&limit=50
func (h *ChatHandler) GetHistoryHandle(c *gin.Context) {
	sessionID := c.Query("session_id")
	userID := c.Query("user_id")
	if sessionID == "" || userID == "" {
		log.Printf("[GetHistoryHandle] 参数缺失 | session_id: %s | user_id: %s", sessionID, userID)
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_id and user_id are required"})
		return
	}

	// 鉴权中间件已注入 userId，强制覆盖（防冒用）
	if uid, ok := middleware.MustUserID(c); ok && uid != "" {
		userID = uid
	}

	log.Printf("[GetHistoryHandle] 获取历史 | user_id: %s | session_id: %s", userID, sessionID)
	limit := 50
	history, err := h.svc.GetHistory(sessionID, userID, limit)
	if err != nil {
		log.Printf("[GetHistoryHandle] 获取历史失败 | user_id: %s | session_id: %s | error: %v", userID, sessionID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[GetHistoryHandle] 获取成功 | user_id: %s | session_id: %s | history_count: %d", userID, sessionID, len(history))
	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": history,
	})
}

// ClearSessionHandle 清理会话
// DELETE /api/chat/session?session_id=xxx&user_id=xxx
func (h *ChatHandler) ClearSessionHandle(c *gin.Context) {
	sessionID := c.Query("session_id")
	userID := c.Query("user_id")
	if sessionID == "" || userID == "" {
		log.Printf("[ClearSessionHandle] 参数缺失 | session_id: %s | user_id: %s", sessionID, userID)
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_id and user_id are required"})
		return
	}

	// 鉴权中间件已注入 userId，强制覆盖
	if uid, ok := middleware.MustUserID(c); ok && uid != "" {
		userID = uid
	}

	log.Printf("[ClearSessionHandle] 清理会话 | user_id: %s | session_id: %s", userID, sessionID)
	if err := h.svc.ClearSession(sessionID, userID); err != nil {
		log.Printf("[ClearSessionHandle] 清理失败 | user_id: %s | session_id: %s | error: %v", userID, sessionID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[ClearSessionHandle] 清理成功 | user_id: %s | session_id: %s", userID, sessionID)
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "session cleared",
	})
}