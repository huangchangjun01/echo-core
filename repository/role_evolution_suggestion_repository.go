package repository

import (
	"context"
	"echo-core/config"
	"echo-core/models"
	"time"

	"gorm.io/gorm"
)

// RoleEvolutionSuggestionRepository 角色内核演化建议(role_evolution_suggestion)的持久化封装。
//
// PRD 铁律 #1：LLM 抽取只写本表,主表（persona/traits/belief）的所有自动更新必须经用户 Apply 后由 service 层从本表搬移。
type RoleEvolutionSuggestionRepository struct {
	db *gorm.DB
}

func NewRoleEvolutionSuggestionRepository() *RoleEvolutionSuggestionRepository {
	return &RoleEvolutionSuggestionRepository{db: config.GetDB()}
}

// Create 写入一条建议(LLM 抽取后调用,默认 status=pending)。
func (r *RoleEvolutionSuggestionRepository) Create(ctx context.Context, s *models.RoleEvolutionSuggestion) error {
	if s.Status == "" {
	 s.Status = models.SuggestionStatusPending
	}
	start := time.Now()
	err := r.db.WithContext(ctx).Create(s).Error
	repoLog(ctx, "RoleEvolutionSuggestionRepository.Create", start, err,
		"userId="+uintToStr(s.UserID)+" roleId="+uintToStr(s.RoleID)+
			" targetType="+s.TargetType+" source="+s.Source)
	return err
}

// GetByID 按主键取建议。
func (r *RoleEvolutionSuggestionRepository) GetByID(ctx context.Context, id uint) (*models.RoleEvolutionSuggestion, error) {
	start := time.Now()
	var s models.RoleEvolutionSuggestion
	err := r.db.WithContext(ctx).First(&s, id).Error
	repoLog(ctx, "RoleEvolutionSuggestionRepository.GetByID", start, err, "id="+uintToStr(id))
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// UpdateStatus 更新状态(applied/skipped/edited)。
func (r *RoleEvolutionSuggestionRepository) UpdateStatus(ctx context.Context, id uint, status string) error {
	start := time.Now()
	now := time.Now()
	err := r.db.WithContext(ctx).Model(&models.RoleEvolutionSuggestion{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":      status,
			"resolved_at": &now,
		}).Error
	repoLog(ctx, "RoleEvolutionSuggestionRepository.UpdateStatus", start, err,
		"id="+uintToStr(id)+" status="+status)
	return err
}

// ListPendingByUserRole 列出某用户某角色下所有 pending 状态的建议。
func (r *RoleEvolutionSuggestionRepository) ListPendingByUserRole(ctx context.Context, userId, roleId uint) ([]models.RoleEvolutionSuggestion, error) {
	start := time.Now()
	var list []models.RoleEvolutionSuggestion
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND role_id = ? AND status = ?", userId, roleId, models.SuggestionStatusPending).
		Order("created_at DESC").
		Find(&list).Error
	repoLog(ctx, "RoleEvolutionSuggestionRepository.ListPendingByUserRole", start, err,
		"userId="+uintToStr(userId)+" roleId="+uintToStr(roleId)+
			" count="+intToStr(len(list)))
	return list, err
}