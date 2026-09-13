package repository

import (
	"context"
	"echo-core/config"
	"echo-core/models"
	"time"

	"gorm.io/gorm"
)

// RolePersonaRepository 结构化人格(role_persona)的持久化封装。
// 全部继承 WithContext(ctx),日志走统一 repoLog。
type RolePersonaRepository struct {
	db *gorm.DB
}

func NewRolePersonaRepository() *RolePersonaRepository {
	return &RolePersonaRepository{db: config.GetDB()}
}

// GetByUserRole 按 (user_id, role_id) 取人格
func (r *RolePersonaRepository) GetByUserRole(ctx context.Context, userId, roleId uint) (*models.RolePersona, error) {
	start := time.Now()
	var p models.RolePersona
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND role_id = ?", userId, roleId).
		First(&p).Error
	repoLog(ctx, "RolePersonaRepository.GetByUserRole", start, err,
		"userId="+uintToStr(userId)+" roleId="+uintToStr(roleId))
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// UpsertWithTx 在事务中写入人格(无则 INSERT,有则 UPDATE 并 version+1)。
//
// 乐观锁策略：每次写入前先 SELECT 旧 version，写入时 version=旧值+1。
// tx 传 nil 时使用 r.db（兼容 M0 阶段暂无事务场景）。
//
// 设计要点：用 SELECT-then-UPDATE/INSERT 而非 FirstOrCreate+Assign,
// 因为 FirstOrCreate+Assign 在已存在行时只 SELECT 不 UPDATE(行为与直觉相反)。
// 详见 role_mood_repository.go 注释。
func (r *RolePersonaRepository) UpsertWithTx(ctx context.Context, tx *gorm.DB, p *models.RolePersona) error {
	start := time.Now()
	db := tx
	if db == nil {
		db = r.db
	}
	// 1. 先查旧 version（乐观锁计数器）
	var oldVersion int
	err := db.WithContext(ctx).Model(&models.RolePersona{}).
		Where("user_id = ? AND role_id = ?", p.UserID, p.RoleID).
		Select("version").Scan(&oldVersion).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		repoLog(ctx, "RolePersonaRepository.UpsertWithTx.query", start, err, "userId="+uintToStr(p.UserID))
		return err
	}
	p.Version = oldVersion + 1

	// 2. 真正 upsert：SELECT-then-UPDATE/INSERT
	var existing models.RolePersona
	err = db.WithContext(ctx).
		Where("user_id = ? AND role_id = ?", p.UserID, p.RoleID).
		First(&existing).Error
	if err == nil {
		// 已存在：UPDATE 全字段
		err = db.WithContext(ctx).Model(&models.RolePersona{}).
			Where("user_id = ? AND role_id = ?", p.UserID, p.RoleID).
			Updates(map[string]interface{}{
				"identity":        p.Identity,
				"background":      p.Background,
				"traits_tags":     p.TraitsTags,
				"values":          p.Values,
				"speaking_style":  p.SpeakingStyle,
				"taboos":          p.Taboos,
				"example_dialogs": p.ExampleDialogs,
				"version":         p.Version,
			}).Error
	} else if err == gorm.ErrRecordNotFound {
		// 不存在：INSERT
		err = db.WithContext(ctx).Create(p).Error
	}
	repoLog(ctx, "RolePersonaRepository.UpsertWithTx", start, err,
		"userId="+uintToStr(p.UserID)+" roleId="+uintToStr(p.RoleID)+" version="+intToStr(p.Version))
	return err
}

// DeleteWithTx 物理删除人格（级联删除场景）。
func (r *RolePersonaRepository) DeleteWithTx(ctx context.Context, tx *gorm.DB, userId, roleId uint) error {
	start := time.Now()
	err := tx.WithContext(ctx).Where("user_id = ? AND role_id = ?", userId, roleId).
		Delete(&models.RolePersona{}).Error
	repoLog(ctx, "RolePersonaRepository.DeleteWithTx", start, err,
		"userId="+uintToStr(userId)+" roleId="+uintToStr(roleId))
	return err
}