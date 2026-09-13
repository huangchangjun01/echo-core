package models

import "time"

// RoleFeedback 用户反馈（role_feedback）
//
// 设计要点：
//   - 自增主键 id，按 (user_id, role_id, created_at) 索引。
//   - Rating 5 档：-2(强烈反对) / -1(反对) / 0(中性) / +1(赞同) / +2(强烈赞同)。
//     0 档为占位（用于"看过但未表态"）。
//   - MessageID 关联具体消息（前端用 nanoid 生成），便于按消息维度统计。
//   - Comment 可选评论文本（最大 1000 字）。
//   - 反馈不直接改 traits（PRD 铁律 #2），由 service 层按累积策略触发 baseline 调整
//     或 LLM 抽取新建议。
type RoleFeedback struct {
	ID        uint      `json:"id" gorm:"primaryKey;autoIncrement;comment:反馈ID"`
	UserID    uint      `json:"userId" gorm:"column:user_id;index:idx_feedback_user_role_time;comment:所属用户ID"`
	RoleID    uint      `json:"roleId" gorm:"column:role_id;index:idx_feedback_user_role_time;comment:所属角色ID"`
	MessageID string    `json:"messageId" gorm:"column:message_id;size:64;index;comment:关联消息ID"`
	Rating    int       `json:"rating" gorm:"column:rating;comment:评分 -2/-1/0/+1/+2"`
	Comment   string    `json:"comment" gorm:"column:comment;type:text;comment:评论"`
	CreatedAt time.Time `json:"createdAt" gorm:"column:created_at;autoCreateTime;index:idx_feedback_user_role_time;comment:创建时间"`
}

// TableName 指定表名
func (RoleFeedback) TableName() string {
	return "role_feedback"
}

// 5 档反馈评分常量
const (
	FeedbackStronglyDisagree = -2
	FeedbackDisagree         = -1
	FeedbackNeutral          = 0
	FeedbackAgree            = 1
	FeedbackStronglyAgree    = 2
)