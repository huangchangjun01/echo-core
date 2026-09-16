package dto

// ===== Role Core DTO =====
//
// 角色内核(role_core)的所有外部交互 DTO 集中在此。
// 字段命名遵循 Go 端约定（json 标签 camelCase），service 层负责将存储的 JSON 字符串
// （traits_tags / values / taboos / example_dialogs）反序列化为结构体，方便前端直接渲染。

// ExampleDialogItem 示例对话项（user → assistant）。
type ExampleDialogItem struct {
	User      string `json:"user"`
	Assistant string `json:"assistant"`
}

// SavePersonaRequest 保存/更新人格（POST /api/role-core/persona）。
type SavePersonaRequest struct {
	RoleID         uint                `json:"roleId" binding:"required"`
	Identity       string              `json:"identity" binding:"omitempty,max=4000"`
	Background     string              `json:"background" binding:"omitempty,max=4000"`
	TraitsTags     []string            `json:"traitsTags" binding:"omitempty"`
	Values         []string            `json:"values" binding:"omitempty"`
	SpeakingStyle  string              `json:"speakingStyle" binding:"omitempty,max=4000"`
	Taboos         []string            `json:"taboos" binding:"omitempty"`
	ExampleDialogs []ExampleDialogItem `json:"exampleDialogs" binding:"omitempty"`
}

// PersonaResponse 人格快照（GET /api/role-core/persona）。
type PersonaResponse struct {
	UserID         uint                `json:"userId"`
	RoleID         uint                `json:"roleId"`
	Identity       string              `json:"identity"`
	Background     string              `json:"background"`
	TraitsTags     []string            `json:"traitsTags"`
	Values         []string            `json:"values"`
	SpeakingStyle  string              `json:"speakingStyle"`
	Taboos         []string            `json:"taboos"`
	ExampleDialogs []ExampleDialogItem `json:"exampleDialogs"`
	Version        int                 `json:"version"`
	CreatedAt      string              `json:"createdAt"`
	UpdatedAt      string              `json:"updatedAt"`
}

// SaveTraitsRequest 保存/更新 OCEAN 五维（POST /api/role-core/traits）。
//
// PresetType 改为「用户级预设 key」,校验放到 service 层查 DB。
// 不再用 hardcoded oneof,允许用户自定义预设。
type SaveTraitsRequest struct {
	RoleID            uint    `json:"roleId" binding:"required"`
	Openness          float64 `json:"openness" binding:"min=-2,max=2"`
	Conscientiousness float64 `json:"conscientiousness" binding:"min=-2,max=2"`
	Extraversion      float64 `json:"extraversion" binding:"min=-2,max=2"`
	Agreeableness     float64 `json:"agreeableness" binding:"min=-2,max=2"`
	Neuroticism       float64 `json:"neuroticism" binding:"min=-2,max=2"`
	PresetType        string  `json:"presetType" binding:"required,min=1,max=64"`
}

// OceanPresetResponse OCEAN 预设条目。
//
// 预设字典按 (user, role) 隔离 —— Response 带上 RoleID 便于前端区分
// 「同一个 key 但属于不同角色」的情况(尽管同名 key 不应该出现,但 RoleID
// 能帮助前端做 active 标记、调试面板追踪等)。
type OceanPresetResponse struct {
	ID                uint    `json:"id"`
	UserID            uint    `json:"userId"`
	RoleID            uint    `json:"roleId"`
	Key               string  `json:"key"`
	Label             string  `json:"label"`
	Description       string  `json:"description"`
	Openness          float64 `json:"openness"`
	Conscientiousness float64 `json:"conscientiousness"`
	Extraversion      float64 `json:"extraversion"`
	Agreeableness     float64 `json:"agreeableness"`
	Neuroticism       float64 `json:"neuroticism"`
	IsProtected       bool    `json:"isProtected"`
	CreatedAt         string  `json:"createdAt"`
	UpdatedAt         string  `json:"updatedAt"`
}

// CreatePresetRequest 新增预设(POST /api/role-core/presets)。
//
// RoleID 必填 —— 预设按角色隔离,新增时必须明确归属哪个角色。
// Key 在 (user, role) 内唯一(允许用户在不同角色用相同 key)。
// 5 维 ∈ [-2,2] 由 binding 校验 —— 与 SaveTraitsRequest / TraitSlider 前端
// 范围保持一致(后者 min=-2 max=2)。历史上曾限 [-1,1],但用户拖动 slider 到
// [-2,-1) 或 (1,2] 时后端拒收,导致「保存功能报错」;放宽到 [-2,2] 与 trait
// 保存端对齐,前端实际 PRD「五维连续值 ∈ [-1,1]」的软约束由 slider 拖动手感
// 自然保证(超出 ±1 的极端值很少被用户主动设置)。
// key 唯一性由 DB 唯一索引保证。
type CreatePresetRequest struct {
	RoleID            uint    `json:"roleId" binding:"required"`
	Key               string  `json:"key" binding:"required,min=1,max=64"`
	Label             string  `json:"label" binding:"required,min=1,max=64"`
	Description       string  `json:"description" binding:"omitempty,max=500"`
	Openness          float64 `json:"openness" binding:"min=-2,max=2"`
	Conscientiousness float64 `json:"conscientiousness" binding:"min=-2,max=2"`
	Extraversion      float64 `json:"extraversion" binding:"min=-2,max=2"`
	Agreeableness     float64 `json:"agreeableness" binding:"min=-2,max=2"`
	Neuroticism       float64 `json:"neuroticism" binding:"min=-2,max=2"`
}

// UpdatePresetRequest 修改预设(PUT /api/role-core/presets/:id?roleId=N)。
//
// 不允许改 key(改了会让历史 traits 关联不上)。isProtected 也由后端控制,不允许前端改。
// RoleID 必填 —— 预设按角色隔离,改预设时必须明确「改的是哪个角色的」,
// 防止越权改其它角色的 preset。
// 5 维 ∈ [-2,2] —— 与 CreatePresetRequest / SaveTraitsRequest / TraitSlider 一致。
type UpdatePresetRequest struct {
	RoleID            uint    `json:"roleId" binding:"required"`
	Label             string  `json:"label" binding:"required,min=1,max=64"`
	Description       string  `json:"description" binding:"omitempty,max=500"`
	Openness          float64 `json:"openness" binding:"min=-2,max=2"`
	Conscientiousness float64 `json:"conscientiousness" binding:"min=-2,max=2"`
	Extraversion      float64 `json:"extraversion" binding:"min=-2,max=2"`
	Agreeableness     float64 `json:"agreeableness" binding:"min=-2,max=2"`
	Neuroticism       float64 `json:"neuroticism" binding:"min=-2,max=2"`
}

// TraitsResponse OCEAN 五维响应（GET /api/role-core/traits）。
//
// MaxDelta:本次保存 5 维中绝对值最大的单步变更(取 abs 后的最大值)。
// Warning:人类可读的提示信息(如"五维有较大调整"),前端据此决定是否弹窗提醒。
// 当前 5 维超 0.05 时由后端写入,但不阻断保存 —— 单步硬上限原本是为防止
// LLM bug 一次性推飞 OCEAN;用户主动切换预设的正常业务行为不应被拦。
type TraitsResponse struct {
	UserID            uint    `json:"userId"`
	RoleID            uint    `json:"roleId"`
	Openness          float64 `json:"openness"`
	Conscientiousness float64 `json:"conscientiousness"`
	Extraversion      float64 `json:"extraversion"`
	Agreeableness     float64 `json:"agreeableness"`
	Neuroticism       float64 `json:"neuroticism"`
	PresetType        string  `json:"presetType"`
	UpdatedAt         string  `json:"updatedAt"`
	MaxDelta          float64 `json:"maxDelta,omitempty"`
	Warning           string  `json:"warning,omitempty"`
}

// CoreSnapshotResponse 完整 role core 快照（GET /api/role-core/core,调试面板用）。
type CoreSnapshotResponse struct {
	Persona      *PersonaResponse      `json:"persona,omitempty"`
	Traits       *TraitsResponse       `json:"traits,omitempty"`
	Mood         *MoodSnapshot         `json:"mood,omitempty"`
	Relationship *RelationshipSnapshot `json:"relationship,omitempty"`
	Beliefs      []BeliefItem          `json:"beliefs,omitempty"`
}

// TraitsListResponse OCEAN 五维列表响应（GET /api/role-core/traits）。
//
// 与早期直接返回 []TraitsResponse 不同,现在包成对象,带上当前生效的 preset:
//   - List: 该 (user, role) 下所有已保存预设的 OCEAN 五维(每个 preset 一行)
//   - ActivePresetType: 当前真正生效的预设 key(决定 chat 回复时读的 OCEAN)
//   - Presets: 该用户下的 OCEAN 预设字典(用户级,与 roleId 无关,但性格页一次拉完
//     就不用再单独 GET /presets 了 —— 详见 PR 重构注释)
//
// 演进历史:
//   - v1:直接返回 []TraitsResponse → 切换预设时值互窜的 bug
//   - v2:返回对象 + activePresetType → 引入「当前使用」跟踪
//   - v3(v3):加上 Presets → 性格页一次 GET 拿全 OCEAN + 预设字典 + active,
//     前端不再单独调 /presets
type TraitsListResponse struct {
	List             []TraitsResponse      `json:"list"`
	ActivePresetType string                `json:"activePresetType"`
	Presets          []OceanPresetResponse `json:"presets"`
}

// SetActivePresetRequest 切换当前使用的 OCEAN 预设（POST /api/role-core/active-preset）。
//
// 与 SaveTraitsRequest 的职责拆分:
//   - SaveTraitsRequest:写入某预设的 OCEAN 五维值(可能改得很细)
//   - SetActivePresetRequest:仅切换 chat 回复时读哪个 preset,不写 OCEAN
//
// 前端对应「保存五维性格配置」+「使用性格」两个独立按钮。
type SetActivePresetRequest struct {
	RoleID     uint   `json:"roleId" binding:"required"`
	PresetType string `json:"presetType" binding:"required,min=1,max=64"`
}

// SetActivePresetResponse 切换当前使用的 OCEAN 预设响应。
//
// 只回写新 activePresetType,前端无需再 GET /traits 来刷新。
type SetActivePresetResponse struct {
	ActivePresetType string `json:"activePresetType"`
}

// ===== Mood(M2 用,M0 先建好类型) =====

// MoodSnapshot 心情快照。
type MoodSnapshot struct {
	InstantVal        float64 `json:"instantVal"`
	InstantIntensity  float64 `json:"instantIntensity"`
	InstantEmotion    string  `json:"instantEmotion"`
	ShortVal          float64 `json:"shortVal"`
	ShortIntensity    float64 `json:"shortIntensity"`
	ShortEmotion      string  `json:"shortEmotion"`
	BaselineVal       float64 `json:"baselineVal"`
	BaselineIntensity float64 `json:"baselineIntensity"`
	BaselineEmotion   string  `json:"baselineEmotion"`
}

// UpdateMoodRequest 上报 mood 事件（POST /api/role-core/mood/event,内部接口）。
type UpdateMoodRequest struct {
	UserID         uint    `json:"userId" binding:"required"`
	RoleID         uint    `json:"roleId" binding:"required"`
	EventImpact    float64 `json:"eventImpact" binding:"min=-1,max=1"`
	EventIntensity float64 `json:"eventIntensity" binding:"min=0,max=1"`
	Emotion        string  `json:"emotion" binding:"omitempty,max=32"`
	TriggerEvent   string  `json:"triggerEvent" binding:"omitempty,max=64"`
}

// ===== Relationship(M3 用,M0 先建好类型) =====

// RelationshipSnapshot 关系值快照。
type RelationshipSnapshot struct {
	Intimacy     float64 `json:"intimacy"`
	Trust        float64 `json:"trust"`
	Satisfaction float64 `json:"satisfaction"`
	CoDays       int     `json:"coDays"`
}

// RelationshipEventRequest 上报关系事件(POST /api/role-core/relationship/event)。
//
// 内部接口:UserID 由调用方(echo-ai)在 body 里传,不依赖 session 中间件。
// EventType: self_disclosure / turn / positive_feedback / negative_feedback / co_days_7
type RelationshipEventRequest struct {
	UserID    uint   `json:"userId" binding:"required"`
	RoleID    uint   `json:"roleId" binding:"required"`
	EventType string `json:"eventType" binding:"required,oneof=self_disclosure turn positive_feedback negative_feedback co_days_7"`
	Source    string `json:"source" binding:"omitempty,max=64"`
}

// ===== Belief(M3 用,M0 先建好类型) =====

// BeliefItem 信念条目。
type BeliefItem struct {
	ID            uint    `json:"id"`
	Topic         string  `json:"topic"`
	Stance        string  `json:"stance"`
	Confidence    float64 `json:"confidence"`
	EvidenceCount int     `json:"evidenceCount"`
	Source        string  `json:"source"`
	UserConfirmed bool    `json:"userConfirmed"`
	CreatedAt     string  `json:"createdAt"`
	UpdatedAt     string  `json:"updatedAt"`
}

// CreateBeliefRequest 新增信念(POST /api/role-core/beliefs)。
type CreateBeliefRequest struct {
	RoleID     uint    `json:"roleId" binding:"required"`
	Topic      string  `json:"topic" binding:"required,min=1,max=128"`
	Stance     string  `json:"stance" binding:"required,min=1,max=4000"`
	Confidence float64 `json:"confidence" binding:"min=0,max=1"`
	Source     string  `json:"source" binding:"omitempty,oneof=user_edit memory_extract dialogue feedback"`
}

// UpdateBeliefRequest 编辑信念(PUT /api/role-core/beliefs/:id)。
//
// 与 Create 不同的是:roleId 来自路径上的 :id 对应的行,这里只带业务字段。
// source 也允许修改,用户可能想把 LLM 抽取的改回手动标注(或反之)。
type UpdateBeliefRequest struct {
	Topic      string  `json:"topic" binding:"required,min=1,max=128"`
	Stance     string  `json:"stance" binding:"required,min=1,max=4000"`
	Confidence float64 `json:"confidence" binding:"min=0,max=1"`
	Source     string  `json:"source" binding:"omitempty,oneof=user_edit memory_extract dialogue feedback"`
}

// ===== Feedback(M7 用,M0 先建好类型) =====

// SubmitFeedbackRequest 提交反馈(POST /api/role-core/feedback)。
type SubmitFeedbackRequest struct {
	RoleID    uint   `json:"roleId" binding:"required"`
	MessageID string `json:"messageId" binding:"required,max=64"`
	Rating    int    `json:"rating" binding:"required,oneof=-2 -1 0 1 2"`
	Comment   string `json:"comment" binding:"omitempty,max=1000"`
}

// ===== Suggestion(M6 用,M0 先建好类型) =====

// SuggestionItem 建议条目。
type SuggestionItem struct {
	ID             uint    `json:"id"`
	UserID         uint    `json:"userId"`
	RoleID         uint    `json:"roleId"`
	TargetType     string  `json:"targetType"`
	SuggestionJSON string  `json:"suggestionJson"`
	Status         string  `json:"status"`
	Source         string  `json:"source"`
	Confidence     float64 `json:"confidence"`
	Reason         string  `json:"reason"`
	CreatedAt      string  `json:"createdAt"`
	ResolvedAt     *string `json:"resolvedAt,omitempty"`
}

// CreateSuggestionRequest LLM 抽取后回写建议(POST /api/role-core/suggestions,内部接口)。
type CreateSuggestionRequest struct {
	UserID         uint    `json:"userId" binding:"required"`
	RoleID         uint    `json:"roleId" binding:"required"`
	TargetType     string  `json:"targetType" binding:"required,oneof=persona traits belief mood"`
	SuggestionJSON string  `json:"suggestionJson" binding:"required"`
	Source         string  `json:"source" binding:"required,oneof=memory_extract feedback user_request"`
	Confidence     float64 `json:"confidence" binding:"min=0,max=1"`
	Reason         string  `json:"reason" binding:"omitempty"`
}

// ResolveSuggestionRequest 用户处理建议(POST /api/role-core/suggestions/:id/{apply|skip|edit})。
type ResolveSuggestionRequest struct {
	EditedJSON string `json:"editedJson" binding:"omitempty"` // 仅 edit 路径使用
}
