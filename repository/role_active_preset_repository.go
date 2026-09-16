package repository

import (
	"context"
	"echo-core/config"
	"echo-core/models"
	"time"

	"gorm.io/gorm"
)

// RoleActivePresetRepository 当前使用的 OCEAN 预设(role_active_preset)的持久化封装。
//
// 关键设计：
//   - 每对 (user_id, role_id) 只有一条 active 记录(复合主键保证唯一)。
//   - SaveTraits 时 service 层自动把被保存的 preset Upsert 到此表。
//   - 删除 preset 前 service 层清理对应的 active 记录,避免悬空引用。
type RoleActivePresetRepository struct {
	db *gorm.DB
}

func NewRoleActivePresetRepository() *RoleActivePresetRepository {
	return &RoleActivePresetRepository{db: config.GetDB()}
}

// GetByUserRole 按 (user_id, role_id) 取 active preset 记录。
//
// 返回 nil + nil 表示从未设置过 active preset(全新角色,默认走 companion_default)。
// 返回 *RoleActivePreset + nil 表示找到。
// 返回 nil + err 表示查询出错(gorm.ErrRecordNotFound 不算错,转 nil 返回)。
func (r *RoleActivePresetRepository) GetByUserRole(ctx context.Context, userId, roleId uint) (*models.RoleActivePreset, error) {
	start := time.Now()
	var ap models.RoleActivePreset
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND role_id = ?", userId, roleId).
		First(&ap).Error
	if err == gorm.ErrRecordNotFound {
		repoLog(ctx, "RoleActivePresetRepository.GetByUserRole", start, nil,
			"userId="+uintToStr(userId)+" roleId="+uintToStr(roleId)+" status=not_set")
		return nil, nil
	}
	repoLog(ctx, "RoleActivePresetRepository.GetByUserRole", start, err,
		"userId="+uintToStr(userId)+" roleId="+uintToStr(roleId))
	if err != nil {
		return nil, err
	}
	return &ap, nil
}

// Upsert 写入或更新 active preset。
//
// 实现说明：用 SELECT-then-UPDATE/INSERT 而非 FirstOrCreate+Assign,
// 因为 FirstOrCreate+Assign 在已存在行时只 SELECT 不 UPDATE(行为与直觉相反)。
// 详见 role_mood_repository.go 注释。
func (r *RoleActivePresetRepository) Upsert(ctx context.Context, userId, roleId uint, presetType string) error {
	start := time.Now()
	db := r.db
	var existing models.RoleActivePreset
	err := db.WithContext(ctx).
		Where("user_id = ? AND role_id = ?", userId, roleId).
		First(&existing).Error
	if err == nil {
		// 已存在:UPDATE preset_type
		err = db.WithContext(ctx).Model(&models.RoleActivePreset{}).
			Where("user_id = ? AND role_id = ?", userId, roleId).
			Update("preset_type", presetType).Error
	} else if err == gorm.ErrRecordNotFound {
		// 不存在:INSERT
		err = db.WithContext(ctx).Create(&models.RoleActivePreset{
			UserID:     userId,
			RoleID:     roleId,
			PresetType: presetType,
		}).Error
	}
	repoLog(ctx, "RoleActivePresetRepository.Upsert", start, err,
		"userId="+uintToStr(userId)+" roleId="+uintToStr(roleId)+" preset="+presetType)
	return err
}

// Delete 删除 active preset 记录(preset 被删除时清理)。
//
// 删不到记录也返回 nil(idempotent),调用方不需关心是否存在。
func (r *RoleActivePresetRepository) Delete(ctx context.Context, userId, roleId uint) error {
	start := time.Now()
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND role_id = ?", userId, roleId).
		Delete(&models.RoleActivePreset{}).Error
	repoLog(ctx, "RoleActivePresetRepository.Delete", start, err,
		"userId="+uintToStr(userId)+" roleId="+uintToStr(roleId))
	return err
}

// DeleteByUserAndKey 删除指定 preset_type 的所有 active 记录(preset 被删时调用)。
//
// 用途:DeletePreset 时如果该 preset 恰好被设为某个 (user, role) 的 active,
// 需要清理该记录,否则后续 chat 会读到悬空引用。
// 注意:按 user_id + preset_type 删除,通常一个用户的 preset key 唯一,
// 所以最多影响一条记录。多角色共享同一 active preset 的边界情况罕见。
func (r *RoleActivePresetRepository) DeleteByUserAndKey(ctx context.Context, userId uint, presetType string) error {
	start := time.Now()
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND preset_type = ?", userId, presetType).
		Delete(&models.RoleActivePreset{}).Error
	repoLog(ctx, "RoleActivePresetRepository.DeleteByUserAndKey", start, err,
		"userId="+uintToStr(userId)+" preset="+presetType)
	return err
}

// DeleteByUserRoleKey 删除某 (user, role, presetType) 的 active 记录 —— 角色级精确清理。
//
// 设计要点:预设字典按角色隔离后,DeletePreset 只应该清理「被删 preset 所属的
// (user, role)」的 active 记录,不能误伤其它角色(同名 preset 在不同角色
// 可以有各自的 active 状态)。
//
// 与 DeleteByUserAndKey 的区别:DeleteByUserAndKey 按 user_id 全局清理(适用
// 于「preset 真的被删了」语义模糊的场景);本方法按 (user, role) 精确清理
// (适用于 preset 按角色隔离后的删除语义)。
func (r *RoleActivePresetRepository) DeleteByUserRoleKey(ctx context.Context, userId, roleId uint, presetType string) error {
	start := time.Now()
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND role_id = ? AND preset_type = ?", userId, roleId, presetType).
		Delete(&models.RoleActivePreset{}).Error
	repoLog(ctx, "RoleActivePresetRepository.DeleteByUserRoleKey", start, err,
		"userId="+uintToStr(userId)+" roleId="+uintToStr(roleId)+" preset="+presetType)
	return err
}
