package models

import "time"

// RoleBelief 信念清单（role_belief）
//
// 设计要点：
//   - 自增主键 id（一条信念可独立增删改）。
//   - 索引 (user_id, role_id) 加快角色级查询。
//   - UserConfirmed：PRD 铁律 #1 — LLM 抽取只写 suggestions 表；本表写入前提是
//     user_confirmed=TRUE，即用户已确认或管理员手动录入。
//   - Source：来源标签(user_edit / memory_extract / dialogue / feedback)。
//   - 单次 confidence 变更硬上限 ±0.15（service 层 enforce）。
type RoleBelief struct {
	ID            uint      `json:"id" gorm:"primaryKey;autoIncrement;comment:信念ID"`
	UserID        uint      `json:"userId" gorm:"column:user_id;index:idx_belief_user_role;comment:所属用户ID"`
	RoleID        uint      `json:"roleId" gorm:"column:role_id;index:idx_belief_user_role;comment:所属角色ID"`
	Topic         string    `json:"topic" gorm:"column:topic;size:128;comment:信念主题(一句话)"`
	Stance        string    `json:"stance" gorm:"column:stance;type:text;comment:立场/态度(详细描述)"`
	Confidence    float64   `json:"confidence" gorm:"column:confidence;type:decimal(4,3);comment:置信度 [0,1]"`
	EvidenceCount int       `json:"evidenceCount" gorm:"column:evidence_count;default:0;comment:证据条数"`
	Source        string    `json:"source" gorm:"column:source;size:32;comment:来源 user_edit/memory_extract/dialogue/feedback"`
	UserConfirmed bool      `json:"userConfirmed" gorm:"column:user_confirmed;default:false;index;comment:是否经用户确认(true才生效)"`
	CreatedAt     time.Time `json:"createdAt" gorm:"column:created_at;autoCreateTime;comment:创建时间"`
	UpdatedAt     time.Time `json:"updatedAt" gorm:"column:updated_at;autoUpdateTime;comment:更新时间"`
}

// TableName 指定表名
func (RoleBelief) TableName() string {
	return "role_belief"
}

// 信念来源常量
const (
	BeliefSourceUserEdit      = "user_edit"
	BeliefSourceMemoryExtract = "memory_extract"
	BeliefSourceDialogue      = "dialogue"
	BeliefSourceFeedback      = "feedback"
)