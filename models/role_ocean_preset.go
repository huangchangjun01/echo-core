package models

import "time"

// RoleOceanPreset OCEAN 五维性格预设字典（role_ocean_preset）
//
// 设计要点：
//   - 按 (user_id, key) 唯一：每个用户维护自己的预设库，与其他用户隔离。
//   - is_protected=true 表示「标准陪伴」等系统级预设，仅允许修改，不允许删除。
//   - 5 维 ∈ [-1, 1]，service 层在写入前拦截越界。
//   - key 用稳定字符串（不用自增 id），原因：role_traits.presetType 仍是字符串列，
//     预设的 key 即是写入时的"标识符"，重命名 label 不会破坏历史数据。
type RoleOceanPreset struct {
	ID                uint      `json:"id" gorm:"column:id;primaryKey;autoIncrement;comment:主键"`
	UserID            uint      `json:"userId" gorm:"column:user_id;not null;uniqueIndex:idx_user_key,priority:1;comment:所属用户ID"`
	Key               string    `json:"key" gorm:"column:key;size:64;not null;uniqueIndex:idx_user_key,priority:2;comment:预设key(用户内唯一)"`
	Label             string    `json:"label" gorm:"column:label;size:64;not null;comment:显示名"`
	Description       string    `json:"description" gorm:"column:description;type:text;comment:描述"`
	Openness          float64   `json:"openness" gorm:"column:openness;type:decimal(5,3);comment:开放性 [-1,1]"`
	Conscientiousness float64   `json:"conscientiousness" gorm:"column:conscientiousness;type:decimal(5,3);comment:尽责性 [-1,1]"`
	Extraversion      float64   `json:"extraversion" gorm:"column:extraversion;type:decimal(5,3);comment:外向性 [-1,1]"`
	Agreeableness     float64   `json:"agreeableness" gorm:"column:agreeableness;type:decimal(5,3);comment:宜人性 [-1,1]"`
	Neuroticism       float64   `json:"neuroticism" gorm:"column:neuroticism;type:decimal(5,3);comment:神经质 [-1,1]"`
	IsProtected       bool      `json:"isProtected" gorm:"column:is_protected;not null;default:false;comment:true=不可删除(仅标准陪伴)"`
	CreatedAt         time.Time `json:"createdAt" gorm:"column:created_at;autoCreateTime;comment:创建时间"`
	UpdatedAt         time.Time `json:"updatedAt" gorm:"column:updated_at;autoUpdateTime;comment:更新时间"`
}

// TableName 指定表名
func (RoleOceanPreset) TableName() string {
	return "role_ocean_preset"
}

// 标准预设 key 常量（与 models/role_traits.go 的 PresetCompanionDefault 等保持一致）
const (
	PresetKeyCompanionDefault = "companion_default"
	PresetKeyRationalAdvisor  = "rational_advisor"
	PresetKeyEnergeticGirl    = "energetic_girl"
	PresetKeyGentleSenior     = "gentle_senior"
	PresetKeySarcasticFriend  = "sarcastic_friend"
	PresetKeyPsychCounselor   = "psych_counselor"
)