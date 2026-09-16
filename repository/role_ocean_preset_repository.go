package repository

import (
	"context"
	"echo-core/config"
	"echo-core/models"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RoleOceanPresetRepository OCEAN 预设字典的持久化封装。
type RoleOceanPresetRepository struct {
	db *gorm.DB
}

func NewRoleOceanPresetRepository() *RoleOceanPresetRepository {
	return &RoleOceanPresetRepository{db: config.GetDB()}
}

// ListByUserRole 列出某 (user, role) 下的所有预设,按 id 升序(种子在前面,自建在后)。
//
// 性格页重构后预设字典按角色隔离,每个 (user, role) 独立一份。
func (r *RoleOceanPresetRepository) ListByUserRole(ctx context.Context, userId, roleId uint) ([]models.RoleOceanPreset, error) {
	start := time.Now()
	var list []models.RoleOceanPreset
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND role_id = ?", userId, roleId).
		Order("id ASC").
		Find(&list).Error
	repoLog(ctx, "RoleOceanPresetRepository.ListByUserRole", start, err,
		"userId="+uintToStr(userId)+" roleId="+uintToStr(roleId)+" count="+intToStr(len(list)))
	return list, err
}

// CountByUserRole 统计某 (user, role) 下已存在的预设数量(用于"首次访问时种入默认")。
func (r *RoleOceanPresetRepository) CountByUserRole(ctx context.Context, userId, roleId uint) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&models.RoleOceanPreset{}).
		Where("user_id = ? AND role_id = ?", userId, roleId).Count(&n).Error
	return n, err
}

// Create 新增一个预设
func (r *RoleOceanPresetRepository) Create(ctx context.Context, p *models.RoleOceanPreset) error {
	start := time.Now()
	err := r.db.WithContext(ctx).Create(p).Error
	repoLog(ctx, "RoleOceanPresetRepository.Create", start, err,
		"userId="+uintToStr(p.UserID)+" roleId="+uintToStr(p.RoleID)+" key="+p.Key)
	return err
}

// BatchCreate 批量插入(种子场景),使用 ON DUPLICATE KEY UPDATE id=id 让 MySQL
// 把"已存在"行变成 no-op,避免并发首次访问时两个请求都尝试 seed 而第二个失败。
func (r *RoleOceanPresetRepository) BatchCreate(ctx context.Context, list []models.RoleOceanPreset) error {
	start := time.Now()
	if len(list) == 0 {
		return nil
	}
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&list).Error
	repoLog(ctx, "RoleOceanPresetRepository.BatchCreate", start, err,
		"count="+intToStr(len(list)))
	return err
}

// GetByID 按主键查找
func (r *RoleOceanPresetRepository) GetByID(ctx context.Context, id uint) (*models.RoleOceanPreset, error) {
	start := time.Now()
	var p models.RoleOceanPreset
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&p).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		repoLog(ctx, "RoleOceanPresetRepository.GetByID", start, err, "id="+intToStr(int(id)))
		return nil, err
	}
	repoLog(ctx, "RoleOceanPresetRepository.GetByID", start, nil, "id="+intToStr(int(id)))
	return &p, nil
}

// GetByUserRoleKey 按 (user_id, role_id, key) 查找 —— 角色级预设查询。
func (r *RoleOceanPresetRepository) GetByUserRoleKey(ctx context.Context, userId, roleId uint, key string) (*models.RoleOceanPreset, error) {
	start := time.Now()
	var p models.RoleOceanPreset
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND role_id = ? AND `key` = ?", userId, roleId, key).
		First(&p).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		repoLog(ctx, "RoleOceanPresetRepository.GetByUserRoleKey", start, err,
			"userId="+uintToStr(userId)+" roleId="+uintToStr(roleId)+" key="+key)
		return nil, err
	}
	repoLog(ctx, "RoleOceanPresetRepository.GetByUserRoleKey", start, nil,
		"userId="+uintToStr(userId)+" roleId="+uintToStr(roleId)+" key="+key)
	return &p, nil
}

// Update 修改 label/description/OCEAN 五维
func (r *RoleOceanPresetRepository) Update(ctx context.Context, p *models.RoleOceanPreset) error {
	start := time.Now()
	err := r.db.WithContext(ctx).Model(&models.RoleOceanPreset{}).
		Where("id = ?", p.ID).
		Updates(map[string]interface{}{
			"label":             p.Label,
			"description":       p.Description,
			"openness":          p.Openness,
			"conscientiousness": p.Conscientiousness,
			"extraversion":      p.Extraversion,
			"agreeableness":     p.Agreeableness,
			"neuroticism":       p.Neuroticism,
		}).Error
	repoLog(ctx, "RoleOceanPresetRepository.Update", start, err,
		"id="+intToStr(int(p.ID))+" key="+p.Key)
	return err
}

// Delete 按主键删除(物理删除;is_protected 由 service 层拦截)
func (r *RoleOceanPresetRepository) Delete(ctx context.Context, id uint) error {
	start := time.Now()
	err := r.db.WithContext(ctx).
		Where("id = ?", id).
		Delete(&models.RoleOceanPreset{}).Error
	repoLog(ctx, "RoleOceanPresetRepository.Delete", start, err, "id="+intToStr(int(id)))
	return err
}
