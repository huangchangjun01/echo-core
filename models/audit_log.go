package models

import "time"

// AuditLog 审计日志：文件下载授权、敏感操作留痕。
//
// 设计目标：
//   - 仅记录"已授权"事件（不管控实际下载是否完成——直连七牛没有完成回调）
//   - 不存签名字符串或 secret，避免日志泄露后被利用
//   - 按 user_id / action / created_at 建索引，便于按人或按时间倒序排
//
// 字段映射：
//   - Action:   download_authorize / 其他敏感操作
//   - Target:   file / memory_source / memory_md
//   - Status:   ok / not_found / forbidden / invalid_request / internal_error / rate_limited
type AuditLog struct {
	Id         uint      `json:"id" gorm:"primaryKey"`
	UserId     string    `json:"userId" gorm:"column:user_id;size:64;index;comment:操作用户 ID"`
	Action     string    `json:"action" gorm:"column:action;size:64;index;comment:操作类型"`
	TargetType string    `json:"targetType" gorm:"column:target_type;size:32;index;comment:资源类型"`
	TargetId   string    `json:"targetId" gorm:"column:target_id;size:128;index;comment:资源 ID(单 ID 或 memoryId:fileKey 复合)"`
	Ip         string    `json:"ip" gorm:"column:ip;size:64;index;comment:客户端 IP"`
	Status     string    `json:"status" gorm:"column:status;size:16;index;comment:授权结果状态"`
	LatencyMs  int       `json:"latencyMs" gorm:"column:latency_ms;comment:处理耗时(ms)"`
	CreatedAt  time.Time `json:"createdAt" gorm:"column:created_at;autoCreateTime;index;comment:创建时间"`
}

func (AuditLog) TableName() string {
	return "audit_log"
}

// AuditLog 状态常量。
const (
	AuditStatusOK             = "ok"
	AuditStatusNotFound       = "not_found"
	AuditStatusForbidden      = "forbidden"
	AuditStatusInvalidRequest = "invalid_request"
	AuditStatusInternalError  = "internal_error"
	AuditStatusRateLimited    = "rate_limited"
)

// AuditLog 操作类型常量。
const (
	AuditActionDownloadAuthorize = "download_authorize"
)
