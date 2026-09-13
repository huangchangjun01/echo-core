package repository

import (
	"context"
	"echo-core/config"
	"echo-core/models"
	"time"

	"gorm.io/gorm"
)

// RoleMoodRepository 三桶心情(role_mood)的持久化封装。
type RoleMoodRepository struct {
	db *gorm.DB
}

func NewRoleMoodRepository() *RoleMoodRepository {
	return &RoleMoodRepository{db: config.GetDB()}
}

// GetByUserRole 按 (user_id, role_id) 取心情快照
func (r *RoleMoodRepository) GetByUserRole(ctx context.Context, userId, roleId uint) (*models.RoleMood, error) {
	start := time.Now()
	var m models.RoleMood
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND role_id = ?", userId, roleId).
		First(&m).Error
	repoLog(ctx, "RoleMoodRepository.GetByUserRole", start, err,
		"userId="+uintToStr(userId)+" roleId="+uintToStr(roleId))
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// UpsertWithTx 在事务中写入三桶心情。
//
// service 层在调用前已经做完三桶耦合公式计算 + 衰减计算。
// tx 传 nil 时使用 r.db（兼容 M0 阶段暂无事务场景）。
//
// 设计要点：用 SELECT-then-UPDATE/INSERT 而非 FirstOrCreate+Assign,
// 因为 FirstOrCreate+Assign 在已存在行时只 SELECT 不 UPDATE(行为与直觉相反)。
func (r *RoleMoodRepository) UpsertWithTx(ctx context.Context, tx *gorm.DB, m *models.RoleMood) error {
	start := time.Now()
	db := tx
	if db == nil {
		db = r.db
	}
	var existing models.RoleMood
	err := db.WithContext(ctx).
		Where("user_id = ? AND role_id = ?", m.UserID, m.RoleID).
		First(&existing).Error
	if err == nil {
		err = db.WithContext(ctx).Model(&models.RoleMood{}).
			Where("user_id = ? AND role_id = ?", m.UserID, m.RoleID).
			Updates(map[string]interface{}{
				"instant_val":        m.InstantVal,
				"instant_intensity":  m.InstantIntensity,
				"instant_emotion":    m.InstantEmotion,
				"short_val":          m.ShortVal,
				"short_intensity":    m.ShortIntensity,
				"short_emotion":      m.ShortEmotion,
				"baseline_val":       m.BaselineVal,
				"baseline_intensity": m.BaselineIntensity,
				"baseline_emotion":   m.BaselineEmotion,
				"last_event_at":      m.LastEventAt,
			}).Error
	} else if err == gorm.ErrRecordNotFound {
		err = db.WithContext(ctx).Create(m).Error
	}
	repoLog(ctx, "RoleMoodRepository.UpsertWithTx", start, err,
		"userId="+uintToStr(m.UserID)+" roleId="+uintToStr(m.RoleID)+
			" instant="+floatToStr(m.InstantVal))
	return err
}

// ListAllWithLastEventBefore 列出所有 (user_id, role_id, last_event_at < before) 的心情
// 用于后台调度器批量衰减。
func (r *RoleMoodRepository) ListAllWithLastEventBefore(ctx context.Context, before time.Time) ([]models.RoleMood, error) {
	start := time.Now()
	var list []models.RoleMood
	err := r.db.WithContext(ctx).
		Where("last_event_at < ?", before).
		Find(&list).Error
	repoLog(ctx, "RoleMoodRepository.ListAllWithLastEventBefore", start, err,
		"before="+before.Format(time.RFC3339)+" count="+intToStr(len(list)))
	return list, err
}