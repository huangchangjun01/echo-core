package dto

// ChatRequest 聊天请求（对齐 Python /chat）
// 设计原则：Go 服务只透传 userId / sessionId / message / roleId 四个语义字段。
// stream 控制返回形态：false 走同步 JSON 响应，true 走 SSE 流。
type ChatRequest struct {
	UserID    string `json:"userId" binding:"required"`
	SessionID string `json:"sessionId"`
	RoleID    string `json:"roleId" binding:"omitempty,max=128"`
	Message   string `json:"message" binding:"required,min=1,max=4096"`
	Stream    bool   `json:"stream"`
}

// ChatEvent 6 类事件的统一结构
// 透传给 Python 时各字段按需填充；omitempty 保证未知字段不会污染最终 JSON。
//
// 事件类型与字段映射（参考 Python 接口文档）：
//
//	"context"          -> PersonaLen, CoreCount, L1Count
//	"tool"             -> Name, Iter, OK, Summary
//	"prefix"           -> Text
//	"delta"            -> Text
//	"done"             -> Full
//	"memory_extracted" -> OK, Error
type ChatEvent struct {
	Type       string `json:"type"`
	PersonaLen *int   `json:"persona_len,omitempty"`
	CoreCount  *int   `json:"core_count,omitempty"`
	L1Count    *int   `json:"l1_count,omitempty"`
	Name       string `json:"name,omitempty"`
	Iter       *int   `json:"iter,omitempty"`
	OK         *bool  `json:"ok,omitempty"`
	Summary    string `json:"summary,omitempty"`
	Text       string `json:"text,omitempty"`
	Full       string `json:"full,omitempty"`
	Error      string `json:"error,omitempty"`
}

// ChatSyncResponse 同步模式响应（stream=false）
// 直接复用 Python /chat 的 JSON 响应形态。
type ChatSyncResponse struct {
	UserID    string      `json:"userId"`
	SessionID string      `json:"sessionId"`
	Reply     string      `json:"reply"`
	Events    []ChatEvent `json:"events"`
	LatencyMs int64       `json:"latencyMs"`
}
