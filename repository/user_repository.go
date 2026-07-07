package repository

import (
	"context"
	"echo-core/config"
	"echo-core/models"
	"echo-core/utils"
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

// repoLog 仓储层统一日志格式：ok/err/SLOW + 延迟（毫秒）。
// ctx 用于继承上游 rid/uid，确保整条调用链都可 grep。
func repoLog(ctx context.Context, method string, start time.Time, err error, extra string) {
	latency := time.Since(start).Milliseconds()
	switch {
	case err != nil:
		utils.LogWithCtx(ctx, "Repo."+method, "err=%v latency=%dms %s", err, latency, extra)
	case latency >= 200:
		utils.LogWithCtx(ctx, "Repo."+method, "SLOW latency=%dms %s", latency, extra)
	default:
		utils.LogWithCtx(ctx, "Repo."+method, "ok latency=%dms %s", latency, extra)
	}
}

// GetByID 根据主键获取用户
func (r *UserRepository) GetByID(ctx context.Context, id uint) (*models.User, error) {
	start := time.Now()
	var u models.User
	err := r.db.WithContext(ctx).First(&u, id).Error
	repoLog(ctx, "UserRepository.GetByID", start, err, "id="+uintToStr(id))
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// GetByUsername 根据账号获取用户
func (r *UserRepository) GetByUsername(ctx context.Context, username string) (*models.User, error) {
	start := time.Now()
	var u models.User
	err := r.db.WithContext(ctx).Where("username = ?", username).First(&u).Error
	repoLog(ctx, "UserRepository.GetByUsername", start, err, "username="+username)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// ExistsByUsername 判断账号是否存在
func (r *UserRepository) ExistsByUsername(ctx context.Context, username string) (bool, error) {
	start := time.Now()
	var count int64
	err := r.db.WithContext(ctx).Model(&models.User{}).Where("username = ?", username).Count(&count).Error
	repoLog(ctx, "UserRepository.ExistsByUsername", start, err, "username="+username)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// Create 新建用户
func (r *UserRepository) Create(ctx context.Context, user *models.User) error {
	if user == nil {
		return errors.New("user is nil")
	}
	start := time.Now()
	err := r.db.WithContext(ctx).Create(user).Error
	repoLog(ctx, "UserRepository.Create", start, err, "username="+user.Username)
	return err
}

// UpdateLastLogin 更新最近登录信息
func (r *UserRepository) UpdateLastLogin(ctx context.Context, id uint, ip string, loginAt time.Time) error {
	start := time.Now()
	err := r.db.WithContext(ctx).Model(&models.User{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"last_login_at": loginAt,
			"last_login_ip": ip,
		}).Error
	repoLog(ctx, "UserRepository.UpdateLastLogin", start, err, "id="+uintToStr(id)+" ip="+ip)
	return err
}

// UpdatePassword 更新密码（哈希与盐）
func (r *UserRepository) UpdatePassword(ctx context.Context, id uint, passwordHash, salt string) error {
	start := time.Now()
	err := r.db.WithContext(ctx).Model(&models.User{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"password_hash": passwordHash,
			"salt":          salt,
		}).Error
	repoLog(ctx, "UserRepository.UpdatePassword", start, err, "id="+uintToStr(id))
	return err
}

// UpdateStatus 启用/禁用用户
func (r *UserRepository) UpdateStatus(ctx context.Context, id uint, status int) error {
	start := time.Now()
	err := r.db.WithContext(ctx).Model(&models.User{}).Where("id = ?", id).Update("status", status).Error
	repoLog(ctx, "UserRepository.UpdateStatus", start, err, "id="+uintToStr(id)+" status="+intToStr(status))
	return err
}

// uintToStr 避免引入 strconv 的小工具
func uintToStr(n uint) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
