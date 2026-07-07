package service

import (
	"context"
	"echo-core/dto"
	"echo-core/models"
	"echo-core/repository"
	"echo-core/utils"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

// 业务错误
var (
	ErrUserNotFound        = errors.New("用户不存在")
	ErrUserExists          = errors.New("账号已存在")
	ErrInvalidCredentials  = errors.New("账号或密码错误")
	ErrUserDisabled        = errors.New("账号已被禁用")
	ErrUsernameUnavailable = errors.New("账号已被占用")
	ErrSessionInvalid      = errors.New("会话无效或已过期")
)

// UserService 用户业务服务
type UserService struct {
	repo     *repository.UserRepository
	sessions utils.SessionStore
}

// NewUserService 构造 UserService
//
// sessions 使用 utils 包提供的全局单例 store，避免早期"每个 service
// 实例独立 store"导致跨请求 session 失效的 bug。后续切 Redis 只需改
// utils/session_store.go 的实现，本构造函数无感。
func NewUserService() *UserService {
	return &UserService{
		repo:     repository.NewUserRepository(),
		sessions: utils.GetSessionStore(),
	}
}

// CheckAccount 校验账号是否可用
func (s *UserService) CheckAccount(ctx context.Context, req dto.CheckAccountRequest) (*dto.CheckAccountResponse, error) {
	username := strings.TrimSpace(req.Username)
	utils.LogWithCtx(ctx, "UserService.CheckAccount", "入参 | username=%s", username)
	if username == "" {
		return nil, errors.New("账号不能为空")
	}
	exists, err := s.repo.ExistsByUsername(ctx, username)
	if err != nil {
		utils.LogWithCtx(ctx, "UserService.CheckAccount", "查询账号失败 | username=%s err=%v", username, err)
		return nil, err
	}
	utils.LogWithCtx(ctx, "UserService.CheckAccount", "查询完成 | username=%s exists=%v", username, exists)
	if exists {
		return &dto.CheckAccountResponse{Username: username, Available: false}, ErrUsernameUnavailable
	}
	return &dto.CheckAccountResponse{Username: username, Available: true}, nil
}

// Register 注册用户
func (s *UserService) Register(ctx context.Context, req dto.RegisterRequest) (*dto.UserResponse, error) {
	username := strings.TrimSpace(req.Username)
	utils.LogWithCtx(ctx, "UserService.Register", "入参 | username=%s nicknameLen=%d emailLen=%d", username, len(req.Nickname), len(req.Email))
	if username == "" {
		return nil, errors.New("账号不能为空")
	}
	// 账号是否已存在
	exists, err := s.repo.ExistsByUsername(ctx, username)
	if err != nil {
		utils.LogWithCtx(ctx, "UserService.Register", "查询账号失败 | username=%s err=%v", username, err)
		return nil, err
	}
	if exists {
		utils.LogWithCtx(ctx, "UserService.Register", "账号已存在 | username=%s", username)
		return nil, ErrUserExists
	}

	// 生成盐值与密码哈希
	saltStart := time.Now()
	salt, err := utils.GenerateSalt()
	if err != nil {
		utils.LogWithCtx(ctx, "UserService.Register", "生成盐值失败 | err=%v", err)
		return nil, errors.New("系统异常，请稍后再试")
	}
	hashed, err := utils.HashPassword(req.Password, salt)
	if err != nil {
		utils.LogWithCtx(ctx, "UserService.Register", "密码哈希失败 | err=%v", err)
		return nil, errors.New("系统异常，请稍后再试")
	}
	utils.LogWithCtx(ctx, "UserService.Register", "盐值/哈希生成完成 | latency=%dms", time.Since(saltStart).Milliseconds())

	user := &models.User{
		Username:     username,
		PasswordHash: hashed,
		Salt:         salt,
		Nickname:     strings.TrimSpace(req.Nickname),
		Email:        strings.TrimSpace(req.Email),
		Status:       1,
	}
	if err := s.repo.Create(ctx, user); err != nil {
		utils.LogWithCtx(ctx, "UserService.Register", "创建用户失败 | username=%s err=%v", username, err)
		return nil, errors.New("创建用户失败")
	}
	utils.LogWithCtx(ctx, "UserService.Register", "创建用户成功 | id=%d username=%s", user.ID, user.Username)
	return s.toResponse(user), nil
}

// Login 登录
func (s *UserService) Login(ctx context.Context, req dto.LoginRequest, clientIP string) (*dto.LoginResponse, error) {
	username := strings.TrimSpace(req.Username)
	// 注意：登录入口**只打 username/ip/passwordLen，不打印 password 字段**
	utils.LogWithCtx(ctx, "UserService.Login", "入参 | username=%s passwordLen=%d ip=%s", username, len(req.Password), clientIP)
	if username == "" || req.Password == "" {
		return nil, ErrInvalidCredentials
	}

	// 1. 账号查询
	user, err := s.repo.GetByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 不区分账号不存在与密码错误，避免账号枚举
			utils.LogWithCtx(ctx, "UserService.Login", "登录失败，账号不存在 | username=%s ip=%s", username, clientIP)
			return nil, ErrInvalidCredentials
		}
		utils.LogWithCtx(ctx, "UserService.Login", "查询用户失败 | username=%s err=%v", username, err)
		return nil, errors.New("登录失败，请稍后再试")
	}

	if !user.IsEnabled() {
		utils.LogWithCtx(ctx, "UserService.Login", "登录失败，账号已禁用 | id=%d username=%s", user.ID, username)
		return nil, ErrUserDisabled
	}

	if !utils.VerifyPassword(req.Password, user.Salt, user.PasswordHash) {
		utils.LogWithCtx(ctx, "UserService.Login", "登录失败，密码错误 | id=%d username=%s ip=%s", user.ID, username, clientIP)
		return nil, ErrInvalidCredentials
	}

	// 2. 创建会话（默认 24h）
	sess, err := s.sessions.Create(user.ID, user.Username, 24*time.Hour)
	if err != nil {
		utils.LogWithCtx(ctx, "UserService.Login", "创建会话失败 | id=%d err=%v", user.ID, err)
		return nil, errors.New("登录失败，请稍后再试")
	}

	// 3. 更新最近登录信息（不影响会话返回）
	now := time.Now()
	if err := s.repo.UpdateLastLogin(ctx, user.ID, clientIP, now); err != nil {
		// 非关键错误，只记录日志
		utils.LogWithCtx(ctx, "UserService.Login", "更新登录信息失败 | id=%d err=%v", user.ID, err)
	}
	user.LastLoginAt = &now
	user.LastLoginIP = clientIP

	utils.LogWithCtx(ctx, "UserService.Login", "登录成功 | id=%d username=%s sid=%s... ip=%s",
		user.ID, user.Username, truncate(sess.SessionID, 8), clientIP)
	return &dto.LoginResponse{
		SessionID: sess.SessionID,
		ExpireAt:  sess.ExpireAt,
		User:      *s.toResponse(user),
	}, nil
}

// Logout 注销会话
func (s *UserService) Logout(ctx context.Context, sessionID string) error {
	utils.LogWithCtx(ctx, "UserService.Logout", "入参 | sid=%s...", truncate(sessionID, 8))
	if sessionID == "" {
		return nil
	}
	if err := s.sessions.Delete(sessionID); err != nil && !errors.Is(err, utils.ErrSessionNotFound) {
		utils.LogWithCtx(ctx, "UserService.Logout", "删除会话失败 | sid=%s... err=%v", truncate(sessionID, 8), err)
		return err
	}
	utils.LogWithCtx(ctx, "UserService.Logout", "注销完成 | sid=%s...", truncate(sessionID, 8))
	return nil
}

// CheckSession 校验会话是否有效
func (s *UserService) CheckSession(ctx context.Context, sessionID string) (*dto.CheckSessionResponse, error) {
	utils.LogWithCtx(ctx, "UserService.CheckSession", "入参 | sid=%s...", truncate(sessionID, 8))
	if strings.TrimSpace(sessionID) == "" {
		return &dto.CheckSessionResponse{Valid: false}, nil
	}
	sess, err := s.sessions.Get(sessionID)
	if err != nil {
		if errors.Is(err, utils.ErrSessionNotFound) {
			utils.LogWithCtx(ctx, "UserService.CheckSession", "会话无效或已过期 | sid=%s...", truncate(sessionID, 8))
			return &dto.CheckSessionResponse{Valid: false}, nil
		}
		utils.LogWithCtx(ctx, "UserService.CheckSession", "查询会话失败 | sid=%s... err=%v", truncate(sessionID, 8), err)
		return nil, errors.New("校验会话失败，请稍后再试")
	}
	// 刷新活跃时间
	if err := s.sessions.Touch(sessionID); err != nil {
		utils.LogWithCtx(ctx, "UserService.CheckSession", "刷新会话失败 | sid=%s... err=%v", truncate(sessionID, 8), err)
	}
	expire := sess.ExpireAt
	utils.LogWithCtx(ctx, "UserService.CheckSession", "校验通过 | sid=%s... userID=%d username=%s", truncate(sessionID, 8), sess.UserID, sess.Username)
	return &dto.CheckSessionResponse{
		Valid:    true,
		Username: sess.Username,
		UserID:   sess.UserID,
		ExpireAt: &expire,
	}, nil
}

// toResponse 实体转响应 DTO
func (s *UserService) toResponse(u *models.User) *dto.UserResponse {
	if u == nil {
		return nil
	}
	return &dto.UserResponse{
		ID:          u.ID,
		Username:    u.Username,
		Nickname:    u.Nickname,
		Email:       u.Email,
		Status:      u.Status,
		LastLoginAt: u.LastLoginAt,
		CreatedAt:   u.CreatedAt,
	}
}

// truncate 截前 n 字符，避免完整 sid 刷屏
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
