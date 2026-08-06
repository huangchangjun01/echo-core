package models

import "time"

// 解析状态常量（recall_memory.parse_status）
const (
	ParseStatusPending = 0 // 待解析
	ParseStatusRunning = 1 // 解析中
	ParseStatusDone    = 2 // 解析完成
	ParseStatusFailed  = 3 // 解析失败
)

// 编辑锁状态常量（recall_memory.edit_status）
const (
	EditStatusIdle    = 0 // 空闲，可手动编辑
	EditStatusAIWrite = 1 // AI 正在写入 md，禁止手动编辑
)

// 记录状态常量（status）
const (
	RecallStatusActive  = 1 // 可用
	RecallStatusDeleted = 2 // 已删除（软删）
)

// RecallMemory 回忆记忆主题（文档即记忆）。
//
// 每个主题对应对象存储中一个目录 /memory/{userId}/{roleId}/{memoryId}/，
// 目录下存放全部源文件 + 由 AI 解析生成的 {memoryId}.md。
// 向量库（EchoRecall）只索引 md 的摘要，命中后按需逐层拉取 md 细节（渐进式回忆）。
//
// 唯一约束：同一 (userId, roleId, topic) 在可用状态下唯一（uk_recall_topic）。
type RecallMemory struct {
	Id             uint      `json:"id" gorm:"primaryKey"`
	MemoryId       string    `json:"memoryId" gorm:"column:memory_id;size:32;uniqueIndex;comment:记忆ID(UUID去横线)"`
	UserId         string    `json:"userId" gorm:"column:user_id;size:255;index:idx_recall_user_role;comment:用户ID"`
	RoleId         string    `json:"roleId" gorm:"column:role_id;size:128;index:idx_recall_user_role;comment:角色ID"`
	Topic          string    `json:"topic" gorm:"column:topic;size:255;comment:记忆主题(时间+地点+人物+事件，一经填写不可改)"`
	SubjectiveDesc string    `json:"subjectiveDesc" gorm:"column:subjective_desc;type:text;comment:记忆主观描述(<=1000)"`
	MdKey          string    `json:"mdKey" gorm:"column:md_key;size:512;comment:{memoryId}.md 的对象存储key，初始为空"`
	MdContent      string    `json:"mdContent,omitempty" gorm:"column:md_content;type:longtext;comment:{memoryId}.md 内容缓存(避免从对象存储下载)"`
	ParseStatus    int       `json:"parseStatus" gorm:"column:parse_status;default:0;comment:0待解析 1解析中 2完成 3失败"`
	EditStatus     int       `json:"editStatus" gorm:"column:edit_status;default:0;comment:0空闲 1AI写入中(编辑锁)"`
	Status         int       `json:"status" gorm:"column:status;default:1;comment:1可用 2已删除"`
	CreatedAt      time.Time `json:"createdAt" gorm:"column:created_at;autoCreateTime;comment:创建时间"`
	UpdatedAt      time.Time `json:"updatedAt" gorm:"column:updated_at;autoUpdateTime;comment:更新时间"`
}

func (RecallMemory) TableName() string {
	return "recall_memory"
}

// RecallSourceFile 回忆记忆的源文件（可多个，多模态）。
type RecallSourceFile struct {
	Id        uint      `json:"id" gorm:"primaryKey"`
	MemoryId  string    `json:"memoryId" gorm:"column:memory_id;size:32;index;comment:所属记忆ID"`
	FileKey   string    `json:"fileKey" gorm:"column:file_key;size:512;comment:对象存储key"`
	FileName  string    `json:"fileName" gorm:"column:file_name;size:255;comment:文件名"`
	FileType  int       `json:"fileType" gorm:"column:file_type;comment:1文本 2图片 3视频 4音频"`
	Status    int       `json:"status" gorm:"column:status;default:1;comment:1可用 2已删除"`
	CreatedAt time.Time `json:"createdAt" gorm:"column:created_at;autoCreateTime;comment:创建时间"`
}

func (RecallSourceFile) TableName() string {
	return "recall_source_file"
}
