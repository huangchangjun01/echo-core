package models

import "time"

// RoleTraits OCEAN 五维性格（role_traits）
//
// 设计要点：
//   - 复合主键 (user_id, role_id, preset_type) — 每个 (user, role) 下每个预设
//     独立保存一套 OCEAN 五维,切换预设不会互相覆盖。
//   - 五维连续值 ∈ [-2, 2],由 service 层在写入前校验。
//   - PresetType：6 种预设之一（companion_default / rational_advisor / energetic_girl /
//     gentle_senior / sarcastic_friend / psych_counselor）,作为主键第三段,允许用户自
//     定义预设 key(由 DB 唯一索引保证 key 唯一性)。
//   - 单步变更硬上限 0.05（service 层 enforce）,周总量 ≤ 0.8（audit_log 校验）。
type RoleTraits struct {
	UserID            uint      `json:"userId" gorm:"column:user_id;primaryKey;comment:所属用户ID"`
	RoleID            uint      `json:"roleId" gorm:"column:role_id;primaryKey;comment:角色ID"`
	PresetType        string    `json:"presetType" gorm:"column:preset_type;size:32;primaryKey;comment:6种预设人设之一"`
	Openness          float64   `json:"openness" gorm:"column:openness;type:decimal(5,3);comment:开放性 [-2,2]"`
	Conscientiousness float64   `json:"conscientiousness" gorm:"column:conscientiousness;type:decimal(5,3);comment:尽责性 [-2,2]"`
	Extraversion      float64   `json:"extraversion" gorm:"column:extraversion;type:decimal(5,3);comment:外向性 [-2,2]"`
	Agreeableness     float64   `json:"agreeableness" gorm:"column:agreeableness;type:decimal(5,3);comment:宜人性 [-2,2]"`
	Neuroticism       float64   `json:"neuroticism" gorm:"column:neuroticism;type:decimal(5,3);comment:神经质 [-2,2]"`
	CreatedAt         time.Time `json:"createdAt" gorm:"column:created_at;autoCreateTime;comment:创建时间"`
	UpdatedAt         time.Time `json:"updatedAt" gorm:"column:updated_at;autoUpdateTime;comment:更新时间"`
}

// TableName 指定表名
func (RoleTraits) TableName() string {
	return "role_traits"
}

// 6 种预设人设标识（写入 PresetType 字段）
const (
	PresetCompanionDefault = "companion_default"
	PresetRationalAdvisor  = "rational_advisor"
	PresetEnergeticGirl    = "energetic_girl"
	PresetGentleSenior     = "gentle_senior"
	PresetSarcasticFriend  = "sarcastic_friend"
	PresetPsychCounselor   = "psych_counselor"
)

// IsValidPreset 判断预设是否合法（service 层在切换预设时校验）
func IsValidPreset(p string) bool {
	switch p {
	case PresetCompanionDefault, PresetRationalAdvisor, PresetEnergeticGirl,
		PresetGentleSenior, PresetSarcasticFriend, PresetPsychCounselor:
		return true
	}
	return false
}
