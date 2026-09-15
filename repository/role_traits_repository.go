package repository

import (
	"context"
	"echo-core/config"
	"echo-core/models"
	"time"

	"gorm.io/gorm"
)

// RoleTraitsRepository OCEAN 五维性格(role_traits)的持久化封装。
//
// 关键设计：每对 (user_id, role_id) 下每个 preset_type 独立保存一份 OCEAN。
// 切换预设保存时只会更新对应预设的那一行,不会影响其他预设的自定义值。
type RoleTraitsRepository struct {
	db *gorm.DB
}

func NewRoleTraitsRepository() *RoleTraitsRepository {
	return &RoleTraitsRepository{db: config.GetDB()}
}

// GetByUserRolePreset 按 (user_id, role_id, preset_type) 取某预设的 OCEAN 五维
func (r *RoleTraitsRepository) GetByUserRolePreset(ctx context.Context, userId, roleId uint, presetType string) (*models.RoleTraits, error) {
	start := time.Now()
	var t models.RoleTraits
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND role_id = ? AND preset_type = ?", userId, roleId, presetType).
		First(&t).Error
	repoLog(ctx, "RoleTraitsRepository.GetByUserRolePreset", start, err,
		"userId="+uintToStr(userId)+" roleId="+uintToStr(roleId)+" preset="+presetType)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// ListByUserRole 取某 (user_id, role_id) 下所有预设的 OCEAN 五维
//
// 返回数组(可能为空) — 每个元素对应一个 preset_type 的独立自定义值。
// 切换预设展示时:若该 preset 在数组中无对应行,前端应展示预设基线;
// 若存在,则展示用户的自定义值(覆盖基线)。
func (r *RoleTraitsRepository) ListByUserRole(ctx context.Context, userId, roleId uint) ([]models.RoleTraits, error) {
	start := time.Now()
	var list []models.RoleTraits
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND role_id = ?", userId, roleId).
		Order("preset_type ASC").
		Find(&list).Error
	repoLog(ctx, "RoleTraitsRepository.ListByUserRole", start, err,
		"userId="+uintToStr(userId)+" roleId="+uintToStr(roleId)+" count="+floatToStr(float64(len(list))))
	return list, err
}

// UpsertWithTx 在事务中写入某预设的 OCEAN 五维(无则 INSERT,有则 UPDATE)。
//
// 写入键 = (user_id, role_id, preset_type):
//   - 不同 preset 写入不同行,互不影响
//   - 同 preset 多次写入会覆盖(正常业务:用户保存同一预设的新值)
//
// service 层在调用本方法前已经做了 [-2,2] 校验 + 单步 0.05 上限校验。
// tx 传 nil 时使用 r.db（兼容 M0 阶段暂无事务场景）。
//
// 设计要点：用 SELECT-then-UPDATE/INSERT 而非 FirstOrCreate+Assign,
// 因为 FirstOrCreate+Assign 在已存在行时只 SELECT 不 UPDATE(行为与直觉相反)。
// 详见 role_mood_repository.go 注释。
func (r *RoleTraitsRepository) UpsertWithTx(ctx context.Context, tx *gorm.DB, t *models.RoleTraits) error {
	start := time.Now()
	db := tx
	if db == nil {
		db = r.db
	}
	var existing models.RoleTraits
	err := db.WithContext(ctx).
		Where("user_id = ? AND role_id = ? AND preset_type = ?", t.UserID, t.RoleID, t.PresetType).
		First(&existing).Error
	if err == nil {
		// 已存在:UPDATE 五维(preset_type 不可变,作为主键一部分)
		err = db.WithContext(ctx).Model(&models.RoleTraits{}).
			Where("user_id = ? AND role_id = ? AND preset_type = ?", t.UserID, t.RoleID, t.PresetType).
			Updates(map[string]interface{}{
				"openness":          t.Openness,
				"conscientiousness": t.Conscientiousness,
				"extraversion":      t.Extraversion,
				"agreeableness":     t.Agreeableness,
				"neuroticism":       t.Neuroticism,
			}).Error
	} else if err == gorm.ErrRecordNotFound {
		// 不存在:INSERT
		err = db.WithContext(ctx).Create(t).Error
	}
	repoLog(ctx, "RoleTraitsRepository.UpsertWithTx", start, err,
		"userId="+uintToStr(t.UserID)+" roleId="+uintToStr(t.RoleID)+
			" preset="+t.PresetType+
			" O="+floatToStr(t.Openness)+
			" C="+floatToStr(t.Conscientiousness)+
			" E="+floatToStr(t.Extraversion))
	return err
}

// UpdateDeltaWithTx 单步增量更新某预设的 OCEAN 五维(service 层做硬上限校验)。
//
// 用于 feedback 累积等场景:传入 delta 值叠加到当前值上。
// 必传 presetType(每个预设独立累积,不会跨预设)。
// service 层必须保证 |delta| <= 0.05 (单步上限)。
func (r *RoleTraitsRepository) UpdateDeltaWithTx(ctx context.Context, tx *gorm.DB, userId, roleId uint,
	presetType string, updates map[string]float64) error {
	start := time.Now()
	cols := make(map[string]interface{}, len(updates))
	for k, v := range updates {
		cols[k] = v // 这里传的是"绝对新值",service 已算好
	}
	err := tx.WithContext(ctx).Model(&models.RoleTraits{}).
		Where("user_id = ? AND role_id = ? AND preset_type = ?", userId, roleId, presetType).
		Updates(cols).Error
	repoLog(ctx, "RoleTraitsRepository.UpdateDeltaWithTx", start, err,
		"userId="+uintToStr(userId)+" roleId="+uintToStr(roleId)+" preset="+presetType)
	return err
}
