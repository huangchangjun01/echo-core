package repository

import (
	"context"
	"echo-core/config"
	"echo-core/models"
	"time"

	"gorm.io/gorm"
)

// AuditLogRepository 审计日志持久化封装。
//
// 全部继承 WithContext(ctx)，日志走统一 repoLog。
type AuditLogRepository struct {
	db *gorm.DB
}

// NewAuditLogRepository 构造
func NewAuditLogRepository() *AuditLogRepository {
	return &AuditLogRepository{db: config.GetDB()}
}

// Create 写入一条审计日志。
//
// 设计：best-effort；调用方应自行处理返回错误（一般仅记日志、不影响主业务返回）。
func (r *AuditLogRepository) Create(ctx context.Context, log *models.AuditLog) error {
	start := time.Now()
	err := r.db.WithContext(ctx).Create(log).Error
	repoLog(ctx, "AuditLogRepository.Create", start, err,
		"action="+log.Action+" targetType="+log.TargetType+" status="+log.Status)
	return err
}
