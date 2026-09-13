package models

import "time"

// RoleCoreAuditLog 角色内核审计日志（role_core_audit_log）
//
// 设计要点：
//   - 自增主键 id，按 (user_id, role_id, created_at) 索引。
//   - Action: create / update / delete / restore / feedback_apply / suggestion_apply。
//   - TargetType: persona / traits / mood / belief / relationship / feedback / suggestion。
//   - BeforeJSON / AfterJSON 完整快照（JSON 字符串），便于一键回滚。
//   - DeltaSummary: 人类可读的差异摘要（如 "O:+0.05, A:-0.02"）。
//   - TriggerEvent: 触发事件（user_edit / memory_extract / feedback / mood_update）。
//
// 隐私：仅本人可见（service 层强制带 user_id 过滤）；管理员后台不展示。
type RoleCoreAuditLog struct {
	ID           uint      `json:"id" gorm:"primaryKey;autoIncrement;comment:审计ID"`
	UserID       uint      `json:"userId" gorm:"column:user_id;index:idx_audit_user_role_time;comment:所属用户ID"`
	RoleID       uint      `json:"roleId" gorm:"column:role_id;index:idx_audit_user_role_time;comment:所属角色ID"`
	Action       string    `json:"action" gorm:"column:action;size:32;index;comment:操作类型"`
	TargetType   string    `json:"targetType" gorm:"column:target_type;size:32;index;comment:资源类型"`
	BeforeJSON   string    `json:"beforeJson" gorm:"column:before_json;type:text;comment:变更前快照"`
	AfterJSON    string    `json:"afterJson" gorm:"column:after_json;type:text;comment:变更后快照"`
	DeltaSummary string    `json:"deltaSummary" gorm:"column:delta_summary;type:text;comment:差异摘要"`
	TriggerEvent string    `json:"triggerEvent" gorm:"column:trigger_event;size:64;comment:触发事件"`
	CreatedAt    time.Time `json:"createdAt" gorm:"column:created_at;autoCreateTime;index:idx_audit_user_role_time;comment:创建时间"`
}

// TableName 指定表名
func (RoleCoreAuditLog) TableName() string {
	return "role_core_audit_log"
}

// 审计操作类型常量
const (
	AuditActionCreate          = "create"
	AuditActionUpdate          = "update"
	AuditActionDelete          = "delete"
	AuditActionRestore         = "restore"
	AuditActionFeedbackApply   = "feedback_apply"
	AuditActionSuggestionApply = "suggestion_apply"
)

// 审计目标类型常量
const (
	AuditTargetPersona      = "persona"
	AuditTargetTraits       = "traits"
	AuditTargetMood         = "mood"
	AuditTargetBelief       = "belief"
	AuditTargetRelationship = "relationship"
	AuditTargetFeedback     = "feedback"
	AuditTargetSuggestion   = "suggestion"
)

// 触发事件常量
const (
	AuditTriggerUserEdit      = "user_edit"
	AuditTriggerMemoryExtract = "memory_extract"
	AuditTriggerFeedback      = "feedback"
	AuditTriggerMoodUpdate    = "mood_update"
)