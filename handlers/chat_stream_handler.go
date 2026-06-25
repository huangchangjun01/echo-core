package handlers

import (
	"echo-core/middleware"
	"echo-core/service"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ChatStreamHandler 流式聊天处理器
type ChatStreamHandler struct {
	svc *service.ChatService
}

// NewChatStreamHandler 创建流式聊天处理器
func NewChatStreamHandler(svc *service.ChatService) *ChatStreamHandler {
	return &ChatStreamHandler{svc: svc}
}

// ChatHandleSSE SSE流式聊天接口
// POST /api/chat
// Content-Type: text/event-stream
// 事件帧格式：
//
//	event: start        | data: {"sessionId":"..."}
//	event: delta        | data: {"delta":"片段文本","reply":"累计文本"}
//	event: finish       | data: {"reply":"完整回复","sessionId":"..."}
//	event: error        | data: {"error":"错误信息"}
func (h *ChatStreamHandler) ChatHandleSSE(c *gin.Context) {
	log.Printf("[ChatSSE] 收到SSE聊天请求 | IP: %s", c.ClientIP())

	var req service.ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[ChatSSE] 请求参数解析失败: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// 鉴权中间件已注入 userId，强制覆盖请求体里的值（防冒用）
	if uid, ok := middleware.MustUserID(c); ok && uid != "" {
		req.UserID = uid
	}
	if req.UserID == "" || req.SessionID == "" || req.Message == "" {
		log.Printf("[ChatSSE] 参数缺失 | userId: %s | sessionId: %s", req.UserID, req.SessionID)
		c.JSON(http.StatusBadRequest, gin.H{"error": "userId, sessionId and message are required"})
		return
	}

	// SSE 响应头
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		log.Printf("[ChatSSE] 响应写入器不支持Flusher")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming unsupported"})
		return
	}

	// start 帧
	writeSSEEvent(c.Writer, "start", map[string]string{"sessionId": req.SessionID})
	flusher.Flush()

	log.Printf("[ChatSSE] 开始流式输出 | userId: %s | sessionId: %s", req.UserID, req.SessionID)

	h.svc.ChatStream(req, func(chunk service.StreamChunk) {
		if chunk.Err != nil {
			log.Printf("[ChatSSE] 流式错误 | error: %v", chunk.Err)
			writeSSEEvent(c.Writer, "error", map[string]string{"error": chunk.Err.Error()})
			flusher.Flush()
			return
		}
		if !chunk.Done {
			writeSSEEvent(c.Writer, "delta", map[string]string{
				"delta": chunk.Delta,
				"reply": chunk.Reply,
			})
			flusher.Flush()
			return
		}
		// 结束帧
		writeSSEEvent(c.Writer, "finish", gin.H{
			"reply":     chunk.Reply,
			"sessionId": req.SessionID,
		})
		flusher.Flush()
	})

	log.Printf("[ChatSSE] 流式输出完成 | userId: %s | sessionId: %s", req.UserID, req.SessionID)
}

// writeSSEEvent 写入一个 SSE 事件帧
func writeSSEEvent(w http.ResponseWriter, event string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[ChatSSE] 事件序列化失败 | event: %s | error: %v", event, err)
		return
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
		log.Printf("[ChatSSE] 事件写入失败 | event: %s | error: %v", event, err)
	}
}