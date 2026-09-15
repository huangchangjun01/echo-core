package models

import "time"

// RoleActivePreset 当前使用的 OCEAN 预设(role_active_preset)
//
// 设计要点：
//   - 复合主键 (user_id, role_id) — 同一角色同一时刻只有唯一一个 active preset。
//   - preset_type 字段记录当前生效的预设 key(6 种系统预设或用户自定义)。
//   - SaveTraits 时由 service 层自动把被保存的 preset 设为 active;
//     用户在 UI 上点击"设置当前性格"等同于切换 active preset。
//   - 不设外键约束到 role_traits:preset_type(preset 可能被删,但 OCEAN 行可能
//     还有保留);service 层用 application 校验保证一致性。
type RoleActivePreset struct {
	UserID     uint      `json:"userId" gorm:"column:user_id;primaryKey;comment:所属用户ID"`
	RoleID     uint      `json:"roleId" gorm:"column:role_id;primaryKey;comment:角色ID"`
	PresetType string    `json:"presetType" gorm:"column:preset_type;size:32;comment:当前使用的OCEAN预设key"`
	UpdatedAt  time.Time `json:"updatedAt" gorm:"column:updated_at;autoUpdateTime;comment:更新时间"`
}

// TableName 指定表名
func (RoleActivePreset) TableName() string {
	return "role_active_preset"
}
