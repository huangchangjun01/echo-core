package models

import "time"

// RoleEvolutionSuggestion 角色内核演化建议（role_evolution_suggestion）
//
// 设计要点：
//   - 自增主键 id，按 (user_id, role_id, status, created_at) 索引。
//   - TargetType: persona / traits / belief / mood — 待更新的目标资源类型。
//   - SuggestionJSON: 完整建议内容（JSON 字符串），由 LLM 抽取产出。
//   - Status: pending（待用户确认） / applied（已应用） / skipped（已跳过） /
//     edited（编辑后应用）。
//   - Source: memory_extract / feedback / user_request。
//   - ResolvedAt: 用户处理时间（pending 时为 NULL）。
//   - PRD 铁律 #1：本表是 LLM 抽取的唯一落地表，主表（persona/traits/belief）的所有
//     自动更新必须经用户 Apply/Skip/Edit 后由 service 层从本表搬移。
type RoleEvolutionSuggestion struct {
	ID             uint       `json:"id" gorm:"primaryKey;autoIncrement;comment:建议ID"`
	UserID         uint       `json:"userId" gorm:"column:user_id;index:idx_sug_user_role_status_time;comment:所属用户ID"`
	RoleID         uint       `json:"roleId" gorm:"column:role_id;index:idx_sug_user_role_status_time;comment:所属角色ID"`
	TargetType     string     `json:"targetType" gorm:"column:target_type;size:32;comment:目标类型 persona/traits/belief/mood"`
	SuggestionJSON string     `json:"suggestionJson" gorm:"column:suggestion_json;type:text;comment:完整建议内容(JSON)"`
	Status         string     `json:"status" gorm:"column:status;size:16;index:idx_sug_user_role_status_time;comment:状态 pending/applied/skipped/edited"`
	Source         string     `json:"source" gorm:"column:source;size:32;comment:来源 memory_extract/feedback/user_request"`
	Confidence     float64    `json:"confidence" gorm:"column:confidence;type:decimal(4,3);default:0;comment:LLM 置信度 [0,1]"`
	Reason         string     `json:"reason" gorm:"column:reason;type:text;comment:建议理由"`
	CreatedAt      time.Time  `json:"createdAt" gorm:"column:created_at;autoCreateTime;index:idx_sug_user_role_status_time;comment:创建时间"`
	ResolvedAt     *time.Time `json:"resolvedAt" gorm:"column:resolved_at;comment:处理时间(pending时为NULL)"`
}

// TableName 指定表名
func (RoleEvolutionSuggestion) TableName() string {
	return "role_evolution_suggestion"
}

// 建议状态常量
const (
	SuggestionStatusPending = "pending"
	SuggestionStatusApplied = "applied"
	SuggestionStatusSkipped = "skipped"
	SuggestionStatusEdited  = "edited"
)

// 建议来源常量
const (
	SuggestionSourceMemoryExtract = "memory_extract"
	SuggestionSourceFeedback      = "feedback"
	SuggestionSourceUserRequest   = "user_request"
)