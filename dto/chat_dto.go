package dto

// ChatMessage 单条对话消息（业界 messages[] 协议，与 Python ChatMessage 对齐）。
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest 聊天请求（对齐 Python /chat）
// 设计原则：Go 服务只透传 userId / sessionId / message(messages) / roleId 四个语义字段。
// stream 控制返回形态：false 走同步 JSON 响应，true 走 SSE 流。
// message 与 messages 二选一（多轮上下文由 echo-ai 按 session 从 DB 加载）：
//   - 旧协议：message 携带当前输入（历史由前端拼串）
//   - messages[] 协议：前端只发当前输入，历史由后端接管
type ChatRequest struct {
	UserID    string        `json:"userId" binding:"required"`
	SessionID string        `json:"sessionId"`
	RoleID    string        `json:"roleId" binding:"omitempty,max=128"`
	Message   string        `json:"message"`
	Messages  []ChatMessage `json:"messages"`
	Stream    bool          `json:"stream"`
}

// ChatEvent 12 类事件的统一结构
// 透传给 Python 时各字段按需填充；omitempty 保证未知字段不会污染最终 JSON。
//
// 事件类型与字段映射（参考 Python 接口文档）：
//
//	"context"          -> PersonaLen, Persona, L0Count, L0Items, L1Count, L1Items,
//	                      L2Count, L2Items, Intent, IntentSource, IntentMs
//	"resource"         -> URL, Name, DisplayName, FileID, Modality, MimeType,
//	                      ChunkIndex, TotalChunks, SizeBytes, Similarity, Source, Iter
//	"tool"             -> Name, Iter, OK, Summary
//	"prefix"           -> Text
//	"delta"            -> Text
//	"done"             -> Full
//	"memory_extracted" -> OK, Error
//	"thinking"         -> ThinkStage, Text（流式输出思考过程）
//	"memory_recall"    -> RecallCount, Hits（回忆检索结果详情）
//	"stage"            -> Stage（M3：阶段事件，含 name/state/ts_ms）
//	"emotion_change"   -> Emotion（M3：情绪迁移，含 from/to/intensity/reason）
//	"relation_change"  -> Relation（M3：关系更新，含 intimacy_delta/trust_delta/.../source）
//
// 关于 Stage 字段名冲突：原 thinking 事件的 stage 字符串字段已重命名为 ThinkStage
// （JSON tag "think_stage"），新 stage 事件的 *StageEvent 占用 "stage" tag。
// 这是 Phase 2 streaming UX 重构的一部分（设计文档 §3.2 / §5.3）。
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
	// 原 "stage" tag 已让给 M3 新增的 Stage *StageEvent 字段；本字段改用 think_stage。
	ThinkStage string `json:"think_stage,omitempty"` // thinking 子阶段：start / intent / context_build / recall_search / react_decision / cascade / model_reasoning
	// --- context 事件扩展：真实注入内容 ---
	Persona string   `json:"persona,omitempty"`  // 人格原文
	L0Items []string `json:"l0_items,omitempty"` // L0 核心记忆条目（最多 50 条）
	L1Items []string `json:"l1_items,omitempty"` // L1 近期摘要条目（最多 100 条，仅 L1）
	L2Count *int     `json:"l2_count,omitempty"` // L2 会话摘要条数（父摘要）
	L2Items []string `json:"l2_items,omitempty"` // L2 会话摘要条目（最多 100 条）
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
	// --- mood_update 事件(M2)：三桶心情实时更新 ---
	InstantVal       float64 `json:"instantVal,omitempty"`
	InstantIntensity float64 `json:"instantIntensity,omitempty"`
	InstantEmotion   string  `json:"instantEmotion,omitempty"`
	ShortVal         float64 `json:"shortVal,omitempty"`
	BaselineVal      float64 `json:"baselineVal,omitempty"`
	Expression       string  `json:"expression,omitempty"` // smile/happy/excited/sad/cry/angry/concern/thinking
	Valence          float64 `json:"valence,omitempty"`
	// --- M3 streaming UX 新增三事件（指针 + omitempty：旧客户端不会看到空 payload） ---
	Stage    *StageEvent     `json:"stage,omitempty"`    // stage 事件：name/state/ts_ms
	Emotion  *EmotionChange  `json:"emotion,omitempty"`  // emotion_change 事件：from/to/intensity/reason
	Relation *RelationChange `json:"relation,omitempty"` // relation_change 事件：intimacy_delta/trust_delta/.../source
	// --- context 事件意图元数据（Python biz/chat.py 透传,旧字段不冲突） ---
	// Python emits these as top-level fields on the context frame:
	//   {"type":"context","intent":"chat","intent_source":"rule_greeting",
	//    "intent_ms":0.0,"persona_len":...,"l0_count":...,"l1_count":...}
	// 它们只是诊断 / 埋点信息,旧客户端即使读不到也不会出错。
	Intent       string  `json:"intent,omitempty"`        // 意图分类：chat/recall/image_search/...
	IntentSource string  `json:"intent_source,omitempty"` // 意图来源：rule_greeting/model_intent/...
	IntentMs     float64 `json:"intent_ms,omitempty"`     // 意图识别耗时(毫秒)
}

// StageEvent stage 事件载荷。标识一次流式响应的阶段进展。
// 前端可按 name 渲染进度条 / 高亮当前阶段，state 用于触发 transition。
type StageEvent struct {
	Name  string `json:"name"`            // 阶段名：answer / intent / recall_search / react_decision ...
	State string `json:"state"`           // 阶段状态：start / end
	TsMs  int64  `json:"ts_ms,omitempty"` // 阶段时间戳（毫秒），用于排序与去抖
}

// EmotionChange emotion_change 事件载荷。标识一次情绪迁移。
// From/To 为旧/新主导情绪；Intensity ∈ [0,1]；Reason 可选，用于解释来源（如 "model_reasoning"）。
type EmotionChange struct {
	From      string  `json:"from"`             // 旧情绪
	To        string  `json:"to"`               // 新情绪
	Intensity float64 `json:"intensity"`        // 新情绪强度
	Reason    string  `json:"reason,omitempty"` // 来源说明，可选
}

// RelationChange relation_change 事件载荷。标识一次关系数值变化。
// Delta 字段为有符号浮点，单位与 Python 端 EmotionEngine 一致。
type RelationChange struct {
	IntimacyDelta     float64 `json:"intimacy_delta,omitempty"`     // 亲密度变化
	TrustDelta        float64 `json:"trust_delta,omitempty"`        // 信任度变化
	SatisfactionDelta float64 `json:"satisfaction_delta,omitempty"` // 满意度变化
	Source            string  `json:"source"`                       // 触发来源：stage:answer / recall:hit ...
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
