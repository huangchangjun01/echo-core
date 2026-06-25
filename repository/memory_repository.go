package repository

import (
	"echo-core/config"
	"echo-core/models"
)

// MemoryRepository 会话消息仓储
type MemoryRepository struct{}

// NewMemoryRepository 创建 MemoryRepository
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{}
}

// SaveSessionMessage 保存会话消息
func (r *MemoryRepository) SaveSessionMessage(msg *models.SessionMessage) error {
	return config.GetDB().Create(msg).Error
}

// GetSessionMessages 获取会话历史消息
func (r *MemoryRepository) GetSessionMessages(sessionID, userID string, limit int) ([]models.SessionMessage, error) {
	var messages []models.SessionMessage
	err := config.GetDB().Where("session_id = ? AND user_id = ?", sessionID, userID).
		Order("created_at asc").Limit(limit).Find(&messages).Error
	return messages, err
}

// DeleteSessionMessages 删除会话消息
func (r *MemoryRepository) DeleteSessionMessages(sessionID string) error {
	return config.GetDB().Where("session_id = ?", sessionID).Delete(&models.SessionMessage{}).Error
}

// AutoMigrate 自动迁移表结构
func (r *MemoryRepository) AutoMigrate() error {
	return config.GetDB().AutoMigrate(&models.SessionMessage{})
}