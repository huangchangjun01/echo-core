package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"time"

	"echo-core/middleware"
	"echo-core/utils"

	"github.com/gin-gonic/gin"
)

// ChatSessionHandler 会话相关代理层（单会话模型下只剩 messages + feedback）。
//
// 设计要点：
//   - 所有业务逻辑在 echo-ai 实现，这里只做 HTTP 协议适配 + 入参校验。
//   - 鉴权由 RequireSession 中间件统一注入 userId,转发时透传到 body.userId。
//   - 失败路径：echo-ai 不可达 → 502；echo-ai 返回 ok=false → 500；其余 → 200 + 透传。
//   - 单角色单会话模型下，list/create/update/delete 已下线 —— (user, role) 永远
//     对应唯一一行 chat_sessions，无「会话列表/切换/新建/删除」概念。
type ChatSessionHandler struct {
	aiBase string
	client *http.Client
}

// NewChatSessionHandler 构造
func NewChatSessionHandler() *ChatSessionHandler {
	base := os.Getenv("ECHO_AI_REMOTE_BASE_URL")
	if base == "" {
		base = "http://localhost:8000"
	}
	return &ChatSessionHandler{
		aiBase: base,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

// requireUserID 统一从 session 拿 userId,统一鉴权失败返回
func requireUserID(c *gin.Context) (string, bool) {
	uid, ok := middleware.MustUserID(c)
	if !ok || uid == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "未登录"})
		return "", false
	}
	return uid, true
}

// forward 把请求转发到 echo-ai 的 path，并返回 echo-ai 的 JSON body
func (h *ChatSessionHandler) forward(ctx context.Context, path string, body map[string]any, rid string) (map[string]any, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", h.aiBase+path, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-Id", rid)
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 500 {
		return nil, &proxyError{status: resp.StatusCode, body: string(respBody)}
	}
	// 即便 4xx 也尝试解析 body,让 echo-ai 的错误消息透传
	var out map[string]any
	if len(respBody) > 0 {
		if jerr := json.Unmarshal(respBody, &out); jerr != nil {
			return nil, &proxyError{status: resp.StatusCode, body: string(respBody)}
		}
	}
	return out, nil
}

// proxyError 区分 echo-ai 5xx 与其他错误
type proxyError struct {
	status int
	body   string
}

func (e *proxyError) Error() string { return e.body }

// ListMessages 取消息 (POST /api/chat/sessions/messages)
//
// 单会话模型下 sessionId 可省略 —— 后端会用确定性 hash 算出当前 (user, role)
// 对应的那一行 id。支持 before 游标分页 (POST 而非 GET,游标在 body 里)。
func (h *ChatSessionHandler) ListMessages(c *gin.Context) {
	uid, ok := requireUserID(c)
	if !ok {
		return
	}
	var body map[string]any
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误: " + err.Error()})
		return
	}
	body["userId"] = uid
	if _, has := body["roleId"]; !has {
		body["roleId"] = "default"
	}
	utils.LogWith(c, "ChatSession", "ListMessages | sessionId=%v before=%v",
		body["sessionId"], body["before"])
	out, err := h.forward(c.Request.Context(), "/chat/sessions/messages", body, c.GetString("RequestID"))
	if err != nil {
		utils.LogWith(c, "ChatSession", "ListMessages 转发失败 | err=%v", err)
		c.JSON(http.StatusBadGateway, gin.H{"code": 502, "message": "echo-ai unreachable", "items": []any{}, "nextBefore": nil})
		return
	}
	c.JSON(http.StatusOK, out)
}

// ListDates 列出有消息的日期 (POST /api/chat/sessions/dates)
//
// 供 Web 端 / Android 端的历史日历使用：返回倒序的
// {"ok":true,"dates":[{"date":"2026-10-01","count":42}]}。
// date 来自 echo-ai 的 DATE(created_at)，与 list_messages 的 startDate/endDate
// 同一套口径，客户端原样回传即可，不做时区换算。
func (h *ChatSessionHandler) ListDates(c *gin.Context) {
	uid, ok := requireUserID(c)
	if !ok {
		return
	}
	var body map[string]any
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误: " + err.Error()})
		return
	}
	body["userId"] = uid
	if _, has := body["roleId"]; !has {
		body["roleId"] = "default"
	}
	utils.LogWith(c, "ChatSession", "ListDates | sessionId=%v",
		body["sessionId"])
	out, err := h.forward(c.Request.Context(), "/chat/sessions/dates", body, c.GetString("RequestID"))
	if err != nil {
		utils.LogWith(c, "ChatSession", "ListDates 转发失败 | err=%v", err)
		c.JSON(http.StatusBadGateway, gin.H{"code": 502, "message": "echo-ai unreachable", "dates": []any{}})
		return
	}
	c.JSON(http.StatusOK, out)
}

// SubmitFeedback 反馈 (POST /api/chat/feedback)
func (h *ChatSessionHandler) SubmitFeedback(c *gin.Context) {
	uid, ok := requireUserID(c)
	if !ok {
		return
	}
	var body map[string]any
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误: " + err.Error()})
		return
	}
	body["userId"] = uid
	if _, has := body["roleId"]; !has {
		body["roleId"] = "default"
	}
	utils.LogWith(c, "ChatSession", "SubmitFeedback | sessionId=%v rating=%v",
		body["sessionId"], body["rating"])
	out, err := h.forward(c.Request.Context(), "/chat/feedback", body, c.GetString("RequestID"))
	if err != nil {
		utils.LogWith(c, "ChatSession", "SubmitFeedback 转发失败 | err=%v", err)
		c.JSON(http.StatusBadGateway, gin.H{"code": 502, "message": "echo-ai unreachable"})
		return
	}
	c.JSON(http.StatusOK, out)
}
