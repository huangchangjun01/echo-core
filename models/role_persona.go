package models

import "time"

// RolePersona 结构化人格（role_persona）
//
// 设计要点：
//   - 复合主键 (user_id, role_id)：每个用户每个角色一份（不允许同人同角色两份）。
//   - 字段全部以 JSON 字符串存储（traits_tags / values / taboos / example_dialogs），
//     让前端能整段渲染，且未来 schema 演进不必 ALTER 表。
//   - Version：乐观锁；写时校验 +1，避免并发覆盖。
//   - 与 personas 表（chat-ai 旧结构）解耦：本表是规范化、字段化的"角色内核"，便于
//     LLM 注入与审计。
type RolePersona struct {
	UserID         uint      `json:"userId" gorm:"column:user_id;primaryKey;comment:所属用户ID"`
	RoleID         uint      `json:"roleId" gorm:"column:role_id;primaryKey;comment:角色ID"`
	Identity       string    `json:"identity" gorm:"column:identity;type:text;comment:身份卡(身份/外貌/年龄)"`
	Background     string    `json:"background" gorm:"column:background;type:text;comment:背景故事"`
	TraitsTags     string    `json:"traitsTags" gorm:"column:traits_tags;type:varchar(2048);comment:性格标签(JSON数组)"`
	Values         string    `json:"values" gorm:"column:values;type:text;comment:价值观(JSON数组)"`
	SpeakingStyle  string    `json:"speakingStyle" gorm:"column:speaking_style;type:text;comment:语气偏好"`
	Taboos         string    `json:"taboos" gorm:"column:taboos;type:text;comment:禁忌(JSON数组)"`
	ExampleDialogs string    `json:"exampleDialogs" gorm:"column:example_dialogs;type:text;comment:示例对话(JSON数组)"`
	Version        int       `json:"version" gorm:"column:version;default:1;comment:乐观锁版本号"`
	CreatedAt      time.Time `json:"createdAt" gorm:"column:created_at;autoCreateTime;comment:创建时间"`
	UpdatedAt      time.Time `json:"updatedAt" gorm:"column:updated_at;autoUpdateTime;comment:更新时间"`
}

// TableName 指定表名
func (RolePersona) TableName() string {
	return "role_persona"
}