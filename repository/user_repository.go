package repository

import (
	"echo-core/config"
	"echo-core/models"
	"errors"
	"time"

	"gorm.io/gorm"
)

// UserRepository 用户仓储
type UserRepository struct {
	db *gorm.DB
}

// NewUserRepository 构造 UserRepository
func NewUserRepository() *UserRepository {
	return &UserRepository{db: config.GetDB()}
}

// GetByID 根据主键获取用户
func (r *UserRepository) GetByID(id uint) (*models.User, error) {
	var u models.User
	if err := r.db.First(&u, id).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// GetByUsername 根据账号获取用户
func (r *UserRepository) GetByUsername(username string) (*models.User, error) {
	var u models.User
	if err := r.db.Where("username = ?", username).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// ExistsByUsername 判断账号是否存在
func (r *UserRepository) ExistsByUsername(username string) (bool, error) {
	var count int64
	if err := r.db.Model(&models.User{}).Where("username = ?", username).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// Create 新建用户
func (r *UserRepository) Create(user *models.User) error {
	if user == nil {
		return errors.New("user is nil")
	}
	return r.db.Create(user).Error
}

// UpdateLastLogin 更新最近登录信息
func (r *UserRepository) UpdateLastLogin(id uint, ip string, loginAt time.Time) error {
	return r.db.Model(&models.User{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"last_login_at": loginAt,
			"last_login_ip": ip,
		}).Error
}

// UpdatePassword 更新密码（哈希与盐）
func (r *UserRepository) UpdatePassword(id uint, passwordHash, salt string) error {
	return r.db.Model(&models.User{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"password_hash": passwordHash,
			"salt":          salt,
		}).Error
}

// UpdateStatus 启用/禁用用户
func (r *UserRepository) UpdateStatus(id uint, status int) error {
	return r.db.Model(&models.User{}).Where("id = ?", id).Update("status", status).Error
}
