package repository

import (
	"context"
	"echo-core/config"
	"echo-core/models"
	"time"

	"gorm.io/gorm"
)

// RoleBeliefRepository 信念清单(role_belief)的持久化封装。
type RoleBeliefRepository struct {
	db *gorm.DB
}

func NewRoleBeliefRepository() *RoleBeliefRepository {
	return &RoleBeliefRepository{db: config.GetDB()}
}

// Create 新增信念(user_confirmed=true 才会被业务层写入本表)。
func (r *RoleBeliefRepository) Create(ctx context.Context, b *models.RoleBelief) error {
	start := time.Now()
	err := r.db.WithContext(ctx).Create(b).Error
	repoLog(ctx, "RoleBeliefRepository.Create", start, err,
		"userId="+uintToStr(b.UserID)+" roleId="+uintToStr(b.RoleID)+
			" topic="+b.Topic)
	return err
}

// GetByID 按主键取信念
func (r *RoleBeliefRepository) GetByID(ctx context.Context, id uint) (*models.RoleBelief, error) {
	start := time.Now()
	var b models.RoleBelief
	err := r.db.WithContext(ctx).First(&b, id).Error
	repoLog(ctx, "RoleBeliefRepository.GetByID", start, err, "id="+uintToStr(id))
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// Update 更新信念(按主键)
func (r *RoleBeliefRepository) Update(ctx context.Context, id uint, updates map[string]interface{}) error {
	start := time.Now()
	err := r.db.WithContext(ctx).Model(&models.RoleBelief{}).Where("id = ?", id).Updates(updates).Error
	repoLog(ctx, "RoleBeliefRepository.Update", start, err, "id="+uintToStr(id))
	return err
}

// Delete 物理删除信念
func (r *RoleBeliefRepository) Delete(ctx context.Context, id uint) error {
	start := time.Now()
	err := r.db.WithContext(ctx).Where("id = ?", id).Delete(&models.RoleBelief{}).Error
	repoLog(ctx, "RoleBeliefRepository.Delete", start, err, "id="+uintToStr(id))
	return err
}

// ListByUserRole 列出某用户某角色下的已确认信念。
//
//   - userConfirmed=true: 用户已确认的信念（主表生效）
//   - userConfirmed=false: LLM 抽取但未确认的（一般从 suggestions 表来,不在本表）
//   - 默认仅返回 userConfirmed=true
func (r *RoleBeliefRepository) ListByUserRole(ctx context.Context, userId, roleId uint, includeUnconfirmed bool) ([]models.RoleBelief, error) {
	start := time.Now()
	var list []models.RoleBelief
	query := r.db.WithContext(ctx).Model(&models.RoleBelief{}).
		Where("user_id = ? AND role_id = ?", userId, roleId)
	if !includeUnconfirmed {
		query = query.Where("user_confirmed = ?", true)
	}
	err := query.Order("created_at DESC").Find(&list).Error
	repoLog(ctx, "RoleBeliefRepository.ListByUserRole", start, err,
		"userId="+uintToStr(userId)+" roleId="+uintToStr(roleId)+
			" count="+intToStr(len(list)))
	return list, err
}

// ConfirmWithTx 把一条 LLM 抽取的信念标记为已确认(从 suggestions 流转来)。
func (r *RoleBeliefRepository) ConfirmWithTx(ctx context.Context, tx *gorm.DB, id uint) error {
	start := time.Now()
	err := tx.WithContext(ctx).Model(&models.RoleBelief{}).
		Where("id = ?", id).
		Update("user_confirmed", true).Error
	repoLog(ctx, "RoleBeliefRepository.ConfirmWithTx", start, err, "id="+uintToStr(id))
	return err
}