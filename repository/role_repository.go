package repository

import (
	"context"
	"echo-core/config"
	"echo-core/models"
	"errors"
	"time"

	"gorm.io/gorm"
)

// RoleRepository 角色仓储
type RoleRepository struct {
	db *gorm.DB
}

// NewRoleRepository 构造 RoleRepository
func NewRoleRepository() *RoleRepository {
	return &RoleRepository{db: config.GetDB()}
}

// GetByID 根据主键获取角色
func (r *RoleRepository) GetByID(ctx context.Context, id uint) (*models.Role, error) {
	start := time.Now()
	var role models.Role
	err := r.db.WithContext(ctx).First(&role, id).Error
	repoLog(ctx, "RoleRepository.GetByID", start, err, "id="+uintToStr(id))
	if err != nil {
		return nil, err
	}
	return &role, nil
}

// GetByUserIDName 根据 (userId, name) 获取角色
func (r *RoleRepository) GetByUserIDName(ctx context.Context, userId, name string) (*models.Role, error) {
	start := time.Now()
	var role models.Role
	err := r.db.WithContext(ctx).Where("user_id = ? AND name = ? AND status = 1", userId, name).First(&role).Error
	repoLog(ctx, "RoleRepository.GetByUserIDName", start, err, "userId="+userId+" name="+name)
	if err != nil {
		return nil, err
	}
	return &role, nil
}

// ExistsByUserIDName 判断同用户下是否已有同名角色
func (r *RoleRepository) ExistsByUserIDName(ctx context.Context, userId, name string) (bool, error) {
	start := time.Now()
	var count int64
	err := r.db.WithContext(ctx).Model(&models.Role{}).
		Where("user_id = ? AND name = ? AND status = 1", userId, name).
		Count(&count).Error
	repoLog(ctx, "RoleRepository.ExistsByUserIDName", start, err, "userId="+userId+" name="+name)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// Create 新建角色
func (r *RoleRepository) Create(ctx context.Context, role *models.Role) error {
	if role == nil {
		return errors.New("role is nil")
	}
	if role.Status == 0 {
		role.Status = 1
	}
	start := time.Now()
	err := r.db.WithContext(ctx).Create(role).Error
	repoLog(ctx, "RoleRepository.Create", start, err, "userId="+role.UserId+" name="+role.Name)
	return err
}

// Update 更新角色（按主键）
func (r *RoleRepository) Update(ctx context.Context, id uint, updates map[string]interface{}) error {
	start := time.Now()
	err := r.db.WithContext(ctx).Model(&models.Role{}).Where("id = ?", id).Updates(updates).Error
	repoLog(ctx, "RoleRepository.Update", start, err, "id="+uintToStr(id))
	return err
}

// Delete 软删除
func (r *RoleRepository) Delete(ctx context.Context, id uint) error {
	start := time.Now()
	err := r.db.WithContext(ctx).Model(&models.Role{}).Where("id = ?", id).Update("status", 2).Error
	repoLog(ctx, "RoleRepository.Delete", start, err, "id="+uintToStr(id))
	return err
}

// GetList 列出某用户下的全部角色（status=1）
func (r *RoleRepository) GetList(ctx context.Context, userId string) ([]models.Role, error) {
	start := time.Now()
	var roles []models.Role
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND status = 1", userId).
		Order("created_at ASC").
		Find(&roles).Error
	repoLog(ctx, "RoleRepository.GetList", start, err, "userId="+userId+" count="+intToStr(len(roles)))
	if err != nil {
		return nil, err
	}
	return roles, nil
}

// CountByUserID 统计某用户当前有效角色数
func (r *RoleRepository) CountByUserID(ctx context.Context, userId string) (int64, error) {
	start := time.Now()
	var count int64
	err := r.db.WithContext(ctx).Model(&models.Role{}).
		Where("user_id = ? AND status = 1", userId).
		Count(&count).Error
	repoLog(ctx, "RoleRepository.CountByUserID", start, err, "userId="+userId)
	if err != nil {
		return 0, err
	}
	return count, nil
}