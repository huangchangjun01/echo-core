package repository

import (
	"context"
	"echo-core/config"
	"echo-core/models"
	"time"

	"gorm.io/gorm"
)

// RoleTraitsRepository OCEAN 五维性格(role_traits)的持久化封装。
type RoleTraitsRepository struct {
	db *gorm.DB
}

func NewRoleTraitsRepository() *RoleTraitsRepository {
	return &RoleTraitsRepository{db: config.GetDB()}
}

// GetByUserRole 按 (user_id, role_id) 取 OCEAN 五维
func (r *RoleTraitsRepository) GetByUserRole(ctx context.Context, userId, roleId uint) (*models.RoleTraits, error) {
	start := time.Now()
	var t models.RoleTraits
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND role_id = ?", userId, roleId).
		First(&t).Error
	repoLog(ctx, "RoleTraitsRepository.GetByUserRole", start, err,
		"userId="+uintToStr(userId)+" roleId="+uintToStr(roleId))
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// UpsertWithTx 在事务中写入 OCEAN 五维(无则 INSERT,有则 UPDATE)。
//
// service 层在调用本方法前已经做了 [-1,1] 校验 + 单步 0.05 上限校验。
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
		Where("user_id = ? AND role_id = ?", t.UserID, t.RoleID).
		First(&existing).Error
	if err == nil {
		// 已存在：UPDATE 全字段
		err = db.WithContext(ctx).Model(&models.RoleTraits{}).
			Where("user_id = ? AND role_id = ?", t.UserID, t.RoleID).
			Updates(map[string]interface{}{
				"openness":          t.Openness,
				"conscientiousness": t.Conscientiousness,
				"extraversion":      t.Extraversion,
				"agreeableness":     t.Agreeableness,
				"neuroticism":       t.Neuroticism,
				"preset_type":       t.PresetType,
			}).Error
	} else if err == gorm.ErrRecordNotFound {
		// 不存在：INSERT
		err = db.WithContext(ctx).Create(t).Error
	}
	repoLog(ctx, "RoleTraitsRepository.UpsertWithTx", start, err,
		"userId="+uintToStr(t.UserID)+" roleId="+uintToStr(t.RoleID)+
			" O="+floatToStr(t.Openness)+
			" C="+floatToStr(t.Conscientiousness)+
			" E="+floatToStr(t.Extraversion))
	return err
}

// UpdateDeltaWithTx 单步增量更新 OCEAN 五维(service 层做硬上限校验)。
//
// 用于 feedback 累积等场景：传入 delta 值叠加到当前值上。
// service 层必须保证 |delta| <= 0.05 (单步上限)。
func (r *RoleTraitsRepository) UpdateDeltaWithTx(ctx context.Context, tx *gorm.DB, userId, roleId uint,
	deltas map[string]float64) error {
	start := time.Now()
	updates := make(map[string]interface{}, len(deltas))
	for k, v := range deltas {
		updates[k] = v // 这里传的是"绝对新值",service 已算好
	}
	err := tx.WithContext(ctx).Model(&models.RoleTraits{}).
		Where("user_id = ? AND role_id = ?", userId, roleId).
		Updates(updates).Error
	repoLog(ctx, "RoleTraitsRepository.UpdateDeltaWithTx", start, err,
		"userId="+uintToStr(userId)+" roleId="+uintToStr(roleId))
	return err
}