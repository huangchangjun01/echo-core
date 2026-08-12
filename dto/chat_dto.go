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

// ChatEvent 9 类事件的统一结构
// 透传给 Python 时各字段按需填充；omitempty 保证未知字段不会污染最终 JSON。
//
// 事件类型与字段映射（参考 Python 接口文档）：
//
//	"context"          -> PersonaLen, Persona, L0Count, L0Items, L1Count, L1Items
//	"resource"         -> URL, Name, DisplayName, FileID, Modality, MimeType,
//	                      ChunkIndex, TotalChunks, SizeBytes, Similarity, Source, Iter
//	"tool"             -> Name, Iter, OK, Summary
//	"prefix"           -> Text
//	"delta"            -> Text
//	"done"             -> Full
//	"memory_extracted" -> OK, Error
//	"thinking"         -> Stage, Text（新增：流式输出思考过程）
//	"memory_recall"    -> RecallCount, Hits（新增：回忆检索结果详情）
type ChatEvent struct {
	Type       string `json:"type"`
	PersonaLen *int   `json:"persona_len,omitempty"`
	L0Count    *int   `json:"l0_count,omitempty"`
	L1Count    *int   `json:"l1_count,omitempty"`
	Name       string `json:"name,omitempty"`
	Iter       *int   `json:"iter,omitempty"`
	OK         *bool  `json:"ok,omitempty"`
	Summary    string `json:"summary,omitempty"`
	Text       string `json:"text,omitempty"`
	Full       string `json:"full,omitempty"`
	Error      string `json:"error,omitempty"`
	// --- thinking 事件：流式输出思考过程 ---
	Stage string `json:"stage,omitempty"` // thinking 子阶段：start / intent / context_build / recall_search / react_decision / cascade / model_reasoning
	// --- context 事件扩展：真实注入内容 ---
	Persona string   `json:"persona,omitempty"`  // 人格原文
	L0Items []string `json:"l0_items,omitempty"` // L0 核心记忆条目（最多 20 条）
	L1Items []string `json:"l1_items,omitempty"` // L1 近期摘要条目（最多 10 条）
	// --- resource 事件：附件资源 ---
	URL         string  `json:"url,omitempty"`
	DisplayName string  `json:"display_name,omitempty"`
	FileID      string  `json:"file_id,omitempty"`
	Modality    string  `json:"modality,omitempty"`
	MimeType    string  `json:"mime_type,omitempty"`
	ChunkIndex  *int    `json:"chunk_index,omitempty"`
	TotalChunks *int    `json:"total_chunks,omitempty"`
	SizeBytes   *int64  `json:"size_bytes,omitempty"`
	Similarity  float64 `json:"similarity,omitempty"`
	Source      string  `json:"source,omitempty"`
	// --- memory_recall 事件：回忆检索命中详情 ---
	RecallCount int             `json:"count,omitempty"`
	Hits        []RecallHitItem `json:"hits,omitempty"`
}

// RecallHitItem 单条回忆命中（memory_recall 事件 hits 数组元素）。
// 前端可折叠展示每条命中的 memoryId / topic / summary / similarity。
type RecallHitItem struct {
	MemoryID   string  `json:"memory_id"`
	Topic      string  `json:"topic"`
	Summary    string  `json:"summary"`
	Similarity float64 `json:"similarity"`
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
