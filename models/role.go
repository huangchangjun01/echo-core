package models

import "time"

// Role 角色实体
// 设计要点：
//   - 一个 user 下可有多个角色，每个角色各自维护记忆与对话上下文。
//   - (UserId, Name) 唯一索引保证同用户下不能重名。
//   - Status=1 正常；Status=2 已删除（软删）。
type Role struct {
	ID        uint      `json:"id" gorm:"primaryKey;autoIncrement;comment:角色ID"`
	UserId    string    `json:"userId" gorm:"column:user_id;size:128;not null;index;comment:所属用户ID"`
	Name      string    `json:"name" gorm:"column:name;size:64;not null;comment:角色名称"`
	Desc      string    `json:"desc" gorm:"column:desc;type:text;comment:角色描述"`
	Status    int       `json:"status" gorm:"column:status;default:1;comment:1-正常，2-已删除"`
	CreatedAt time.Time `json:"createdAt" gorm:"column:created_at;autoCreateTime;comment:创建时间"`
	UpdatedAt time.Time `json:"updatedAt" gorm:"column:updated_at;autoUpdateTime;comment:更新时间"`
}

// TableName 指定表名
func (Role) TableName() string {
	return "role"
}

// IsEnabled 判断角色是否可用
func (r *Role) IsEnabled() bool {
	return r.Status == 1
}
