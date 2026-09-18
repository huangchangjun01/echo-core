package handlers

import (
	"echo-core/dto"
	"echo-core/middleware"
	"echo-core/service"
	"echo-core/utils"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// heartbeatInterval 应用层 SSE 心跳周期：每 15s 写一条注释帧,
//
// 防止反向代理 / CDN / 网关因空闲超时关闭长连接。
const heartbeatInterval = 15 * time.Second

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
	svc     *service.ChatService
	coreSvc *service.RoleCoreService
}

// NewChatHandler 构造 ChatHandler
//
// 注入 roleCoreService 用于在 chat 完成后触发「关系值」累积(turn 事件)。
// 早期 chat 完成后没有调用任何角色内核接口,导致关系值永远不动。
func NewChatHandler(svc *service.ChatService, coreSvc *service.RoleCoreService) *ChatHandler {
	return &ChatHandler{svc: svc, coreSvc: coreSvc}
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

	// chat 成功 → 触发关系值累积(turn 事件)。
	// OnChatTurn 内部失败仅记日志,不影响 chat 主响应。
	if h.coreSvc != nil {
		h.coreSvc.OnChatTurn(ctx, req.UserID, req.RoleID)
	}
	c.JSON(http.StatusOK, resp)
}

// streamResponse SSE 流式
// 透传 Python 事件：context / resource / tool / presence / delta / done /
// memory_extracted / thinking / memory_recall（旧 prefix 事件随级联下线）
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

	// SSE 事件节流：每 N 条打一条进度汇总，避免 delta 风暴刷屏
	const progressEvery = 5

	// 15s 应用层心跳：单独 goroutine 在 idle 期间持续打注释帧,避免被
	// 反向代理空闲超时切断。事件写入与心跳共用同一把互斥锁,保证对
	// c.Writer 的写串行化(否则 SSE 帧会被两个 goroutine 交错污染)。
	heartbeatTicker := time.NewTicker(heartbeatInterval)
	heartbeatStop := make(chan struct{})
	heartbeatDone := make(chan struct{})
	var writeMu sync.Mutex
	go func() {
		defer close(heartbeatDone)
		for {
			select {
			case <-heartbeatStop:
				return
			case <-heartbeatTicker.C:
				writeMu.Lock()
				hbErr := writeSSEHeartbeat(c.Writer, flusher, time.Now().UnixMilli())
				writeMu.Unlock()
				if hbErr != nil {
					utils.LogWith(c, "Chat", "心跳写入失败 | userId=%s err=%v", req.UserID, hbErr)
					return
				}
			}
		}
	}()
	// defers 在 streamResponse 返回前按 LIFO 顺序执行:先发停止信号、等待
	// 心跳 goroutine 退出,再停 ticker,保证不会有 goroutine 在响应关闭后
	// 还往 c.Writer 写数据。
	defer close(heartbeatStop)
	defer func() { <-heartbeatDone }()
	defer heartbeatTicker.Stop()

	start := time.Now()
	eventSeq := 0
	typeCounts := make(map[string]int)
	var lastType string
	// 标记是否收到过 done 事件(用于判断「这一轮 turn 是否完成」)。
	// 流结束 + 没收到 done → 不算成功 turn,不触发关系值累积。
	turnCompleted := false
	emitProgress := func(force bool) {
		utils.LogWith(c, "Chat", "SSE 进度 | userId=%s events=%d typeCounts=%v lastType=%s elapsed=%dms",
			req.UserID, eventSeq, typeCounts, lastType, time.Since(start).Milliseconds())
	}

	err := h.svc.ChatStream(ctx, req, func(ev dto.ChatEvent) error {
		eventSeq++
		typeCounts[ev.Type]++
		lastType = ev.Type
		if ev.Type == "done" {
			turnCompleted = true
		}
		writeErr := writeSSEEvent(c, flusher, eventSeq, ev, &writeMu)
		// 每 N 条事件打一次进度
		if eventSeq%progressEvery == 0 {
			emitProgress(false)
		}
		return writeErr
	})
	if err != nil {
		utils.LogWith(c, "Chat", "流式错误 | userId=%s err=%v typeCounts=%v", req.UserID, err, typeCounts)
		_ = writeSSEEvent(c, flusher, eventSeq+1, dto.ChatEvent{Type: "error", Error: err.Error()}, &writeMu)
		utils.LogWith(c, "Chat", "SSE 流结束(含 error) | userId=%s events=%d elapsed=%dms",
			req.UserID, eventSeq+1, time.Since(start).Milliseconds())
		return
	}
	utils.LogWith(c, "Chat", "SSE 流结束 | userId=%s events=%d typeCounts=%v elapsed=%dms",
		req.UserID, eventSeq, typeCounts, time.Since(start).Milliseconds())

	// chat 流成功完成 → 触发关系值累积(turn 事件)。
	// 仅在收到过 done 事件时触发(没收到 done 视为流异常中断,
	// 例如 Python 端提前断连,不算一次完成的 turn,避免重复累积)。
	// OnChatTurn 内部失败仅记日志。
	if turnCompleted && h.coreSvc != nil {
		h.coreSvc.OnChatTurn(ctx, req.UserID, req.RoleID)
	}
}

// writeSSEEvent 写入一个标准 SSE 事件帧（Python 协议格式）：
//
//	event: <type>     ← 透传 Python.type，前端按事件名订阅
//	id: <seq>         ← 序号，便于 last-event-id 跟踪
//	data: <json>      ← ChatEvent 完整 JSON
//	（空行）
//
// mu 用于与 15s 心跳 goroutine 串行化对 c.Writer 的写入,防止两个
// goroutine 写出的帧在网络层交错、破坏 SSE 帧边界。
func writeSSEEvent(c *gin.Context, flusher http.Flusher, seq int, ev dto.ChatEvent, mu *sync.Mutex) error {
	data, err := json.Marshal(ev)
	if err != nil {
		utils.LogWith(c, "Chat", "事件序列化失败 | seq=%d type=%s err=%v", seq, ev.Type, err)
		return err
	}
	mu.Lock()
	defer mu.Unlock()
	if _, err := fmt.Fprintf(c.Writer, "event: %s\nid: %d\ndata: %s\n\n", ev.Type, seq, data); err != nil {
		utils.LogWith(c, "Chat", "事件写入失败 | seq=%d type=%s err=%v", seq, ev.Type, err)
		return err
	}
	flusher.Flush()
	utils.LogWith(c, "Chat", "SSE 已发送 | seq=%d type=%s bytes=%d replySoFar=%d",
		seq, ev.Type, len(data), len(ev.Full)+len(ev.Text))
	return nil
}

// writeSSEHeartbeat 写入一条 SSE 注释心跳帧:": ping <ts_ms>\n\n"。
//
// SSE 协议约定以 ':' 开头的行为注释,会被 EventSource 解析器忽略;
// 但写一帧空字节能让 TCP/proxy/CDN/网关知道连接没有僵死,避免被
// 反向代理空闲超时切断。tsMs 用毫秒时间戳便于排查连接时间。
// 写完必须 Flush,否则字节仍留在 net/http 缓冲区,起不到 keepalive 作用。
func writeSSEHeartbeat(w io.Writer, f http.Flusher, tsMs int64) error {
	if _, err := fmt.Fprintf(w, ": ping %d\n\n", tsMs); err != nil {
		return err
	}
	f.Flush()
	return nil
}
