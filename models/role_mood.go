package models

import "time"

// RoleMood 三桶心情（role_mood）
//
// 设计要点：
//   - 复合主键 (user_id, role_id) — 同一角色同时刻只有一份心情状态。
//   - 三桶耦合公式（PRD 03_PRD_change_engine.md）：
//     instant_val = baseline*0.20 + short*0.30 + event_impact*event_intensity + personality_bias + relationship_influence + noise
//     short 衰减:   short = 0.7*short + 0.3*instant     (每 30min)
//     baseline 衰减: baseline = 0.7*baseline + 0.3*short (每 24h)
//   - 强度 intensity ∈ [0, 1]，emotion 为枚举字符串(happy/sad/angry/neutral...)。
//   - last_event_at 用于驱动衰减调度（service 层定时任务据此判断是否需要衰减）。
type RoleMood struct {
	UserID            uint      `json:"userId" gorm:"column:user_id;primaryKey;comment:所属用户ID"`
	RoleID            uint      `json:"roleId" gorm:"column:role_id;primaryKey;comment:角色ID"`
	InstantVal        float64   `json:"instantVal" gorm:"column:instant_val;type:decimal(5,3);comment:即时心情 value [-1,1]"`
	InstantIntensity  float64   `json:"instantIntensity" gorm:"column:instant_intensity;type:decimal(5,3);comment:即时强度 [0,1]"`
	InstantEmotion    string    `json:"instantEmotion" gorm:"column:instant_emotion;size:32;comment:即时情绪标签"`
	ShortVal          float64   `json:"shortVal" gorm:"column:short_val;type:decimal(5,3);comment:短期心情 value [-1,1] 半衰期30min"`
	ShortIntensity    float64   `json:"shortIntensity" gorm:"column:short_intensity;type:decimal(5,3);comment:短期强度 [0,1]"`
	ShortEmotion      string    `json:"shortEmotion" gorm:"column:short_emotion;size:32;comment:短期情绪标签"`
	BaselineVal       float64   `json:"baselineVal" gorm:"column:baseline_val;type:decimal(5,3);comment:基线心情 value [-1,1] 半衰期7d"`
	BaselineIntensity float64   `json:"baselineIntensity" gorm:"column:baseline_intensity;type:decimal(5,3);comment:基线强度 [0,1]"`
	BaselineEmotion   string    `json:"baselineEmotion" gorm:"column:baseline_emotion;size:32;comment:基线情绪标签"`
	LastEventAt       time.Time `json:"lastEventAt" gorm:"column:last_event_at;comment:最近事件时间(驱动衰减)"`
	CreatedAt         time.Time `json:"createdAt" gorm:"column:created_at;autoCreateTime;comment:创建时间"`
	UpdatedAt         time.Time `json:"updatedAt" gorm:"column:updated_at;autoUpdateTime;comment:更新时间"`
}

// TableName 指定表名
func (RoleMood) TableName() string {
	return "role_mood"
}