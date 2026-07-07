package handlers

import (
	"echo-core/dto"
	"echo-core/middleware"
	"echo-core/service"
	"echo-core/utils"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ChatHandler 聊天处理器
// 同一端点 /api/chat 依据请求体中的 stream 字段走两种形态：
//   - stream=false：同步返回 ChatSyncResponse（JSON）
//   - stream=true ：SSE 流式逐帧推送 ChatEvent
//
// 对话实现完全由 Python 服务完成，本层只做协议适配（HTTP/SSE）+ 入参校验。
// 流式模式下 Go 端按 Python 服务的 6 类事件原样透传：context / tool / prefix /
// delta / done / memory_extracted（event 字段等于 ChatEvent.Type，data 字段为
// ChatEvent 的 JSON 序列化）。前端按 event 名订阅即可。
type ChatHandler struct {
	svc *service.ChatService
}

// NewChatHandler 构造 ChatHandler
func NewChatHandler(svc *service.ChatService) *ChatHandler {
	return &ChatHandler{svc: svc}
}

// Chat 统一入口
// POST /api/chat
func (h *ChatHandler) Chat(c *gin.Context) {
	utils.LogWith(c, "Chat", "收到请求 | method=POST path=%s ip=%s contentType=%s",
		c.Request.URL.Path, c.ClientIP(), c.GetHeader("Content-Type"))

	var req dto.ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.LogWith(c, "Chat", "请求参数解析失败 | err=%v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 鉴权中间件已注入 userId，强制覆盖请求体里的值（防冒用）
	if uid, ok := middleware.MustUserID(c); ok && uid != "" {
		req.UserID = uid
	}
	if req.UserID == "" || strings.TrimSpace(req.Message) == "" {
		utils.LogWith(c, "Chat", "参数缺失 | userId=%q messageLen=%d", req.UserID, len(req.Message))
		c.JSON(http.StatusBadRequest, gin.H{"error": "userId and message are required"})
		return
	}
	utils.LogWith(c, "Chat", "参数解析完成 | userId=%s sessionId=%s stream=%v msgLen=%d",
		req.UserID, req.SessionID, req.Stream, len(req.Message))

	if req.Stream {
		h.streamResponse(c, req)
		return
	}
	h.syncResponse(c, req)
}

// syncResponse 同步模式
func (h *ChatHandler) syncResponse(c *gin.Context, req dto.ChatRequest) {
	ctx := c.Request.Context()
	start := time.Now()
	utils.LogWith(c, "Chat", "进入 sync 模式 | userId=%s sessionId=%s", req.UserID, req.SessionID)
	resp, err := h.svc.ChatSync(ctx, req)
	if err != nil {
		utils.LogWith(c, "Chat", "sync 失败 | userId=%s latency=%dms err=%v", req.UserID, time.Since(start).Milliseconds(), err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// 本端额外覆盖 latencyMs（若 Python 未填则用本地计时）
	if resp.LatencyMs == 0 {
		resp.LatencyMs = time.Since(start).Milliseconds()
	}
	utils.LogWith(c, "Chat", "sync 完成 | userId=%s sessionId=%s events=%d replyLen=%d latencyMs=%d",
		req.UserID, resp.SessionID, len(resp.Events), len(resp.Reply), resp.LatencyMs)
	c.JSON(http.StatusOK, resp)
}

// streamResponse SSE 流式
// 透传 Python 6 类事件：context / tool / prefix / delta / done / memory_extracted
func (h *ChatHandler) streamResponse(c *gin.Context, req dto.ChatRequest) {
	ctx := c.Request.Context()
	c.Writer.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		utils.LogWith(c, "Chat", "响应写入器不支持 Flusher | userId=%s", req.UserID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming unsupported"})
		return
	}
	// 立刻 flush 响应头，便于前端尽早建立 EventSource 连接；同时写一条注释帧
	// 确认 SSE 通道就绪（注释以 ':' 开头，浏览器解析器忽略）。
	fmt.Fprintf(c.Writer, ": connected userId=%s\n\n", req.UserID)
	flusher.Flush()
	utils.LogWith(c, "Chat", "SSE 头已发送 | userId=%s sessionId=%s contentType=%s",
		req.UserID, req.SessionID, c.Writer.Header().Get("Content-Type"))
	utils.LogWith(c, "Chat", "开始流式输出 | userId=%s sessionId=%s msgLen=%d",
		req.UserID, req.SessionID, len(req.Message))

	// SSE 事件节流：每 N 条或每 2 秒打一条进度汇总，避免 delta 风暴刷屏
	const progressEvery = 5
	progressTicker := time.NewTicker(2 * time.Second)
	defer progressTicker.Stop()

	start := time.Now()
	eventSeq := 0
	typeCounts := make(map[string]int)
	var lastType string
	emitProgress := func(force bool) {
		utils.LogWith(c, "Chat", "SSE 进度 | userId=%s events=%d typeCounts=%v lastType=%s elapsed=%dms",
			req.UserID, eventSeq, typeCounts, lastType, time.Since(start).Milliseconds())
	}

	err := h.svc.ChatStream(ctx, req, func(ev dto.ChatEvent) error {
		eventSeq++
		typeCounts[ev.Type]++
		lastType = ev.Type
		writeErr := writeSSEEvent(c, flusher, eventSeq, ev)
		// 每 N 条事件打一次进度
		if eventSeq%progressEvery == 0 {
			emitProgress(false)
		}
		return writeErr
	})
	// ticker 不直接驱动日志（goroutine 风险），仅在错误/成功时汇总
	_ = progressTicker
	if err != nil {
		utils.LogWith(c, "Chat", "流式错误 | userId=%s err=%v typeCounts=%v", req.UserID, err, typeCounts)
		_ = writeSSEEvent(c, flusher, eventSeq+1, dto.ChatEvent{Type: "error", Error: err.Error()})
		utils.LogWith(c, "Chat", "SSE 流结束(含 error) | userId=%s events=%d elapsed=%dms",
			req.UserID, eventSeq+1, time.Since(start).Milliseconds())
		return
	}
	utils.LogWith(c, "Chat", "SSE 流结束 | userId=%s events=%d typeCounts=%v elapsed=%dms",
		req.UserID, eventSeq, typeCounts, time.Since(start).Milliseconds())
}

// writeSSEEvent 写入一个标准 SSE 事件帧（Python 协议格式）：
//
//	event: <type>     ← 透传 Python.type，前端按事件名订阅
//	id: <seq>         ← 序号，便于 last-event-id 跟踪
//	data: <json>      ← ChatEvent 完整 JSON
//	（空行）
func writeSSEEvent(c *gin.Context, flusher http.Flusher, seq int, ev dto.ChatEvent) error {
	data, err := json.Marshal(ev)
	if err != nil {
		utils.LogWith(c, "Chat", "事件序列化失败 | seq=%d type=%s err=%v", seq, ev.Type, err)
		return err
	}
	if _, err := fmt.Fprintf(c.Writer, "event: %s\nid: %d\ndata: %s\n\n", ev.Type, seq, data); err != nil {
		utils.LogWith(c, "Chat", "事件写入失败 | seq=%d type=%s err=%v", seq, ev.Type, err)
		return err
	}
	flusher.Flush()
	utils.LogWith(c, "Chat", "SSE 已发送 | seq=%d type=%s bytes=%d replySoFar=%d",
		seq, ev.Type, len(data), len(ev.Full)+len(ev.Text))
	return nil
}
