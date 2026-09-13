package repository

import (
	"context"
	"echo-core/config"
	"echo-core/models"
	"time"

	"gorm.io/gorm"
)

// RoleFeedbackRepository 用户反馈(role_feedback)的持久化封装。
type RoleFeedbackRepository struct {
	db *gorm.DB
}

func NewRoleFeedbackRepository() *RoleFeedbackRepository {
	return &RoleFeedbackRepository{db: config.GetDB()}
}

// Create 写入一条反馈。
func (r *RoleFeedbackRepository) Create(ctx context.Context, f *models.RoleFeedback) error {
	start := time.Now()
	err := r.db.WithContext(ctx).Create(f).Error
	repoLog(ctx, "RoleFeedbackRepository.Create", start, err,
		"userId="+uintToStr(f.UserID)+" roleId="+uintToStr(f.RoleID)+
			" rating="+intToStr(f.Rating))
	return err
}

// CountRecentByRating 统计某角色近 N 天内指定评分的条数（用于触发 baseline 调整）。
//
//   - rating: 1 或 -1(只看单向反馈)
//   - days: 统计窗口(默认 7)
//   - 返回 positive_count / negative_count / total
type FeedbackStats struct {
	PositiveCount int `json:"positiveCount"` // rating > 0
	NegativeCount int `json:"negativeCount"` // rating < 0
	Total         int `json:"total"`
}

func (r *RoleFeedbackRepository) CountRecentByRating(ctx context.Context, userId, roleId uint, days int) (*FeedbackStats, error) {
	start := time.Now()
	stats := &FeedbackStats{}
	since := time.Now().AddDate(0, 0, -days)

	rows, err := r.db.WithContext(ctx).Model(&models.RoleFeedback{}).
		Select("rating, COUNT(*) AS cnt").
		Where("user_id = ? AND role_id = ? AND created_at >= ?", userId, roleId, since).
		Group("rating").
		Rows()
	if err != nil {
		repoLog(ctx, "RoleFeedbackRepository.CountRecentByRating", start, err,
			"userId="+uintToStr(userId)+" roleId="+uintToStr(roleId))
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var rating, cnt int
		if err := rows.Scan(&rating, &cnt); err != nil {
			continue
		}
		stats.Total += cnt
		if rating > 0 {
			stats.PositiveCount += cnt
		} else if rating < 0 {
			stats.NegativeCount += cnt
		}
	}
	repoLog(ctx, "RoleFeedbackRepository.CountRecentByRating", start, nil,
		"userId="+uintToStr(userId)+" roleId="+uintToStr(roleId)+
			" positive="+intToStr(stats.PositiveCount)+
			" negative="+intToStr(stats.NegativeCount))
	return stats, nil
}