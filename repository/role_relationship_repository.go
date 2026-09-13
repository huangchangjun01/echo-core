package repository

import (
	"context"
	"echo-core/config"
	"echo-core/models"
	"time"

	"gorm.io/gorm"
)

// RoleRelationshipRepository 三维关系值(role_relationship)的持久化封装。
type RoleRelationshipRepository struct {
	db *gorm.DB
}

func NewRoleRelationshipRepository() *RoleRelationshipRepository {
	return &RoleRelationshipRepository{db: config.GetDB()}
}

// GetByUserRole 按 (user_id, role_id) 取三维关系值。
// 无记录时返回 (nil, nil) — service 层负责初始化默认值。
func (r *RoleRelationshipRepository) GetByUserRole(ctx context.Context, userId, roleId uint) (*models.RoleRelationship, error) {
	start := time.Now()
	var rel models.RoleRelationship
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND role_id = ?", userId, roleId).
		First(&rel).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			repoLog(ctx, "RoleRelationshipRepository.GetByUserRole", start, nil,
				"userId="+uintToStr(userId)+" roleId="+uintToStr(roleId)+" not_found")
			return nil, nil
		}
		repoLog(ctx, "RoleRelationshipRepository.GetByUserRole", start, err,
			"userId="+uintToStr(userId)+" roleId="+uintToStr(roleId))
		return nil, err
	}
	repoLog(ctx, "RoleRelationshipRepository.GetByUserRole", start, nil,
		"userId="+uintToStr(userId)+" roleId="+uintToStr(roleId))
	return &rel, nil
}

// UpsertWithTx 在事务中写入三维关系值。
// tx 传 nil 时使用 r.db（兼容 M0 阶段暂无事务场景）。
//
// 设计要点：用 SELECT-then-UPDATE/INSERT 而非 FirstOrCreate+Assign,
// 因为 FirstOrCreate+Assign 在已存在行时只 SELECT 不 UPDATE(行为与直觉相反)。
// 详见 role_mood_repository.go 注释。
func (r *RoleRelationshipRepository) UpsertWithTx(ctx context.Context, tx *gorm.DB, rel *models.RoleRelationship) error {
	start := time.Now()
	db := tx
	if db == nil {
		db = r.db
	}
	var existing models.RoleRelationship
	err := db.WithContext(ctx).
		Where("user_id = ? AND role_id = ?", rel.UserID, rel.RoleID).
		First(&existing).Error
	if err == nil {
		// 已存在：UPDATE 全字段
		err = db.WithContext(ctx).Model(&models.RoleRelationship{}).
			Where("user_id = ? AND role_id = ?", rel.UserID, rel.RoleID).
			Updates(map[string]interface{}{
				"intimacy":     rel.Intimacy,
				"trust":        rel.Trust,
				"satisfaction": rel.Satisfaction,
				"co_days":      rel.CoDays,
			}).Error
	} else if err == gorm.ErrRecordNotFound {
		// 不存在：INSERT
		err = db.WithContext(ctx).Create(rel).Error
	}
	repoLog(ctx, "RoleRelationshipRepository.UpsertWithTx", start, err,
		"userId="+uintToStr(rel.UserID)+" roleId="+uintToStr(rel.RoleID)+
			" intimacy="+floatToStr(rel.Intimacy))
	return err
}

// IncrementCoDaysWithTx 累计陪伴天数(每天首次对话触发)。
func (r *RoleRelationshipRepository) IncrementCoDaysWithTx(ctx context.Context, tx *gorm.DB, userId, roleId uint) error {
	start := time.Now()
	err := tx.WithContext(ctx).Exec(
		"INSERT INTO role_relationship (user_id, role_id, intimacy, trust, satisfaction, co_days) "+
			"VALUES (?, ?, 0, 0, 0, 1) "+
			"ON DUPLICATE KEY UPDATE co_days = co_days + 1",
		userId, roleId).Error
	repoLog(ctx, "RoleRelationshipRepository.IncrementCoDaysWithTx", start, err,
		"userId="+uintToStr(userId)+" roleId="+uintToStr(roleId))
	return err
}