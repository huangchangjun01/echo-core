package models

import "time"

// RoleRelationship 三维关系值（role_relationship）
//
// 设计要点：
//   - 复合主键 (user_id, role_id)：每用户每角色唯一一份关系状态。
//   - 三维 ∈ [0, 1]：
//     intimacy  亲密（自我披露、对话轮次、陪伴天数驱动）
//     trust     信任（正向反馈、建议采纳驱动；负向反馈下调）
//     satisfaction 满意度（平稳衰减 + 正负反馈比率）
//   - co_days 陪伴天数（用于触发"7 日奖励"等里程碑逻辑）。
//   - 单步累积上限：intimacy +0.05 / trust +0.02 / satisfaction 自适应。
type RoleRelationship struct {
	UserID       uint      `json:"userId" gorm:"column:user_id;primaryKey;comment:所属用户ID"`
	RoleID       uint      `json:"roleId" gorm:"column:role_id;primaryKey;comment:所属角色ID"`
	Intimacy     float64   `json:"intimacy" gorm:"column:intimacy;type:decimal(4,3);default:0;comment:亲密度 [0,1]"`
	Trust        float64   `json:"trust" gorm:"column:trust;type:decimal(4,3);default:0;comment:信任度 [0,1]"`
	Satisfaction float64   `json:"satisfaction" gorm:"column:satisfaction;type:decimal(4,3);default:0;comment:满意度 [0,1]"`
	CoDays       int       `json:"coDays" gorm:"column:co_days;default:0;comment:陪伴天数"`
	UpdatedAt    time.Time `json:"updatedAt" gorm:"column:updated_at;autoUpdateTime;comment:更新时间"`
}

// TableName 指定表名
func (RoleRelationship) TableName() string {
	return "role_relationship"
}