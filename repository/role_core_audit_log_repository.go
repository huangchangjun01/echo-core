package repository

import (
	"context"
	"echo-core/config"
	"echo-core/models"
	"time"

	"gorm.io/gorm"
)

// RoleCoreAuditLogRepository 角色内核审计日志(role_core_audit_log)的持久化封装。
type RoleCoreAuditLogRepository struct {
	db *gorm.DB
}

func NewRoleCoreAuditLogRepository() *RoleCoreAuditLogRepository {
	return &RoleCoreAuditLogRepository{db: config.GetDB()}
}

// Create 写入一条审计日志(best-effort,失败仅记日志不影响主业务)。
func (r *RoleCoreAuditLogRepository) Create(ctx context.Context, log *models.RoleCoreAuditLog) error {
	start := time.Now()
	err := r.db.WithContext(ctx).Create(log).Error
	repoLog(ctx, "RoleCoreAuditLogRepository.Create", start, err,
		"action="+log.Action+" targetType="+log.TargetType)
	return err
}

// ListByTarget 按 (user_id, role_id, target_type) 列出最近 N 条审计。
func (r *RoleCoreAuditLogRepository) ListByTarget(ctx context.Context, userId, roleId uint, targetType string, limit int) ([]models.RoleCoreAuditLog, error) {
	start := time.Now()
	var list []models.RoleCoreAuditLog
	query := r.db.WithContext(ctx).Model(&models.RoleCoreAuditLog{}).
		Where("user_id = ? AND role_id = ?", userId, roleId).
		Order("created_at DESC")
	if targetType != "" {
		query = query.Where("target_type = ?", targetType)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}
	err := query.Find(&list).Error
	repoLog(ctx, "RoleCoreAuditLogRepository.ListByTarget", start, err,
		"userId="+uintToStr(userId)+" roleId="+uintToStr(roleId)+
			" targetType="+targetType+" count="+intToStr(len(list)))
	return list, err
}

// GetLatestBefore 按 (user_id, role_id, target_type) 取最新一条审计之前的快照。
// 用于一键回滚场景。
func (r *RoleCoreAuditLogRepository) GetLatestBefore(ctx context.Context, userId, roleId uint, targetType string) (*models.RoleCoreAuditLog, error) {
	start := time.Now()
	var log models.RoleCoreAuditLog
	err := r.db.WithContext(ctx).Model(&models.RoleCoreAuditLog{}).
		Where("user_id = ? AND role_id = ? AND target_type = ?", userId, roleId, targetType).
		Order("created_at DESC").
		First(&log).Error
	if err != nil {
		repoLog(ctx, "RoleCoreAuditLogRepository.GetLatestBefore", start, err,
			"userId="+uintToStr(userId)+" roleId="+uintToStr(roleId)+" targetType="+targetType)
		return nil, err
	}
	repoLog(ctx, "RoleCoreAuditLogRepository.GetLatestBefore", start, nil,
		"id="+uintToStr(log.ID))
	return &log, nil
}